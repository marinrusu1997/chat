package kafka

import (
	"chat/src/platform/perr"
	"chat/src/platform/validation"
	"chat/src/util"
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"time"

	"github.com/creasty/defaults"
	"github.com/rs/zerolog"
	"github.com/samber/oops"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/plugin/kzerolog"
)

type GeneralConfig struct {
	ClientID               string        `validate:"required,alphanum,min=10,max=50"`
	ServiceName            string        `validate:"required,alphanum,min=5,max=50"`
	ServiceVersion         string        `validate:"required,min=1,max=30"`
	SeedBrokers            []string      `validate:"required,min=1,max=10,unique,dive,required,hostname_port"`
	TLSConfig              *tls.Config   ``
	Username               string        `validate:"required_with=Password,required,min=5,max=50"`
	Password               string        `validate:"required_with=Username,required,min=5,max=50"`
	RequestTimeoutOverhead time.Duration `validate:"min=1000000000,max=15000000000" default:"5s"` // [1s, 15s], default 5s
}

type ProducerConfig struct {
	RequiredAcks          *kgo.Acks              ``                                                          // nil means AllISRAcks
	RecordPartitioner     *kgo.Partitioner       ``                                                          // nil means default partitioner
	BatchCompression      []kgo.CompressionCodec ``                                                          // defaults to Zstd, Lz4, Snappy, None
	MaxWriteBytes         int32                  `validate:"gte=1024,lte=536870912" default:"104857600"`     // [1KB, 512MB], default 100MB
	BatchMaxBytes         int32                  `validate:"gte=1024,lte=10485760" default:"8388608"`        // [1KB, 10MB], default 8MB
	MaxBufferedRecords    int                    `validate:"gte=1000,lte=1000000" default:"100000"`          // [1K, 1M], default 100K
	MaxBufferedBytes      int                    `validate:"gte=1048576,lte=1073741824" default:"536870912"` // [1MB, 1GB], default 512MB
	RequestTimeout        time.Duration          `validate:"min=1000000000,max=30000000000" default:"5s"`    // [1s, 30s], default 5s
	RecordRetries         int                    `validate:"gte=1,lte=30" default:"5"`                       // [1, 30], default 10
	UnknownTopicRetries   int                    `validate:"gte=0,lte=5" default:"1"`                        // [0, 5], default 1
	ProducerLinger        time.Duration          `validate:"gte=-1,lte=10000000000" default:"50ms"`          // [-1, 10s], default 50ms (-1 = disabled)
	RecordDeliveryTimeout time.Duration          `validate:"gte=10000000000,lte=300000000000" default:"30s"` // [10s, 5min], default 30s
}

type TransactionConfig struct {
	ID                            string        `validate:"required,alphanum,min=10,max=50"`
	Timeout                       time.Duration `validate:"required,gte=5000000000,lte=300000000000"`      // [5s, 5min]
	SessionTimeout                time.Duration `validate:"gte=6000000000,lte=360000000000" default:"45s"` // [6s, 6min]
	RequireStableFetchOffsets     bool          ``
	ConcurrentTransactionsBackoff time.Duration `validate:"required,gte=10000000,lte=100000000" default:"50ms"` // [10ms, 100ms]
}

type ConsumerConfig struct {
	MaxReadBytes           int32                           `validate:"gte=1024,lte=536870912" default:"104857600"` // [1KB, 512MB], default 100MB
	FetchMaxWait           time.Duration                   `validate:"gte=500000000,lte=10000000000" default:"1s"` // [500ms, 10s], default 1s
	FetchMinBytes          int32                           `validate:"gte=1024,lte=10485760" default:"51200"`      // [1KB, 10MB], default 50KB
	FetchMaxBytes          int32                           `validate:"gte=1024,lte=104857600" default:"52428800"`  // [1KB, 100MB], default 50MB
	FetchMaxPartitionBytes int32                           `validate:"gte=1024,lte=10485760" default:"2097152"`    // [1KB, 10MB], default 2MB
	ConsumeStartOffset     *kgo.Offset                     ``                                                      // default AtStart
	ConsumeResetOffset     *kgo.Offset                     ``                                                      // default AtStart
	FetchIsolationLevel    *kgo.IsolationLevel             ``                                                      // default ReadUncommitted
	ConsumePreferringLagFn kgo.PreferLagFn                 ``                                                      // default PreferLagAt(500)
	MaxConcurrentFetches   int                             `validate:"gte=-1,lte=100" default:"10"`                // [-1, 100], default 10
	ConsumeTopics          []string                        `validate:"unique"`
	ConsumePartitions      map[string]map[int32]kgo.Offset ``
	RegexConsumption       bool                            ``
}

