package kafka

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dustinxie/lockfree"
	"github.com/dustinxie/lockfree/hashmap"
	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/sync/semaphore"
)

type fetchErrorSeverity uint8

const (
	fetchErrorSeverityLow = iota
	fetchErrorSeverityMedium
	fetchErrorSeverityHigh
)

const fetchTimeout = 30 * time.Second

type TopicPartitionHandler func(topic string, partition int32, records []*kgo.Record)

type ConsumptionRouter struct {
	kafkaClient                      *Client //	@fixme	cooperative sticky + AutoCommitMarks
	topicHandlers                    map[string]TopicPartitionHandler
	partitionsRunStatus              lockfree.HashMap // topic -> HashMap partition -> bool
	partitionsRunningWg              sync.WaitGroup
	partitionsRunningSemaphore       *semaphore.Weighted
	recordProcessingTimeoutEstimator *TimeoutEstimator
	logger                           *zerolog.Logger
}

type ConsumptionRouterOptions struct {
	Client               *Client
	TimeoutEstimator     *TimeoutEstimator //	@fixme	better construct ourselves
	PartitionParallelism int64
	Logger               *zerolog.Logger
}

func NewConsumptionRouter(options *ConsumptionRouterOptions) *ConsumptionRouter {
	router := &ConsumptionRouter{
		kafkaClient:                      options.Client,
		topicHandlers:                    make(map[string]TopicPartitionHandler),
		partitionsRunStatus:              lockfree.NewHashMap(hashmap.BucketSizeOption(16)),
		recordProcessingTimeoutEstimator: options.TimeoutEstimator,
		partitionsRunningSemaphore:       semaphore.NewWeighted(options.PartitionParallelism),
		logger:                           options.Logger,
	}
	return router
}

func (r *ConsumptionRouter) Handle(topic string, handler TopicPartitionHandler) {
	r.topicHandlers[topic] = handler
	r.kafkaClient.Driver.AddConsumeTopics(topic)
}

