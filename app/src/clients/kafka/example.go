package kafka

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Constants for the test
const (
	// Assuming Kafka is running on the default port
	topicName  = "group-inbox"
	partitions = 4
)

type ScratchConfig struct {
	Username string
	Password string
	Logger   zerolog.Logger
}

func newClient(cfg ScratchConfig) (*Client, error) {
	var tlsConfig *tls.Config

	consumeStartOffset := kgo.NewOffset().AtEnd()
	consumeResetOffset := kgo.NewOffset().AtEnd()
	partitioner := kgo.ManualPartitioner()

	builder := NewConfigurationBuilder(&ConfigurationLoggers{
		Client: cfg.Logger,
		Driver: cfg.Logger,
	})
	_ = builder.SetGeneralConfig(&GeneralConfig{
		ClientID:       "exampleclientid",
		ServiceName:    "chatapp",
		ServiceVersion: "v1.0.0",
		SeedBrokers:    []string{"kafka1:9092", "kafka2:9092", "kafka3:9092"},
		TLSConfig:      tlsConfig,
		Username:       cfg.Username,
		Password:       cfg.Password,
	}) &&
		builder.SetProducerConfig(&ProducerConfig{
			RecordPartitioner: &partitioner,
		}) &&
		builder.SetConsumerConfig(&ConsumerConfig{
			ConsumeStartOffset: &consumeStartOffset,
			ConsumeResetOffset: &consumeResetOffset,
		})

	return NewClient(builder)
}

func OrchestrateKafkaTest(logger *zerolog.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := newClient(ScratchConfig{
		Username: "chat_admin",
		Password: "eP7-KlADI-hWmWeGcEQC",
		Logger:   logger.With().Logger().Level(zerolog.InfoLevel),
	})
	if err != nil {
		return err
	}

	adminClient := kadm.NewClient(client.Driver)
	defer adminClient.Close()

	client, err = newClient(ScratchConfig{
		Username: "chat_prod_cons",
		Password: "cT5_mevs6MCdzo19H3xn",
		Logger:   logger.With().Logger().Level(zerolog.InfoLevel),
	})
	if err != nil {
		return err
	}

	chatClient := client
	defer chatClient.Stop(context.Background())

	if err := createTopic(ctx, logger, adminClient); err != nil {
		return fmt.Errorf("setup failed because topic wasn't created: %v", err)
	}
	logger.Info().Msgf("Setup complete. Topic %s created with %d partitions.", topicName, partitions)

	runDynamicSubscriptionTest(ctx, logger, chatClient.Driver)
	logger.Info().Msg("Test finished successfully.")
	return nil
}

func createTopic(ctx context.Context, logger *zerolog.Logger, adm *kadm.Client) error {
	logger.Info().Msgf("Attempting to delete and re-create topic %s...", topicName)

	// Clean up previous runs
	_, err := adm.DeleteTopics(ctx, topicName)
	if err != nil {
		logger.Warn().Msgf("Note: Failed to delete topic (may not exist): %v", err)
	}

	// Create the new topic
	resp, err := adm.CreateTopic(ctx, partitions, 3, nil, topicName)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}
	if resp.Err != nil {
		return fmt.Errorf("broker error creating topic %s: %w", topicName, resp.Err)
	}
	return nil
}