type ConsumerGroupConfig struct {
	GroupID              string `validate:"required,min=5,max=50"`
	InstanceID           string `validate:"omitempty,min=5,max=50"`
	Balancers            []kgo.GroupBalancer
	SessionTimeout       time.Duration `validate:"gte=10000000000,lte=600000000000" default:"60s"` // [10s, 10min], default 60s
	RebalanceTimeout     time.Duration `validate:"gte=10000000000,lte=60000000000" default:"60s"`  // [10s, 1min], default 60s
	HeartbeatInterval    time.Duration `validate:"gte=5000000000,lte=15000000000" default:"5s"`    // [5s, 15s], default 5s
	OnPartitionsRevoked  func(context.Context, *kgo.Client, map[string][]int32)
	OnPartitionsAssigned func(context.Context, *kgo.Client, map[string][]int32)
	OnPartitionsLost     func(context.Context, *kgo.Client, map[string][]int32)
	OnOffsetsFetched     func(context.Context, *kgo.Client, *kmsg.OffsetFetchResponse) error
	BlockRebalanceOnPoll bool
	DisableAutoCommit    bool
	GreedyAutoCommit     bool
	AutoCommitMarks      bool
	AutoCommitInterval   time.Duration `validate:"gte=100000000,lte=10000000000" default:"1s"` // [100ms, 10s], default 1s
	AutoCommitCallback   func(*kgo.Client, *kmsg.OffsetCommitRequest, *kmsg.OffsetCommitResponse, error)
}

type ConfigurationLoggers struct {
	Client zerolog.Logger
	Driver zerolog.Logger
}

type ConfigurationBuilder struct {
	options  map[string]kgo.Opt
	required []string
	err      error
	logger   *ConfigurationLoggers
}

func NewConfigurationBuilder(loggers *ConfigurationLoggers) ConfigurationBuilder {
	return ConfigurationBuilder{
		options:  make(map[string]kgo.Opt),
		required: []string{"ClientID"},
		err:      nil,
		logger:   loggers,
	}
}

