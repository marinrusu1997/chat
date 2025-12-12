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
	TLS      *tls.Config
	Logger   zerolog.Logger
}

func newClient(cfg ScratchConfig) (*Client, error) {
	builder := NewConfigurationBuilder(&ConfigurationLoggers{
		Client: cfg.Logger,
		Driver: cfg.Logger,
	})
	_ = builder.SetGeneralConfig(&GeneralConfig{
		ClientID:       "exampleclientid",
		ServiceName:    "chatapp",
		ServiceVersion: "v1.0.0",
		SeedBrokers:    []string{"kafka1:9092", "kafka2:9092", "kafka3:9092"},
		TLSConfig:      cfg.TLS,
		Username:       cfg.Username,
		Password:       cfg.Password,
	}) &&
		builder.SetConsumerGroupConfig(&ConsumerGroupConfig{
			GroupID:         "chat-sample-group-id",
			Balancers:       []kgo.GroupBalancer{kgo.CooperativeStickyBalancer()},
			AutoCommitMarks: true,
		})

	client, error := NewClient(builder)
	if error != nil {
		return nil, error
	}

	if err := client.Start(context.Background()); err != nil {
		return nil, err
	}

	return client, nil
}

func OrchestrateKafkaTest(logger *zerolog.Logger, tlsConfig *tls.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	underlyingAdminclient, err := newClient(ScratchConfig{
		Username: "chat_admin",
		Password: "eP7-KlADI-hWmWeGcEQC",
		TLS:      tlsConfig,
		Logger:   logger.With().Logger().Level(zerolog.InfoLevel),
	})
	if err != nil {
		return err
	}
	adminClient := kadm.NewClient(underlyingAdminclient.Driver)
	defer underlyingAdminclient.Stop(context.Background())

	chatClient, err := newClient(ScratchConfig{
		Username: "chat_prod_cons",
		Password: "cT5_mevs6MCdzo19H3xn",
		TLS:      tlsConfig,
		Logger:   logger.With().Logger().Level(zerolog.InfoLevel),
	})
	if err != nil {
		return err
	}
	defer chatClient.Stop(context.Background())

	if err := createTopic(ctx, logger, adminClient); err != nil {
		return fmt.Errorf("setup failed because topic wasn't created: %v", err)
	}
	logger.Info().Msgf("Setup complete. Topic %s created with %d partitions.", topicName, partitions)

	runDynamicSubscriptionTest(ctx, logger, chatClient)
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

func runDynamicSubscriptionTest(ctx context.Context, logger *zerolog.Logger, chatClient *Client) {
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
			if res := chatClient.Driver.ProduceSync(ctx, record); res.FirstErr() != nil {
				logger.Error().Msgf("Failed to produce record to P%d with value '%s'. Broker Error: %v",
					p, value, res.FirstErr(),
				)
				continue
			}
			logger.Info().Msgf("  -> Produced: %s to P%d", value, p)
		}
	}

	router := NewConsumptionRouter(&ConsumptionRouterOptions{
		Client:               chatClient,
		TimeoutEstimator:     NewTimeoutEstimator(300, 0.7, 300*time.Millisecond, 3000*time.Millisecond),
		PartitionParallelism: 100,
		Logger:               logger,
	})
	router.Handle(topicName, func(topic string, partition int32, records []*kgo.Record) {
		for _, r := range records {
			logger.Info().Msgf("  <- Consumed: '%s' from P%d (Offset: %d)", string(r.Value), r.Partition, r.Offset)
		}
	})
	chatClient.Driver.PauseFetchTopics(topicName)

	ctx, cancel := context.WithCancel(ctx)
	go router.Run(ctx)
	defer cancel()

	// --- Test Flow ---

	produceMessages(1)
	time.Sleep(1000 * time.Millisecond) // Wait for messages to be available

	produceMessages(2)
	time.Sleep(1000 * time.Millisecond)

	produceMessages(3)
	time.Sleep(1000 * time.Millisecond)

	// Final status check
	select {
	case <-time.After(30 * time.Second):
	}
	logger.Info().Msg("\nAll dynamic subscription and consumption tests completed.")
}
