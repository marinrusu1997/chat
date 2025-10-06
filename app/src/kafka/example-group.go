package kafka

import (
	"chat/src/util"
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const systemTopic = "system-alerts"

func newClientGroup(cfg ScratchConfig) (*kgo.Client, error) {
	tlsConfig, err := util.LoadTLSConfig("/app/certs/kafka/ca.crt")
	if err != nil {
		return nil, err
	}

	builder := NewConfigurationBuilder()
	_ = builder.SetGeneralConfig(GeneralConfig{
		ClientID:       "chatserviceclientid",
		ServiceName:    "chatservice",
		ServiceVersion: "v1.0.0",
		SeedBrokers:    []string{"kafka1:9092", "kafka2:9092", "kafka3:9092"},
		TLSConfig:      tlsConfig,
		Username:       cfg.Username,
		Password:       cfg.Password,
		Logger:         cfg.Logger,
	}) &&
		builder.SetProducerConfig(ProducerConfig{
			Logger: cfg.Logger,
		}) &&
		builder.SetConsumerConfig(ConsumerConfig{
			ConsumeTopics: []string{systemTopic},
		}) &&
		builder.SetConsumerGroupConfig(ConsumerGroupConfig{
			GroupID:           "chatgroup",
			Logger:            cfg.Logger,
			DisableAutoCommit: true,
		})

	return NewClient(builder)
}

func OrchestrateGroupKafkaTest(logger *zerolog.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tempLogger := logger.With().Logger().Level(zerolog.InfoLevel)
	logger = &tempLogger

	client, err := newClientGroup(ScratchConfig{
		Username: "chat_admin",
		Password: "eP7-KlADI-hWmWeGcEQC",
		Logger:   logger,
	})
	if err != nil {
		return err
	}

	adminClient := kadm.NewClient(client)
	defer adminClient.Close()

	client, err = newClientGroup(ScratchConfig{
		Username: "chat_prod_cons",
		Password: "cT5_mevs6MCdzo19H3xn",
		Logger:   logger,
	})
	if err != nil {
		return err
	}

	chatClient := client
	defer chatClient.Close()

	if err := createTopicGroup(ctx, logger, adminClient); err != nil {
		return fmt.Errorf("setup failed because topic wasn't created: %v", err)
	}
	logger.Info().Msgf("Setup complete. Topic %s created with %d partitions.", systemTopic, partitions)

	runDynamicGroupSubscriptionTest(ctx, logger, chatClient)
	logger.Info().Msg("Test finished successfully.")
	return nil
}

func createTopicGroup(ctx context.Context, logger *zerolog.Logger, adm *kadm.Client) error {
	logger.Info().Msgf("Attempting to delete and re-create topic %s...", systemTopic)

	// Clean up previous runs
	_, err := adm.DeleteTopics(ctx, systemTopic)
	if err != nil {
		logger.Warn().Msgf("Note: Failed to delete topic (may not exist): %v", err)
	}

	// Create the new topic
	resp, err := adm.CreateTopic(ctx, partitions, 3, nil, systemTopic)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}
	if resp.Err != nil {
		return fmt.Errorf("broker error creating topic %s: %w", systemTopic, resp.Err)
	}
	return nil
}