func (b *ConfigurationBuilder) SetGeneralConfig(config *GeneralConfig) bool {
	if !b.applyDefaultsAndValidate(&config) {
		return false
	}

	return b.setOption("ClientID", kgo.ClientID(config.ClientID)) &&
		b.setOption("DialTimeout", kgo.DialTimeout(5*time.Second)) &&
		((config.TLSConfig != nil && b.setOption("DialTLSConfig", kgo.DialTLSConfig(config.TLSConfig))) || true) &&
		b.setOption("RequestTimeoutOverhead", kgo.RequestTimeoutOverhead(config.RequestTimeoutOverhead)) &&
		b.setOption("ConnIdleTimeout", kgo.ConnIdleTimeout(10*time.Minute)) &&
		b.setOption("SoftwareNameAndVersion", kgo.SoftwareNameAndVersion(config.ServiceName, config.ServiceVersion)) &&
		b.setOption("WithLogger", kgo.WithLogger(kzerolog.New(&b.logger.Driver))) &&
		b.setOption("SeedBrokers", kgo.SeedBrokers(config.SeedBrokers...)) &&
		b.setOption("RetryBackoffFn", kgo.RetryBackoffFn(func(attempts int) time.Duration {
			// Start at 100ms and double up to a max of 5s
			baseDelay := 100 * time.Millisecond
			maxDelay := 5 * time.Second

			// Calculate 2^attempts (clamped)
			delay := min(time.Duration(baseDelay.Nanoseconds()*int64(math.Pow(2, float64(attempts)))), maxDelay)

			// Add jitter (e.g., +/- 20% randomness)
			jitter := time.Duration(rand.Float64() * float64(delay.Nanoseconds()) * 0.4) //nolint:gosec    // 40% range
			delay = delay - (delay / 5) + jitter                                         // Apply -20% offset and add jitter up to +20%

			return delay
		})) &&
		b.setOption("RetryTimeout", kgo.RetryTimeout(30*time.Second)) &&
		b.setOption("RetryTimeoutFn", kgo.RetryTimeoutFn(func(req int16) time.Duration {
			switch kmsg.Key(req) { //nolint:revive // We don't need to cover every single key here.
			// 1. Group Membership & Stability (Critical, must fail quickly on session loss)
			case kmsg.JoinGroup, kmsg.SyncGroup, kmsg.LeaveGroup:
				// Core rebalance steps. Must fail within a reasonable window,
				// ideally close to the SessionTimeout (default 45s) but often a bit
				// less to avoid the full penalty.
				return 45 * time.Second

			case kmsg.Heartbeat:
				// The most time-sensitive group request. A prolonged failure is
				// a high risk for an unnecessary rebalance. Set it significantly
				// lower than the SessionTimeout to force a faster failure/retry cycle.
				return 20 * time.Second

			case kmsg.FindCoordinator:
				// Finding the coordinator should be fast. If it fails repeatedly,
				// something is fundamentally wrong. Allow some time for retries.
				return 30 * time.Second

			case kmsg.ConsumerGroupHeartbeat, kmsg.ConsumerGroupDescribe:
				// Modern consumer group protocol requests. Treat similarly to group coordination.
				return 45 * time.Second

			// 2. Transactional Requests (Must be resilient but not infinite)
			case kmsg.InitProducerID:
				// A producer must get its ID to proceed. Allow time for broker failover.
				return 90 * time.Second

			case kmsg.TxnOffsetCommit, kmsg.AddPartitionsToTxn, kmsg.AddOffsetsToTxn, kmsg.EndTxn:
				// Transaction control requests. They need a generous timeout to ensure
				// atomicity and survive brief broker outages.
				return 60 * time.Second

			// 3. Cluster Metadata & Discovery (Can tolerate longer delays)
			case kmsg.Metadata, kmsg.ApiVersions:
				// Essential for bootstrapping and recovering from broker failures/restarts.
				// Allow the longest timeout to maximize cluster discovery success.
				return 120 * time.Second

			case kmsg.ListOffsets, kmsg.OffsetFetch, kmsg.OffsetCommit:
				// Offset management. Important for consumer recovery, but not as critical
				// as Heartbeat. Allow time to find a healthy replica/broker.
				return 45 * time.Second

			// 4. Administrative & Non-Critical Requests (Very long timeout is acceptable)
			case kmsg.CreateTopics, kmsg.DeleteTopics, kmsg.CreatePartitions, kmsg.DeleteGroups,
				kmsg.DescribeACLs, kmsg.CreateACLs, kmsg.DeleteACLs, kmsg.DescribeConfigs,
				kmsg.AlterConfigs, kmsg.ElectLeaders:
				// These are control-plane/admin operations, not on the hot data path.
				// Latency is not critical, and they should be resilient to long-term broker
				// instability or maintenance events.
				return 180 * time.Second // 3 minutes

			// 5. Default/Others
			default:
				// Fallback for all other keys not explicitly listed (e.g., DeleteRecords,
				// DescribeProducers, internal Raft keys, etc.).
				// Use a common, moderate default, similar to your previous 30s.
				return 30 * time.Second
			}
		})) &&
		b.setOption("MetadataMaxAge", kgo.MetadataMaxAge(3*time.Minute)) &&
		b.setOption("MetadataMinAge", kgo.MetadataMinAge(7*time.Second)) &&
		((config.Username != "" && config.Password != "" && b.setOption("SASL", kgo.SASL(plain.Auth{
			User: config.Username,
			Pass: config.Password,
		}.AsMechanism()))) || true)
}

