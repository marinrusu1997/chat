package kafka

import (
	"chat/src/util"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/plugin/kzerolog"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"
)

var validatorG *validator.Validate

func init() {
	validatorG = validator.New(validator.WithRequiredStructEnabled())

	err := validatorG.RegisterValidation("host_port_list", util.ValidateHostPortList)
	if err != nil {
		panic(fmt.Sprintf("Failed to register custom validator 'host_port_list': %v", err))
	}

	err = validatorG.RegisterValidation("not_blank", util.ValidateNotBlank)
	if err != nil {
		panic(fmt.Sprintf("Failed to register custom validator 'not_blank': %v", err))
	}

	err = validatorG.RegisterValidation("enum", util.ValidateEnum)
	if err != nil {
		panic(fmt.Sprintf("Failed to register custom validator 'enum': %v", err))
	}

	err = validatorG.RegisterValidation("unique", util.ValidateUnique)
	if err != nil {
		panic(fmt.Sprintf("Failed to register custom validator 'unique': %v", err))
	}
}

type GeneralConfig struct {
	ClientID               string          `validate:"not_blank,alphanum,min=10,max=50"`
	ServiceName            string          `validate:"not_blank,alphanum,min=5,max=50"`
	ServiceVersion         string          `validate:"not_blank,min=1,max=30"`
	SeedBrokers            []string        `validate:"required,min=1,max=10,host_port_list,unique"`
	TLSConfig              *tls.Config     ``
	Username               string          `validate:"required_with=Password,not_blank,min=5,max=50"`
	Password               string          `validate:"required_with=Username,not_blank,min=5,max=50"`
	RequestTimeoutOverhead time.Duration   `validate:"min=1000000000,max=15000000000" default:"5s"` // [1s, 15s], default 5s
	Logger                 *zerolog.Logger `validate:"required"`
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
	Logger                *zerolog.Logger        `validate:"required"`
}

type TransactionConfig struct {
	ID                            string          `validate:"not_blank,alphanum,min=10,max=50"`
	Timeout                       time.Duration   `validate:"required,gte=5000000000,lte=300000000000"`      // [5s, 5min]
	SessionTimeout                time.Duration   `validate:"gte=6000000000,lte=360000000000" default:"45s"` // [6s, 6min]
	RequireStableFetchOffsets     bool            ``
	ConcurrentTransactionsBackoff time.Duration   `validate:"required,gte=10000000,lte=100000000" default:"50ms"` // [10ms, 100ms]
	Logger                        *zerolog.Logger `validate:"required"`
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
	GroupID              string                                                                          `validate:"not_blank,min=5,max=50"`
	InstanceID           string                                                                          `validate:"omitempty,min=5,max=50"`
	Balancers            []kgo.GroupBalancer                                                             ``
	SessionTimeout       time.Duration                                                                   `validate:"gte=10000000000,lte=600000000000" default:"60s"` // [10s, 10min], default 60s
	RebalanceTimeout     time.Duration                                                                   `validate:"gte=10000000000,lte=60000000000" default:"60s"`  // [10s, 1min], default 60s
	HeartbeatInterval    time.Duration                                                                   `validate:"gte=5000000000,lte=15000000000" default:"5s"`    // [5s, 15s], default 5s
	OnPartitionsRevoked  func(context.Context, *kgo.Client, map[string][]int32)                          ``
	OnPartitionsAssigned func(context.Context, *kgo.Client, map[string][]int32)                          ``
	OnPartitionsLost     func(context.Context, *kgo.Client, map[string][]int32)                          ``
	OnOffsetsFetched     func(context.Context, *kgo.Client, *kmsg.OffsetFetchResponse) error             ``
	BlockRebalanceOnPoll bool                                                                            ``
	DisableAutoCommit    bool                                                                            ``
	GreedyAutoCommit     bool                                                                            ``
	AutoCommitMarks      bool                                                                            ``
	AutoCommitInterval   time.Duration                                                                   `validate:"gte=100000000,lte=10000000000" default:"1s"` // [100ms, 10s], default 1s
	AutoCommitCallback   func(*kgo.Client, *kmsg.OffsetCommitRequest, *kmsg.OffsetCommitResponse, error) ``
	Logger               *zerolog.Logger                                                                 `validate:"required"`
}

type ConfigurationBuilder struct {
	options map[string]kgo.Opt
	error   error
}

var requiredConfigurationOptions = [...]string{"ClientID"}