func (r *ConsumptionRouter) Run(ctx context.Context) {
	defer func() {
		r.partitionsRunningWg.Wait()
		if err := r.kafkaClient.Driver.CommitMarkedOffsets(context.Background()); err != nil {
			r.logger.Error().Err(err).Msg("CommitMarkedOffsets failed on shutdown")
		}
		r.logger.Info().Msg("Consumption router stopped.")
	}()

	for {
		// Resume paused partitions
		pausedPartitions := r.kafkaClient.Driver.PauseFetchPartitions(nil)
		if len(pausedPartitions) > 0 {
			partitionsToResume := make(map[string][]int32)

			for topic, partitions := range pausedPartitions {
				for _, pausedPartition := range partitions {
					runningPartitions, found := r.partitionsRunStatus.Get(topic)
					if !found || runningPartitions == nil {
						continue
					}

					running, found := runningPartitions.(lockfree.HashMap).Get(pausedPartition) //nolint:errcheck,forcetypeassert // we know the type
					if !found || running.(bool) {                                               //nolint:errcheck,revive,forcetypeassert // we know the type
						continue
					}

					partitionsToResume[topic] = append(partitionsToResume[topic], pausedPartition)
					r.logger.Info().Msgf("Resuming partition %s-%d", topic, pausedPartition)
				}
			}

			if len(partitionsToResume) > 0 {
				r.kafkaClient.Driver.ResumeFetchPartitions(partitionsToResume)
			}
		}

		// Fetch records
		fetchCtx, fetchCtxCancel := context.WithTimeout(ctx, fetchTimeout)
		fetches := r.kafkaClient.Driver.PollFetches(fetchCtx) //	@fixme	what happens if everything is paused?
		fetchCtxCancel()

		// Stop condition
		if err := fetches.Err0(); err != nil {
			r.logger.Warn().Err(err).Msg("PollFetches failed.")

			if errors.Is(err, kgo.ErrClientClosed) || errors.Is(err, context.Canceled) {
				return
			}
		}

		// Error handling
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, err := range errs {
				sev := classifyFetchError(err.Err)
				switch sev {
				case fetchErrorSeverityLow:
					r.logger.Warn().Err(err.Err).Msgf("Temporary error while fetching topic-partition %s-%d", err.Topic, err.Partition)
				case fetchErrorSeverityMedium:
					r.logger.Error().Err(err.Err).Msgf("Persistent error on topic-partition %s-%d, pausing partition", err.Topic, err.Partition)
					r.kafkaClient.Driver.PauseFetchPartitions(map[string][]int32{
						err.Topic: {err.Partition},
					})
				case fetchErrorSeverityHigh:
					r.logger.Error().Err(err.Err).Msgf("Fatal error on topic-partition %s-%d, stopping router", err.Topic, err.Partition)
					return
				}
			}
		}

		// No records fetched
		if fetches.Empty() {
			continue
		}

		// Records processing
		var wg sync.WaitGroup
		fetches.EachTopic(func(fetchTopic kgo.FetchTopic) {
			handler, found := r.topicHandlers[fetchTopic.Topic]
			if !found {
				r.logger.Warn().Msgf("There is no registered handler for topic '%s'.", fetchTopic.Topic)
				return
			}

			runningPartitions, found := r.partitionsRunStatus.Get(fetchTopic.Topic)
			if !found {
				runningPartitions = lockfree.NewHashMap(hashmap.BucketSizeOption(16))
				r.partitionsRunStatus.Set(fetchTopic.Topic, runningPartitions)
			}

			fetchTopic.EachPartition(func(fetchPartition kgo.FetchPartition) {
				if len(fetchPartition.Records) == 0 {
					return
				}

				if err := r.partitionsRunningSemaphore.Acquire(ctx, 1); err != nil {
					if errors.Is(err, context.Canceled) {
						r.logger.Warn().Err(err).Msgf(
							"Shutdown in progress, skipping handler for topic-partition %s-%d during partition semaphore acquire.",
							fetchTopic.Topic, fetchPartition.Partition,
						)
						return
					}

					r.logger.Error().Err(err).Msgf(
						"Failed to acquire partition semaphore for topic-partition %s-%d",
						fetchTopic.Topic, fetchPartition.Partition,
					)
					return
				}

				wg.Add(1) //nolint:revive // we need the old version of wg.Add here
				go func(topic string, partition int32, records []*kgo.Record) {
					defer wg.Done()
					defer r.partitionsRunningSemaphore.Release(1)

					r.partitionsRunningWg.Add(1)
					runningPartitions.(lockfree.HashMap).Set(partition, true) //nolint:errcheck,forcetypeassert // we know the type
					defer func() {
						r.partitionsRunningWg.Done()
						runningPartitions.(lockfree.HashMap).Set(partition, false) //nolint:errcheck,forcetypeassert // we know the type
					}()

					start := time.Now()
					handler(topic, partition, records)
					r.recordProcessingTimeoutEstimator.AddSample(time.Since(start))

					r.kafkaClient.Driver.MarkCommitRecords(records...)
				}(fetchTopic.Topic, fetchPartition.Partition, fetchPartition.Records)
			})
		})

		// Wait for handlers OR timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		timeout := r.recordProcessingTimeoutEstimator.EstimateTimeout(Percentile(90))
		select {
		case <-done:
			r.logger.Debug().Msgf("All handlers finished before timeout %v", timeout)

		case <-time.After(timeout):
			r.logger.Warn().Msgf("Timeout %v exceeded; pausing slow partitions", timeout)

			partitionsToPause := make(map[string][]int32)

			// Detect which partitions are still running
			r.partitionsRunStatus.Lock()
			for k, v, ok := r.partitionsRunStatus.Next(); ok; k, v, ok = r.partitionsRunStatus.Next() {
				topic := k.(string)                       //nolint:errcheck,forcetypeassert,revive // we know the type
				runningPartitions := v.(lockfree.HashMap) //nolint:errcheck,forcetypeassert,revive // we know the type

				runningPartitions.Lock()
				for k, v, ok := runningPartitions.Next(); ok; k, v, ok = runningPartitions.Next() {
					partition := k.(int32) //nolint:errcheck,forcetypeassert,revive // we know the type
					running := v.(bool)    //nolint:errcheck,forcetypeassert,revive // we know the type

					if running {
						partitionsToPause[topic] = append(partitionsToPause[topic], partition)
						r.logger.Warn().Msgf("Pausing slow partition %s-%d", topic, partition)
					}
				}
				runningPartitions.Unlock()
			}
			r.partitionsRunStatus.Unlock()

			// Pause slow partitions
			r.kafkaClient.Driver.PauseFetchPartitions(partitionsToPause)
		}
	}
}

func classifyFetchError(err error) fetchErrorSeverity {
	var ke *kerr.Error
	if errors.As(err, &ke) {
		switch ke.Code {
		case kerr.UnknownTopicOrPartition.Code:
			return fetchErrorSeverityMedium
		case kerr.GroupAuthorizationFailed.Code,
			kerr.ClusterAuthorizationFailed.Code:
			return fetchErrorSeverityHigh
		default:
			if kerr.IsRetriable(ke) {
				return fetchErrorSeverityLow
			}
			return fetchErrorSeverityMedium
		}
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fetchErrorSeverityLow
	}

	return fetchErrorSeverityMedium
}