func (b *ConfigurationBuilder) SetProducerConfig(config *ProducerConfig) bool {
	if !b.applyDefaultsAndValidate(&config) {
		return false
	}
	if config.RequestTimeout*time.Duration(config.RecordRetries) > config.RecordDeliveryTimeout {
		b.err = oops.
			In(util.GetFunctionName()).
			Code(perr.EINVAL).
			Errorf(
				"RecordDeliveryTimeout (%s) must be greater than RequestTimeout (%s) * RecordRetries (%d) = %s",
				config.RecordDeliveryTimeout, config.RequestTimeout, config.RecordRetries,
				config.RequestTimeout*time.Duration(config.RecordRetries),
			)
		return false
	}

	if config.RequiredAcks == nil {
		allAcks := kgo.AllISRAcks()
		config.RequiredAcks = &allAcks
	}
	if config.RecordPartitioner == nil {
		partitioner := kgo.UniformBytesPartitioner(256*1024, true, true, nil) // 256KB
		config.RecordPartitioner = &partitioner
	}
	if len(config.BatchCompression) == 0 {
		config.BatchCompression = append(config.BatchCompression,
			kgo.ZstdCompression(),   // 1. Highest compression ratio for text data.
			kgo.Lz4Compression(),    // 2. Extremely fast compression, low CPU cost.
			kgo.SnappyCompression(), // 3. Good general-purpose, fast fallback.
			kgo.NoCompression(),     // 4. Final safety net.
		)
	}

	return b.setOption("BrokerMaxWriteBytes", kgo.BrokerMaxWriteBytes(config.MaxWriteBytes)) &&
		b.setOption("RequiredAcks", kgo.RequiredAcks(*config.RequiredAcks)) &&
		b.setOption("ProducerBatchCompression", kgo.ProducerBatchCompression(config.BatchCompression...)) &&
		b.setOption("ProducerBatchMaxBytes", kgo.ProducerBatchMaxBytes(config.BatchMaxBytes)) &&
		b.setOption("MaxBufferedRecords", kgo.MaxBufferedRecords(config.MaxBufferedRecords)) &&
		b.setOption("MaxBufferedBytes", kgo.MaxBufferedBytes(config.MaxBufferedBytes)) &&
		b.setOption("ProduceRequestTimeout", kgo.ProduceRequestTimeout(config.RequestTimeout)) &&
		b.setOption("RecordRetries", kgo.RecordRetries(config.RecordRetries)) &&
		b.setOption("UnknownTopicRetries", kgo.UnknownTopicRetries(config.UnknownTopicRetries)) &&
		((config.ProducerLinger > 0 && b.setOption("ProducerLinger", kgo.ProducerLinger(config.ProducerLinger))) || true) &&
		b.setOption("RecordDeliveryTimeout", kgo.RecordDeliveryTimeout(config.RecordDeliveryTimeout)) &&
		b.setOption("ConsiderMissingTopicDeletedAfter", kgo.ConsiderMissingTopicDeletedAfter(20*time.Second)) &&
		b.setOption("RecordPartitioner", kgo.RecordPartitioner(*config.RecordPartitioner)) &&
		b.setOption("ProducerOnDataLossDetected", kgo.ProducerOnDataLossDetected(func(topic string, partition int32) {
			// CRITICAL: Log this event and send an alert (e.g., to PagerDuty or Slack)
			b.logger.Client.Error().Msgf("!!! CRITICAL KAFKA PRODUCER DATA LOSS DETECTED !!! Topic: %s, Partition: %d. Producer is CONTINUING.", topic, partition)
			// We would also need call an alerting service here:
			// AlertService.Trigger("Kafka Data Loss", fmt.Sprintf("Topic: %s, Partition: %d", topic, partition))
		}))
}

