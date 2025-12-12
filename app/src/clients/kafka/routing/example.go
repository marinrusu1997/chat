package routing

import (
	"chat/src/clients/kafka"
	"context"
	"fmt"
	"sync/atomic"
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

func OrchestrateKafkaTest(logger *zerolog.Logger, adminClient *kafka.Client, dataClient *kafka.Client) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := createTopic(ctx, logger, kadm.NewClient(adminClient.Driver)); err != nil {
		return fmt.Errorf("setup failed because topic wasn't created: %w", err)
	}
	logger.Info().Msgf("Setup complete. Topic %s created with %d partitions.", topicName, partitions)

	runDynamicSubscriptionTest(ctx, logger, dataClient)
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

	logger.Info().Msgf("Created topic %s...", topicName)
	return nil
}

func runDynamicSubscriptionTest(ctx context.Context, logger *zerolog.Logger, chatClient *kafka.Client) {
	// --- Helper functions for clarity ---

	// Produces one message to each of the 4 partitions
	produceMessages := func(run int) {
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
			if res := chatClient.Driver.ProduceSync(ctx, record); res.FirstErr() != nil {
				logger.Error().Msgf("Failed to produce record to P%d with value '%s'. Broker Error: %v",
					p, value, res.FirstErr(),
				)
				continue
			}
		}
	}

	router, err := NewConsumerRouter(&ConsumerRouterOptions{
		Client: chatClient,
		Logger: logger,
	})
	if err != nil {
		panic(err)
	}

	var consumedCount atomic.Int64
	router.OnRecordsFrom(topicName, func(records []*kgo.Record) {
		for _, r := range records {
			logger.Info().Msgf("  <- Consumed: '%s' from T%s P%d (Offset: %d)", string(r.Value), r.Topic, r.Partition, r.Offset)
		}
		consumedCount.Add(int64(len(records)))

		timer := time.NewTimer(4000 * time.Millisecond)
		defer timer.Stop() // emulate long processing
		<-timer.C
	})
	if err := router.Start(); err != nil {
		panic(err)
	}
	defer router.Stop()

	// --- Test Flow ---
	var producedCount int64
	for i := 1; i <= 5; i++ {
		produceMessages(i)
		producedCount += 4
		time.Sleep(1000 * time.Millisecond) // Wait for messages to be available
	}

	// Final status check
	select {
	case <-time.After(10 * time.Second):
	}

	if consumedCount.Load() != producedCount {
		panic(fmt.Errorf("Test failed: Not all produced messages %d were consumed %d", producedCount, consumedCount.Load()))
	}

	logger.Info().Msg("All dynamic subscription and consumption tests completed.")
}