func runDynamicSubscriptionTest(ctx context.Context, logger *zerolog.Logger, chatClient *kgo.Client) {
	// --- Helper functions for clarity ---

	// Produces one message to each of the 4 partitions
	produceMessages := func(run int) {
		logger.Info().Msgf("\n--- Run %d: PRODUCER: Writing 4 messages (1 per partition) ---", run)
		for p := int32(0); p < partitions; p++ {
			value := fmt.Sprintf("Run %d Message for P%d", run, p)
			key := fmt.Sprintf("Key-%d", p)

			record := &kgo.Record{
				Topic:     topicName,
				Partition: p,
				Key:       []byte(key),
				Value:     []byte(value),
			}

			// Use a blocking ProduceSync for reliable testing, wait for result
			if res := chatClient.ProduceSync(ctx, record); res.FirstErr() != nil {
				logger.Error().Msgf("Failed to produce record to P%d with value '%s'. Broker Error: %v",
					p, value, res.FirstErr(),
				)
				continue
			}
			logger.Info().Msgf("  -> Produced: %s to P%d", value, p)
		}
	}

	// Consumes up to maxMessages and reports the count
	consumeMessages := func(expectedCount int, expectedPartitions map[int32]struct{}) int {
		logger.Info().Msgf("--- CONSUMER: Polling for up to %d messages ---", expectedCount)

		// Poll in a loop until the expected number of messages are collected or a timeout occurs
		ctxTimeout, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		consumedCount := 0
		consumedPartitions := make([]int32, 0, len(expectedPartitions))
		for consumedCount < expectedCount {
			fetches := chatClient.PollRecords(ctxTimeout, 100)
			if fetches.Err() != nil {
				logger.Error().Msgf("Error during consumption: %v", fetches.Err())
				break
			}

			fetches.EachRecord(func(r *kgo.Record) {
				logger.Info().Msgf("  <- Consumed: '%s' from P%d (Offset: %d)", string(r.Value), r.Partition, r.Offset)
				consumedPartitions = append(consumedPartitions, r.Partition)
				consumedCount++
			})
		}

		if consumedCount != expectedCount {
			logger.Warn().Msgf("  WARNING: Expected %d messages, but consumed %d.", expectedCount, consumedCount)
		} else {
			logger.Info().Msgf("  SUCCESS: Consumed expected %d messages.", consumedCount)
		}

		for _, p := range consumedPartitions {
			if _, exists := expectedPartitions[p]; !exists {
				logger.Warn().Msgf("  WARNING: Consumed message from unexpected partition P%d.", p)
			}
		}

		// Since this is a non-group consumer, we do not commit offsets to Kafka.
		// The next consumption will start based on the offset provided in
		// cl.AddConsumePartitions (which is currently kgo.End).
		return consumedCount
	}

	// --- Test Flow ---

	// ** Step 1: Initial Subscription (P1, P2) and Production **
	logger.Info().Msg("\n=== PHASE 1: Subscribing to P1 and P2 only ===")
	// Note: We use AddConsumePartitions to start consumption explicitly on partitions 1 and 2
	chatClient.AddConsumePartitions(map[string]map[int32]kgo.Offset{
		topicName: {
			1: kgo.NewOffset().AtEnd(),
			2: kgo.NewOffset().AtEnd(),
		},
	})

	// Wait briefly for the client to register the subscription
	time.Sleep(500 * time.Millisecond)

	produceMessages(1)
	time.Sleep(500 * time.Millisecond) // Wait for messages to be available

	// ** Step 2: Consumption 1 (Expect 2 messages from P1, P2) **
	// Messages produced to P0 and P3 will not be consumed yet.
	consumeMessages(2, map[int32]struct{}{1: {}, 2: {}})

	// ** Step 3: Dynamic Addition of partitions (P3, P4) and Production **
	// IMPORTANT: Set P3 and P4 to start consuming from the LATEST offset (End)
	// This means they will NOT consume the messages produced in Run 1.
	logger.Info().Msg("\n=== PHASE 2: Adding P0 and P3 from LATEST offset ===")
	chatClient.AddConsumePartitions(map[string]map[int32]kgo.Offset{
		topicName: {
			0: kgo.NewOffset().AtEnd(),
			3: kgo.NewOffset().AtEnd(),
		},
	})

	// Wait briefly for the client to register the subscription
	time.Sleep(500 * time.Millisecond)

	produceMessages(2)
	time.Sleep(500 * time.Millisecond)

	// ** Step 4: Consumption 2 (Expect 4 messages) **
	// P1, P2 consume Run 2 messages. P0, P3 consume Run 2 messages.
	consumeMessages(4, map[int32]struct{}{0: {}, 1: {}, 2: {}, 3: {}})

	// ** Step 5: Dynamic Removal of partitions (P1, P2) and Production **
	logger.Info().Msg("\n=== PHASE 3: Unsubscribing from P1 and P2 ===")
	// Remove the partitions from the subscription list
	chatClient.RemoveConsumePartitions(map[string][]int32{
		topicName: {1, 2},
	})

	time.Sleep(500 * time.Millisecond)

	produceMessages(3)
	time.Sleep(500 * time.Millisecond)

	// ** Step 6: Consumption 3 (Expect 2 messages from P0, P3) **
	// Messages produced to P1 and P2 will be ignored.
	consumeMessages(2, map[int32]struct{}{0: {}, 3: {}})

	// ** Step 7: Dynamic Removal of partitions (P0, P3) and Production **
	logger.Info().Msg("\n=== PHASE 4: Unsubscribing from P0 and P3 (Consumer stops all consumption) ===")
	// Remove the remaining partitions
	chatClient.RemoveConsumePartitions(map[string][]int32{
		topicName: {0, 3},
	})

	time.Sleep(500 * time.Millisecond)

	produceMessages(4)
	time.Sleep(500 * time.Millisecond)

	// ** Step 8: Consumption 4 (Expect 0 messages) **
	consumeMessages(0, map[int32]struct{}{})

	// Final status check
	logger.Info().Msg("\nAll dynamic subscription and consumption tests completed.")
}