func (b *ConfigurationBuilder) SetTransactionConfig(config *TransactionConfig) bool {
	if !b.applyDefaultsAndValidate(&config) {
		return false
	}

	if !config.RequireStableFetchOffsets {
		if config.Timeout <= config.SessionTimeout {
			b.logger.Client.Warn().Msgf(
				"Transaction timeout (%s) should be greater than the group session timeout (%s) when "+
					"RequireStableFetchOffsets is false. This will likely lead to duplicate messages in failure scenarios.",
				config.Timeout, config.SessionTimeout,
			)
		}
	} else {
		if config.Timeout >= 10*time.Second {
			b.logger.Client.Warn().Msgf(
				"With RequireStableFetchOffsets enabled, it is recommended to set a low transaction timeout "+
					"(e.g., 10s) to prevent long blocking periods. Current timeout is %s.",
				config.Timeout,
			)
		}
	}

	return b.setOption("TransactionalID", kgo.TransactionalID(config.ID)) &&
		b.setOption("TransactionTimeout", kgo.TransactionTimeout(config.Timeout)) &&
		b.setOption("SessionTimeout", kgo.SessionTimeout(config.SessionTimeout)) &&
		b.setOption("ConcurrentTransactionsBackoff", kgo.ConcurrentTransactionsBackoff(config.ConcurrentTransactionsBackoff)) &&
		((config.RequireStableFetchOffsets && b.setOption("RequireStableFetchOffsets", kgo.RequireStableFetchOffsets())) || true)
}

func (b *ConfigurationBuilder) SetConsumerConfig(config *ConsumerConfig) bool {
	if !b.applyDefaultsAndValidate(&config) {
		return false
	}

	partitionsGiven := len(config.ConsumePartitions) > 0
	topicsGiven := len(config.ConsumeTopics) > 0
	if topicsGiven && partitionsGiven {
		b.err = oops.
			In(util.GetFunctionName()).
			Code(perr.EINVAL).
			Errorf("invalid configuration: cannot provide both ConsumeTopics and ConsumePartitions; only one is allowed")
		return false
	}
	if partitionsGiven && config.RegexConsumption {
		b.err = oops.
			In(util.GetFunctionName()).
			Code(perr.EINVAL).
			Errorf("invalid configuration: RegexConsumption must be false when ConsumePartitions is provided")
		return false
	}

	return b.setOption("BrokerMaxReadBytes", kgo.BrokerMaxReadBytes(config.MaxReadBytes)) &&
		b.setOption("FetchMaxWait", kgo.FetchMaxWait(config.FetchMaxWait)) &&
		b.setOption("FetchMinBytes", kgo.FetchMinBytes(config.FetchMinBytes)) &&
		b.setOption("FetchMaxBytes", kgo.FetchMaxBytes(config.FetchMaxBytes)) &&
		b.setOption("FetchMaxPartitionBytes", kgo.FetchMaxPartitionBytes(config.FetchMaxPartitionBytes)) &&
		((config.ConsumeStartOffset != nil && b.setOption("ConsumeStartOffset", kgo.ConsumeStartOffset(*config.ConsumeStartOffset))) || true) &&
		((config.ConsumeResetOffset != nil && b.setOption("ConsumeResetOffset", kgo.ConsumeResetOffset(*config.ConsumeResetOffset))) || true) &&
		((config.FetchIsolationLevel != nil && b.setOption("FetchIsolationLevel", kgo.FetchIsolationLevel(*config.FetchIsolationLevel))) || true) &&
		((config.ConsumePreferringLagFn != nil && b.setOption("ConsumePreferringLagFn", kgo.ConsumePreferringLagFn(config.ConsumePreferringLagFn))) || true) &&
		((config.MaxConcurrentFetches > 0 && b.setOption("MaxConcurrentFetches", kgo.MaxConcurrentFetches(config.MaxConcurrentFetches))) || true) &&
		b.setOption("RecheckPreferredReplicaInterval", kgo.RecheckPreferredReplicaInterval(20*time.Minute)) &&
		((topicsGiven && b.setOption("ConsumeTopics", kgo.ConsumeTopics(config.ConsumeTopics...))) || true) &&
		((partitionsGiven && b.setOption("ConsumePartitions", kgo.ConsumePartitions(config.ConsumePartitions))) || true) &&
		((config.RegexConsumption && b.setOption("ConsumeRegex", kgo.ConsumeRegex())) || true)
}