func NewConfigurationBuilder() ConfigurationBuilder {
	return ConfigurationBuilder{
		options: make(map[string]kgo.Opt),
		error:   nil,
	}
}

func (builder *ConfigurationBuilder) GetOptions() ([]kgo.Opt, error) {
	if builder.error != nil {
		return nil, builder.error
	}

	for _, reqKey := range requiredConfigurationOptions {
		if _, exists := builder.options[reqKey]; !exists {
			builder.error = fmt.Errorf("missing required configuration option '%s' in %s", reqKey, util.GetFunctionName(1))
			return nil, builder.error
		}
	}

	opts := make([]kgo.Opt, 0, len(builder.options))
	for _, opt := range builder.options {
		opts = append(opts, opt)
	}
	return opts, nil
}

func (builder *ConfigurationBuilder) SetGeneralConfig(config GeneralConfig) bool {
	if !builder.applyDefaultsAndValidate(&config) {
		return false
	}

	return builder.setOption("ClientID", kgo.ClientID(config.ClientID)) &&
		builder.setOption("DialTimeout", kgo.DialTimeout(5*time.Second)) &&
		((config.TLSConfig != nil && builder.setOption("DialTLSConfig", kgo.DialTLSConfig(config.TLSConfig))) || true) &&
		builder.setOption("RequestTimeoutOverhead", kgo.RequestTimeoutOverhead(config.RequestTimeoutOverhead)) &&
		builder.setOption("ConnIdleTimeout", kgo.ConnIdleTimeout(10*time.Minute)) &&
		builder.setOption("SoftwareNameAndVersion", kgo.SoftwareNameAndVersion(config.ServiceName, config.ServiceVersion)) &&
		builder.setOption("WithLogger", kgo.WithLogger(kzerolog.New(config.Logger))) &&
		builder.setOption("SeedBrokers", kgo.SeedBrokers(config.SeedBrokers...)) &&
		builder.setOption("RetryBackoffFn", kgo.RetryBackoffFn(func(attempts int) time.Duration {
			// Start at 100ms and double up to a max of 5s
			baseDelay := 100 * time.Millisecond
			maxDelay := 5 * time.Second

			// Calculate 2^attempts (clamped)
			delay := time.Duration(baseDelay.Nanoseconds() * int64(math.Pow(2, float64(attempts))))
			if delay > maxDelay {
				delay = maxDelay
			}

			// Add jitter (e.g., +/- 20% randomness)
			jitter := time.Duration(rand.Float64() * float64(delay.Nanoseconds()) * 0.4) // 40% range
			delay = delay - (delay / 5) + jitter                                         // Apply -20% offset and add jitter up to +20%

			return delay
		})) &&
		builder.setOption("RetryTimeout", kgo.RetryTimeout(30*time.Second)) &&
		builder.setOption("RetryTimeoutFn", kgo.RetryTimeoutFn(func(req int16) time.Duration {
			switch kmsg.Key(req) {

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
		builder.setOption("MetadataMaxAge", kgo.MetadataMaxAge(3*time.Minute)) &&
		builder.setOption("MetadataMinAge", kgo.MetadataMinAge(7*time.Second)) &&
		((config.Username != "" && config.Password != "" && builder.setOption("SASL", kgo.SASL(plain.Auth{
			User: config.Username,
			Pass: config.Password,
		}.AsMechanism()))) || true)
}

func (builder *ConfigurationBuilder) SetProducerConfig(config ProducerConfig) bool {
	if !builder.applyDefaultsAndValidate(&config) {
		return false
	}
	if config.RequestTimeout*time.Duration(config.RecordRetries) > config.RecordDeliveryTimeout {
		builder.error = fmt.Errorf(
			"ProducerConfig: RecordDeliveryTimeout (%s) must be greater than RequestTimeout (%s) * RecordRetries (%d) = %s",
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

	return builder.setOption("BrokerMaxWriteBytes", kgo.BrokerMaxWriteBytes(config.MaxWriteBytes)) &&
		builder.setOption("RequiredAcks", kgo.RequiredAcks(*config.RequiredAcks)) &&
		builder.setOption("ProducerBatchCompression", kgo.ProducerBatchCompression(config.BatchCompression...)) &&
		builder.setOption("ProducerBatchMaxBytes", kgo.ProducerBatchMaxBytes(config.BatchMaxBytes)) &&
		builder.setOption("MaxBufferedRecords", kgo.MaxBufferedRecords(config.MaxBufferedRecords)) &&
		builder.setOption("MaxBufferedBytes", kgo.MaxBufferedBytes(config.MaxBufferedBytes)) &&
		builder.setOption("ProduceRequestTimeout", kgo.ProduceRequestTimeout(config.RequestTimeout)) &&
		builder.setOption("RecordRetries", kgo.RecordRetries(config.RecordRetries)) &&
		builder.setOption("UnknownTopicRetries", kgo.UnknownTopicRetries(config.UnknownTopicRetries)) &&
		((config.ProducerLinger > 0 && builder.setOption("ProducerLinger", kgo.ProducerLinger(config.ProducerLinger))) || true) &&
		builder.setOption("RecordDeliveryTimeout", kgo.RecordDeliveryTimeout(config.RecordDeliveryTimeout)) &&
		builder.setOption("ConsiderMissingTopicDeletedAfter", kgo.ConsiderMissingTopicDeletedAfter(20*time.Second)) &&
		builder.setOption("RecordPartitioner", kgo.RecordPartitioner(*config.RecordPartitioner)) &&
		builder.setOption("ProducerOnDataLossDetected", kgo.ProducerOnDataLossDetected(func(topic string, partition int32) {
			// CRITICAL: Log this event and send an alert (e.g., to PagerDuty or Slack)
			config.Logger.Printf("!!! CRITICAL KAFKA PRODUCER DATA LOSS DETECTED !!! Topic: %s, Partition: %d. Producer is CONTINUING.", topic, partition)
			// We would also need call an alerting service here:
			// AlertService.Trigger("Kafka Data Loss", fmt.Sprintf("Topic: %s, Partition: %d", topic, partition))
		}))
}

func (builder *ConfigurationBuilder) SetTransactionConfig(config TransactionConfig) bool {
	if !builder.applyDefaultsAndValidate(&config) {
		return false
	}

	if !config.RequireStableFetchOffsets {
		if config.Timeout <= config.SessionTimeout {
			config.Logger.Warn().Msgf(
				"Transaction timeout (%s) should be greater than the group session timeout (%s) when "+
					"RequireStableFetchOffsets is false. This will likely lead to duplicate messages in failure scenarios.",
				config.Timeout, config.SessionTimeout,
			)
		}
	} else {
		if config.Timeout >= 10*time.Second {
			config.Logger.Warn().Msgf(
				"With RequireStableFetchOffsets enabled, it is recommended to set a low transaction timeout "+
					"(e.g., 10s) to prevent long blocking periods. Current timeout is %s.",
				config.Timeout,
			)
		}
	}

	return builder.setOption("TransactionalID", kgo.TransactionalID(config.ID)) &&
		builder.setOption("TransactionTimeout", kgo.TransactionTimeout(config.Timeout)) &&
		builder.setOption("SessionTimeout", kgo.SessionTimeout(config.SessionTimeout)) &&
		builder.setOption("ConcurrentTransactionsBackoff", kgo.ConcurrentTransactionsBackoff(config.ConcurrentTransactionsBackoff)) &&
		((config.RequireStableFetchOffsets && builder.setOption("RequireStableFetchOffsets", kgo.RequireStableFetchOffsets())) || true)
}

func (builder *ConfigurationBuilder) SetConsumerConfig(config ConsumerConfig) bool {
	if !builder.applyDefaultsAndValidate(&config) {
		return false
	}

	partitionsGiven := config.ConsumePartitions != nil && len(config.ConsumePartitions) > 0
	topicsGiven := len(config.ConsumeTopics) > 0
	if topicsGiven && partitionsGiven {
		builder.error = fmt.Errorf("invalid configuration: cannot provide both ConsumeTopics and ConsumePartitions; only one is allowed")
		return false
	}
	if partitionsGiven && config.RegexConsumption {
		builder.error = fmt.Errorf("invalid configuration: RegexConsumption must be false when ConsumePartitions is provided")
		return false
	}

	return builder.setOption("BrokerMaxReadBytes", kgo.BrokerMaxReadBytes(config.MaxReadBytes)) &&
		builder.setOption("FetchMaxWait", kgo.FetchMaxWait(config.FetchMaxWait)) &&
		builder.setOption("FetchMinBytes", kgo.FetchMinBytes(config.FetchMinBytes)) &&
		builder.setOption("FetchMaxBytes", kgo.FetchMaxBytes(config.FetchMaxBytes)) &&
		builder.setOption("FetchMaxPartitionBytes", kgo.FetchMaxPartitionBytes(config.FetchMaxPartitionBytes)) &&
		((config.ConsumeStartOffset != nil && builder.setOption("ConsumeStartOffset", kgo.ConsumeStartOffset(*config.ConsumeStartOffset))) || true) &&
		((config.ConsumeResetOffset != nil && builder.setOption("ConsumeResetOffset", kgo.ConsumeResetOffset(*config.ConsumeResetOffset))) || true) &&
		((config.FetchIsolationLevel != nil && builder.setOption("FetchIsolationLevel", kgo.FetchIsolationLevel(*config.FetchIsolationLevel))) || true) &&
		((config.ConsumePreferringLagFn != nil && builder.setOption("ConsumePreferringLagFn", kgo.ConsumePreferringLagFn(config.ConsumePreferringLagFn))) || true) &&
		((config.MaxConcurrentFetches > 0 && builder.setOption("MaxConcurrentFetches", kgo.MaxConcurrentFetches(config.MaxConcurrentFetches))) || true) &&
		builder.setOption("RecheckPreferredReplicaInterval", kgo.RecheckPreferredReplicaInterval(20*time.Minute)) &&
		((topicsGiven && builder.setOption("ConsumeTopics", kgo.ConsumeTopics(config.ConsumeTopics...))) || true) &&
		((partitionsGiven && builder.setOption("ConsumePartitions", kgo.ConsumePartitions(config.ConsumePartitions))) || true) &&
		((config.RegexConsumption && builder.setOption("ConsumeRegex", kgo.ConsumeRegex())) || true)
}

func (builder *ConfigurationBuilder) SetConsumerGroupConfig(config ConsumerGroupConfig) bool {
	if !builder.applyDefaultsAndValidate(&config) {
		return false
	}

	if config.OnPartitionsRevoked == nil && config.BlockRebalanceOnPoll == false {
		config.OnPartitionsRevoked = func(ctx context.Context, cl *kgo.Client, revoked map[string][]int32) {
			config.Logger.Warn().Msgf("Partitions revoked: %v", revoked)

			if err := cl.CommitUncommittedOffsets(ctx); err != nil {
				config.Logger.Error().Msgf("Blocking commit in OnPartitionsRevoked failed: %v", err)
			} else {
				config.Logger.Info().Msg("Successfully committed uncommitted offsets before revocation.")
			}
		}
	}
	if config.OnPartitionsAssigned == nil {
		config.OnPartitionsAssigned = func(ctx context.Context, cl *kgo.Client, assigned map[string][]int32) {
			config.Logger.Info().Msgf("Partitions assigned: %v", assigned)
		}
	}
	if config.OnPartitionsLost == nil {
		config.OnPartitionsLost = func(ctx context.Context, cl *kgo.Client, lost map[string][]int32) {
			config.Logger.Error().Msgf("Partitions lost due to unrecoverable group error: %v", lost)
		}
	}
	if config.OnOffsetsFetched == nil {
		config.OnOffsetsFetched = func(ctx context.Context, cl *kgo.Client, resp *kmsg.OffsetFetchResponse) error {
			var sb strings.Builder

			if _, err := fmt.Fprintf(&sb, "Offsets Fetched: Version=%d, Throttle=%dms, TopLevelError=%d\n",
				resp.Version, resp.ThrottleMillis, resp.ErrorCode); err != nil {
				config.Logger.Error().Err(err).Msg("Failed to write offset fetch response header")
				return nil
			}

			for _, groupResp := range resp.Groups {
				if _, err := fmt.Fprintf(&sb, "  Group: %s, GroupError: %d\n", groupResp.Group, groupResp.ErrorCode); err != nil {
					config.Logger.Error().Err(err).Msg("Failed to write group header")
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

							config.Logger.Error().Err(err).Msg("Failed to write group topic partition info")
							return nil
						}
					}
				}
			}

			if _, err := fmt.Fprintln(&sb, "Fetched Offsets Summary:"); err != nil {
				config.Logger.Error().Err(err).Msg("Failed to write fetched offsets summary header")
				return nil
			}

			for _, topicResp := range resp.Topics {
				for _, partitionResp := range topicResp.Partitions {
					if _, err := fmt.Fprintf(&sb, "  -> %s/%d: Offset=%d, Epoch=%d, Metadata=%q, PartitionError=%d\n",
						topicResp.Topic,
						partitionResp.Partition,
						partitionResp.Offset,
						partitionResp.LeaderEpoch,
						util.DereferenceString(partitionResp.Metadata),
						partitionResp.ErrorCode); err != nil {

						config.Logger.Error().Err(err).Msg("Failed to write topic partition info")
						return nil
					}
				}
			}

			config.Logger.Info().Msg(sb.String())
			return nil
		}
	}

	return builder.setOption("ConsumerGroup", kgo.ConsumerGroup(config.GroupID)) &&
		((config.InstanceID != "" && builder.setOption("InstanceID", kgo.InstanceID(config.InstanceID))) || true) &&
		((len(config.Balancers) > 0 && builder.setOption("Balancers", kgo.Balancers(config.Balancers...))) || true) &&
		builder.setOption("SessionTimeout", kgo.SessionTimeout(config.SessionTimeout)) &&
		builder.setOption("RebalanceTimeout", kgo.RebalanceTimeout(config.RebalanceTimeout)) &&
		builder.setOption("HeartbeatInterval", kgo.HeartbeatInterval(config.HeartbeatInterval)) &&
		builder.setOption("OnPartitionsAssigned", kgo.OnPartitionsAssigned(config.OnPartitionsAssigned)) &&
		builder.setOption("OnPartitionsRevoked", kgo.OnPartitionsRevoked(config.OnPartitionsRevoked)) &&
		builder.setOption("OnPartitionsLost", kgo.OnPartitionsLost(config.OnPartitionsLost)) &&
		builder.setOption("OnOffsetsFetched", kgo.OnOffsetsFetched(config.OnOffsetsFetched)) &&
		((config.BlockRebalanceOnPoll && builder.setOption("BlockRebalanceOnPoll", kgo.BlockRebalanceOnPoll())) || true) &&
		((config.DisableAutoCommit && builder.setOption("DisableAutoCommit", kgo.DisableAutoCommit())) || true) &&
		((config.GreedyAutoCommit && builder.setOption("GreedyAutoCommit", kgo.GreedyAutoCommit())) || true) &&
		((config.AutoCommitMarks && builder.setOption("AutoCommitMarks", kgo.AutoCommitMarks())) || true) &&
		builder.setOption("AutoCommitInterval", kgo.AutoCommitInterval(config.AutoCommitInterval)) &&
		((config.AutoCommitCallback != nil && builder.setOption("AutoCommitCallback", kgo.AutoCommitCallback(config.AutoCommitCallback))) || true)
}

func (builder *ConfigurationBuilder) applyDefaultsAndValidate(config interface{}) bool {
	if builder.error != nil {
		return false
	}

	v := reflect.ValueOf(config)
	if v.Kind() != reflect.Ptr || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		builder.error = fmt.Errorf(
			"%s: configuration must be a non-nil pointer to a struct: given %s",
			util.GetFunctionName(1), v.Kind().String(),
		)
		return false
	}

	if err := defaults.Set(config); err != nil {
		typeName := v.Elem().Type().Name()
		builder.error = fmt.Errorf("failed to set %s defaults in %s: %w", typeName, util.GetFunctionName(1), err)
		return false
	}

	if err := validatorG.Struct(config); err != nil {
		typeName := v.Elem().Type().Name()
		builder.error = fmt.Errorf("%s configuration failed validation in %s: %w", typeName, util.GetFunctionName(1), err)
		return false
	}

	return true
}

func (builder *ConfigurationBuilder) setOption(key string, opt kgo.Opt) bool {
	if _, exists := builder.options[key]; exists {
		builder.error = fmt.Errorf("option with key %s already exists in %s", key, util.GetFunctionName(1))
		return false
	}
	builder.options[key] = opt
	return true
}

func NewClient(config ConfigurationBuilder) (*kgo.Client, error) {
	options, err := config.GetOptions()
	if err != nil {
		return nil, fmt.Errorf("can't create a new Kafka client because configuration is broken: %w", err)
	}
	return kgo.NewClient(options...)
}

// @fixme this function needs to be called on process shutdown
func Cleanup(client *kgo.Client) {
	// 1. Final Synchronous Commit
	// Use a context with a timeout (e.g., 5 seconds) for the final commit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.CommitUncommittedOffsets(ctx)
	if err != nil {
		log.Printf("ERROR: Final synchronous commit failed: %v", err) // @fixme use proper logger here
	} else {
		log.Println("Successfully performed final synchronous commit.") // @fixme use proper logger here
	}
	// CommitRecords,CommitMarkedOffsets,CommitOffsetsSync,CommitUncommittedOffsets,CommitOffsets

	// 2. Close the Client
	client.Close()
	log.Println("Kafka client closed.") // @fixme use proper logger here
}