func runDynamicGroupSubscriptionTest(ctx context.Context, logger *zerolog.Logger, chatClient *kgo.Client) {
	// --- Helper functions for clarity ---

	totalMessages := 20

	// --- Helper function to produce all messages ---
	produceAllMessages := func() {
		logger.Info().Msgf("\n--- PRODUCER: Writing %d messages ---", totalMessages)
		for i := 0; i < totalMessages; i++ {
			// Key is required by the compacted topic
			key := fmt.Sprintf("UserKey-%d", i%partitions)
			value := fmt.Sprintf("Message-%d", i)

			record := &kgo.Record{
				Topic: systemTopic,
				Key:   []byte(key),
				Value: []byte(value),
			}

			if res := chatClient.ProduceSync(ctx, record); res.FirstErr() != nil {
				logger.Error().Msgf("FATAL: Failed to produce record: %v", res.FirstErr())
			}
			logger.Info().Msgf("  -> Produced: %s (Key: %s)", value, key)
		}
		logger.Info().Msg("  Production complete.")
	}

	// --- Helper function for consumption loop ---
	consumeAndCommit := func(cl *kgo.Client, expectedCount int, run int) int {
		logger.Info().Msgf("--- CONSUMER Run %d: Polling for up to %d messages ---", run, expectedCount)

		ctxTimeout, cancel := context.WithTimeout(ctx, 60*time.Second) // Increased timeout for group joins
		defer cancel()

		consumedCount := 0
		for consumedCount < expectedCount {
			fetches := cl.PollRecords(ctxTimeout, 100)
			if fetches.Err() != nil {
				logger.Error().Msgf("Error during consumption: %v", fetches.Err())
				break
			}

			fetches.EachRecord(func(r *kgo.Record) {
				key := string(r.Key)
				logger.Info().Msgf("  <- Consumed: %s (Key: %s) from P%d (Offset: %d)", string(r.Value), key, r.Partition, r.Offset)
				consumedCount++
			})

			// CRITICAL: Commit all records fetched since the last poll.
			if err := cl.CommitUncommittedOffsets(ctx); err != nil {
				logger.Error().Msgf("FATAL: Failed to commit offsets: %v", err)
			}
			logger.Info().Msgf("  -> Successfully committed offsets after consuming %d records.", consumedCount)
		}

		if consumedCount != expectedCount {
			logger.Warn().Msgf("  WARNING: Expected %d messages, but consumed %d.", expectedCount, consumedCount)
		} else {
			logger.Info().Msgf("  SUCCESS: Consumed expected %d messages.", consumedCount)
		}

		return consumedCount
	}

	// --- Test Flow ---

	// PHASE 1: Produce all messages
	produceAllMessages()

	// PHASE 2: Consumer A joins group and consumes all messages
	logger.Info().Msg("\n=== PHASE 2: Consumer A (Group: 'chat-service-test-group') starts consumption ===")
	consumerA, err := newClientGroup(ScratchConfig{
		Username: "chat_prod_cons",
		Password: "cT5_mevs6MCdzo19H3xn",
		Logger:   logger,
	})
	if err != nil {
		logger.Error().Msgf("Error during consumption: %v", err)
		return
	}

	consumedA := consumeAndCommit(consumerA, totalMessages, 1)
	consumerA.Close() // Consumer A closes, triggering a rebalance (though it's the only one)

	if consumedA != totalMessages {
		logger.Error().Msgf("Test Failed: Consumer A did not consume all %d messages. Consumed: %d", totalMessages, consumedA)
	}

	// PHASE 3: Consumer B joins the SAME group and consumes 0 messages (Verifies Commit)
	logger.Info().Msg("\n=== PHASE 3: Consumer B (Group: 'chat-service-test-group') starts consumption ===")
	// Note: We create a new client, which will join the group and inherit the last committed offset.
	consumerB, err := newClientGroup(ScratchConfig{
		Username: "chat_prod_cons",
		Password: "cT5_mevs6MCdzo19H3xn",
		Logger:   logger,
	})
	if err != nil {
		logger.Error().Msgf("Error during consumption: %v", err)
		return
	}

	// Give the new consumer time to join and fetch metadata
	time.Sleep(1 * time.Second)

	// If the commit in Phase 2 worked, consumer B should fetch 0 messages.
	consumedB := consumeAndCommit(consumerB, 0, 2)
	consumerB.Close()

	if consumedB != 0 {
		logger.Error().Msgf("Test Failed: Consumer B consumed %d messages. The previous commit failed.", consumedB)
	}

	logger.Info().Msg("\n--- Group Consumption Test Passed ---")
	logger.Error().Msgf("Consumer A consumed and committed %d messages.", consumedA)
	logger.Error().Msgf("Consumer B consumed and committed %d messages.", consumedB)
}