func (b *ConfigurationBuilder) SetConsumerGroupConfig(config *ConsumerGroupConfig) bool {
	if !b.applyDefaultsAndValidate(&config) {
		return false
	}

	if config.OnPartitionsRevoked == nil && !config.BlockRebalanceOnPoll {
		config.OnPartitionsRevoked = func(ctx context.Context, cl *kgo.Client, revoked map[string][]int32) {
			b.logger.Client.Warn().Msgf("Partitions revoked: %v", revoked)

			if err := cl.CommitUncommittedOffsets(ctx); err != nil {
				b.logger.Client.Error().Msgf("Blocking commit in OnPartitionsRevoked failed: %v", err)
			} else {
				b.logger.Client.Info().Msg("Successfully committed uncommitted offsets before revocation.")
			}
		}
	}
	if config.OnPartitionsAssigned == nil {
		config.OnPartitionsAssigned = func(_ context.Context, _ *kgo.Client, assigned map[string][]int32) {
			b.logger.Client.Info().Msgf("Partitions assigned: %v", assigned)
		}
	}
	if config.OnPartitionsLost == nil {
		config.OnPartitionsLost = func(_ context.Context, _ *kgo.Client, lost map[string][]int32) {
			b.logger.Client.Error().Msgf("Partitions lost due to unrecoverable group error: %v", lost)
		}
	}
	if config.OnOffsetsFetched == nil {
		config.OnOffsetsFetched = func(_ context.Context, _ *kgo.Client, resp *kmsg.OffsetFetchResponse) error {
			var sb strings.Builder

			if _, err := fmt.Fprintf(&sb, "Offsets Fetched: Version=%d, Throttle=%dms, TopLevelError=%d\n",
				resp.Version, resp.ThrottleMillis, resp.ErrorCode); err != nil {
				b.logger.Client.Error().Err(err).Msg("Failed to write offset fetch response header")
				return nil
			}

			for _, groupResp := range resp.Groups {
				if _, err := fmt.Fprintf(&sb, "  Group: %s, GroupError: %d\n", groupResp.Group, groupResp.ErrorCode); err != nil {
					b.logger.Client.Error().Err(err).Msg("Failed to write group header")
					return nil
				}

				for _, topicResp := range groupResp.Topics {
					for _, partitionResp := range topicResp.Partitions {
						if _, err := fmt.Fprintf(&sb, "    -> %s/%d: Offset=%d, Epoch=%d, Metadata=%q, PartitionError=%d\n",
							topicResp.Topic,
							partitionResp.Partition,
							partitionResp.Offset,
							partitionResp.LeaderEpoch,
							util.DereferenceString(partitionResp.Metadata),
							partitionResp.ErrorCode); err != nil {

							b.logger.Client.Error().Err(err).Msg("Failed to write group topic partition info")
							return nil
						}
					}
				}
			}

			if _, err := fmt.Fprintln(&sb, "Fetched Offsets Summary:"); err != nil {
				b.logger.Client.Error().Err(err).Msg("Failed to write fetched offsets summary header")
				return nil
			}

			for _, topicResp := range resp.Topics {
				for _, partitionResp := range topicResp.Partitions {
					_, err := fmt.Fprintf(&sb, "  -> %s/%d: Offset=%d, Epoch=%d, Metadata=%q, PartitionError=%d\n",
						topicResp.Topic,
						partitionResp.Partition,
						partitionResp.Offset,
						partitionResp.LeaderEpoch,
						util.DereferenceString(partitionResp.Metadata),
						partitionResp.ErrorCode)
					if err != nil {
						b.logger.Client.Error().Err(err).Msg("Failed to write topic partition info")
						return nil
					}
				}
			}

			b.logger.Client.Info().Msg(sb.String())
			return nil
		}
	}

	return b.setOption("ConsumerGroup", kgo.ConsumerGroup(config.GroupID)) &&
		((config.InstanceID != "" && b.setOption("InstanceID", kgo.InstanceID(config.InstanceID))) || true) &&
		((len(config.Balancers) > 0 && b.setOption("Balancers", kgo.Balancers(config.Balancers...))) || true) &&
		b.setOption("SessionTimeout", kgo.SessionTimeout(config.SessionTimeout)) &&
		b.setOption("RebalanceTimeout", kgo.RebalanceTimeout(config.RebalanceTimeout)) &&
		b.setOption("HeartbeatInterval", kgo.HeartbeatInterval(config.HeartbeatInterval)) &&
		b.setOption("OnPartitionsAssigned", kgo.OnPartitionsAssigned(config.OnPartitionsAssigned)) &&
		b.setOption("OnPartitionsRevoked", kgo.OnPartitionsRevoked(config.OnPartitionsRevoked)) &&
		b.setOption("OnPartitionsLost", kgo.OnPartitionsLost(config.OnPartitionsLost)) &&
		b.setOption("OnOffsetsFetched", kgo.OnOffsetsFetched(config.OnOffsetsFetched)) &&
		((config.BlockRebalanceOnPoll && b.setOption("BlockRebalanceOnPoll", kgo.BlockRebalanceOnPoll())) || true) &&
		((config.DisableAutoCommit && b.setOption("DisableAutoCommit", kgo.DisableAutoCommit())) || true) &&
		((config.GreedyAutoCommit && b.setOption("GreedyAutoCommit", kgo.GreedyAutoCommit())) || true) &&
		((config.AutoCommitMarks && b.setOption("AutoCommitMarks", kgo.AutoCommitMarks())) || true) &&
		b.setOption("AutoCommitInterval", kgo.AutoCommitInterval(config.AutoCommitInterval)) &&
		((config.AutoCommitCallback != nil && b.setOption("AutoCommitCallback", kgo.AutoCommitCallback(config.AutoCommitCallback))) || true)
}

