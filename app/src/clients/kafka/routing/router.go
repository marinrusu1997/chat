package routing

import (
	"chat/src/clients/kafka"
	"chat/src/platform/validation"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/creasty/defaults"
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

var ErrNoTopicHandler = errors.New("no topic handlers defined")

type ConsumerHandler func(records []*kgo.Record)

// ConsumerRouter routes Kafka records fetched from different topics to their respective handlers.
// It requires Cooperative Sticky rebalancing strategy and AutoCommitMarks to be enabled in the Kafka client configuration.
type ConsumerRouter struct {
	// @fixme test rebalances
	kafkaClient             *kafka.Client
	topicHandlers           map[string]ConsumerHandler
	runningHandlersWg       sync.WaitGroup
	handlerConcurrencySem   *semaphore.Weighted
	handlerTimeoutEstimator *timeoutEstimator
	stopPollFetches         context.CancelFunc
	pollFetchesStopped      chan struct{}
	logger                  *zerolog.Logger
}

type ConsumerRouterOptions struct {
	Client             *kafka.Client   `validate:"required"`
	MinHandlerTimeout  time.Duration   `validate:"required,min=100000000,max=1000000000" default:"500ms"`                              // 100ms to 1s
	MaxHandlerTimeout  time.Duration   `validate:"required,min=1000000000,max=10000000000,gtfield=MinHandlerTimeout" default:"5000ms"` // 1s to 10s
	HandlerConcurrency int64           `validate:"required,min=1,max=1000" default:"100"`
	Logger             *zerolog.Logger `validate:"required"`
}

func NewConsumerRouter(options *ConsumerRouterOptions) (*ConsumerRouter, error) {
	if err := defaults.Set(options); err != nil {
		return nil, fmt.Errorf("failed to set config defaults: %w", err)
	}
	if err := validation.Instance.Struct(options); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	timeoutEstimator, err := newTimeoutEstimator(&timeoutEstimatorOptions{
		MinTimeout: options.MinHandlerTimeout,
		MaxTimeout: options.MaxHandlerTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create timeout estimator: %w", err)
	}

	return &ConsumerRouter{
		kafkaClient:             options.Client,
		topicHandlers:           make(map[string]ConsumerHandler),
		handlerConcurrencySem:   semaphore.NewWeighted(options.HandlerConcurrency),
		handlerTimeoutEstimator: timeoutEstimator,
		pollFetchesStopped:      make(chan struct{}),
		logger:                  options.Logger,
	}, nil
}

func (r *ConsumerRouter) OnRecordsFrom(topic string, handler ConsumerHandler) {
	r.topicHandlers[topic] = handler
	r.kafkaClient.Driver.AddConsumeTopics(topic)
}

func (r *ConsumerRouter) Start() error {
	if len(r.topicHandlers) == 0 {
		return ErrNoTopicHandler
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.stopPollFetches = cancel
	go r.pollFetches(ctx)
	return nil
}

func (r *ConsumerRouter) Stop() {
	r.stopPollFetches()
	<-r.pollFetchesStopped
}

func (r *ConsumerRouter) pollFetches(ctx context.Context) {
	defer func() {
		r.runningHandlersWg.Wait()

		if err := r.kafkaClient.Driver.CommitMarkedOffsets(context.Background()); err != nil {
			r.logger.Error().Err(err).Msg("CommitMarkedOffsets failed on shutdown of poll fetches loop.")
		}

		close(r.pollFetchesStopped)
		r.logger.Info().Msg("Poll Fetches loop stopped.")
	}()

	for {
		// Fetch records
		fetches := r.kafkaClient.Driver.PollFetches(ctx)

		// Stop condition
		if err := fetches.Err0(); err != nil {
			if errors.Is(err, kgo.ErrClientClosed) || errors.Is(err, context.Canceled) {
				r.logger.Info().Err(err).Msg("Exiting poll fetches loop.")
				break
			}

			r.logger.Warn().Err(err).Msg("PollFetches failed.")
		}

		// Error handling
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, err := range errs {
				severity := classifyFetchError(err.Err)
				switch severity {
				case fetchErrorSeverityLow:
					r.logger.Warn().Err(err.Err).Msgf("Temporary error while fetching topic-partition %s-%d.", err.Topic, err.Partition)
				case fetchErrorSeverityMedium:
					r.logger.Error().Err(err.Err).Msgf("Persistent error on topic-partition %s-%d, pausing partition.", err.Topic, err.Partition)
					r.kafkaClient.Driver.PauseFetchPartitions(map[string][]int32{
						err.Topic: {err.Partition},
					})
				case fetchErrorSeverityHigh:
					r.logger.Error().Err(err.Err).Msgf("Fatal error on topic-partition %s-%d, stopping poll fetches loop.", err.Topic, err.Partition)
					return
				}
			}
		}

		// No records fetched
		if fetches.Empty() {
			continue
		}
		r.logger.Debug().Msgf("Fetched %d records for processing.", fetches.NumRecords())

		// Records processing
		handlerTimeout := r.handlerTimeoutEstimator.EstimateTimeout(percentile(90))
		ctxHandlersDeadline, cancelHandlersDeadline := context.WithTimeout(ctx, handlerTimeout)

		var iterationWg sync.WaitGroup
		fetches.EachTopic(func(fetchTopic kgo.FetchTopic) {
			handler, found := r.topicHandlers[fetchTopic.Topic]
			if !found {
				r.logger.Warn().Msgf("There is no registered handler for topic '%s'.", fetchTopic.Topic)
				return
			}

			fetchTopic.EachPartition(func(fetchPartition kgo.FetchPartition) {
				if len(fetchPartition.Records) == 0 {
					r.logger.Warn().Msgf("There are no fetched records from topic-partition %s-%d.", fetchTopic.Topic, fetchPartition.Partition)
					return
				}

				if err := r.handlerConcurrencySem.Acquire(ctx, 1); err != nil {
					if errors.Is(err, context.Canceled) {
						r.logger.Warn().Err(err).Msgf(
							"Shutdown in progress, skipping handler for topic-partition %s-%d.", fetchTopic.Topic, fetchPartition.Partition,
						)
						return
					}

					r.logger.Error().Err(err).Msgf(
						"Failed to acquire handler semaphore for topic-partition %s-%d",
						fetchTopic.Topic, fetchPartition.Partition,
					)
					return
				}

				iterationWg.Add(1) //nolint:revive // we need the old version of wg.Add here
				go func(topic string, partition int32, records []*kgo.Record) {
					handlerDoneCh := make(chan struct{})
					r.runningHandlersWg.Add(1)
					go func() {
						defer close(handlerDoneCh)
						defer r.runningHandlersWg.Done()
						defer r.handlerConcurrencySem.Release(1)

						start := time.Now()
						handler(records)
						r.handlerTimeoutEstimator.AddSample(time.Since(start))

						r.kafkaClient.Driver.MarkCommitRecords(records...)
					}()

					select {
					case <-handlerDoneCh:
						iterationWg.Done()

					case <-ctxHandlersDeadline.Done():
						partitionToPause := map[string][]int32{
							topic: {partition},
						}

						r.logger.Warn().Msgf("Pausing partition %s-%d due to handler timeout %v.", topic, partition, handlerTimeout)
						r.kafkaClient.Driver.PauseFetchPartitions(partitionToPause)

						iterationWg.Done()

						<-handlerDoneCh

						r.logger.Info().Msgf("Resuming partition %s-%d.", topic, partition)
						r.kafkaClient.Driver.ResumeFetchPartitions(partitionToPause)
					}
				}(fetchTopic.Topic, fetchPartition.Partition, fetchPartition.Records)
			})
		})

		iterationWg.Wait()
		cancelHandlersDeadline()
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