func (b *ConfigurationBuilder) applyDefaultsAndValidate(config any) bool {
	if b.err != nil {
		return false
	}

	valueOfConfig := reflect.ValueOf(config)
	if valueOfConfig.Kind() != reflect.Ptr || valueOfConfig.IsNil() || valueOfConfig.Elem().Kind() != reflect.Struct {
		b.err = oops.
			In(util.GetFunctionName()).
			Code(perr.EINVAL).
			Errorf("configuration must be a non-nil pointer to a struct: given %s", valueOfConfig.Kind().String())
		return false
	}

	if err := defaults.Set(config); err != nil {
		b.err = oops.
			In(util.GetFunctionName()).
			Code(perr.ECONFIG).
			Wrapf(err, "failed to set defaults for %s", valueOfConfig.Elem().Type().Name())
		return false
	}

	if err := validation.Instance.Struct(config); err != nil {
		b.err = oops.
			In(util.GetFunctionName()).
			Code(perr.ECONFIG).
			Wrapf(err, "failed to validate %s", valueOfConfig.Elem().Type().Name())
		return false
	}

	return true
}

func (b *ConfigurationBuilder) setOption(key string, opt kgo.Opt) bool {
	if _, exists := b.options[key]; exists {
		b.err = oops.
			In(util.GetFunctionName()).
			Code(perr.ECONFIG).
			Errorf("option with key %s already exists", key)
		return false
	}
	b.options[key] = opt
	return true
}

func (b *ConfigurationBuilder) getOptions() ([]kgo.Opt, error) {
	if b.err != nil {
		return nil, b.err
	}

	for _, reqKey := range b.required {
		if _, exists := b.options[reqKey]; !exists {
			b.err = oops.
				In(util.GetFunctionName()).
				Code(perr.ENOENT).
				Hint("Ensure all required configuration options are set before retrieving options").
				Errorf("missing required configuration option '%s'", reqKey)
			return nil, b.err
		}
	}

	opts := make([]kgo.Opt, 0, len(b.options))
	for _, opt := range b.options {
		opts = append(opts, opt)
	}
	return opts, nil
}
