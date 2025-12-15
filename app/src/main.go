package main

import (
	"chat/src/clients/elasticsearch"
	"chat/src/clients/email"
	"chat/src/clients/etcd"
	"chat/src/clients/kafka"
	"chat/src/clients/kafka/routing"
	"chat/src/clients/nats"
	"chat/src/clients/neo4j"
	"chat/src/clients/postgresql"
	"chat/src/clients/redis"
	"chat/src/clients/scylla"
	emailv1 "chat/src/gen/proto/email/v1"
	"chat/src/platform/config"
	"chat/src/platform/health"
	"chat/src/platform/lifecycle"
	"chat/src/platform/logging"
	"chat/src/platform/security"
	"chat/src/platform/state"
	emailsvc "chat/src/services/email"
	"chat/src/services/presence"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//	@FIXME:	https://github.com/uber-go/guide/tree/master

func main() {
	cfg, err := config.Load(config.LoadConfigOptions{
		YamlFilePaths: []string{"/etc/chat/config.yaml"},
		EnvVarPrefix:  "CHAT_APP_",
	})
	if err != nil {
		panic(fmt.Sprintf("Error loading config: %+v", err))
	}

	loggerFactory, err := logging.NewFactory(&logging.Options{
		AppInstanceID: cfg.Application.InstanceName,
		AppVersion:    cfg.Application.Version,
		AppCommit:     cfg.Application.Commit,
		AppBuildDate:  cfg.Application.BuildTime,
		RootLevel:     cfg.Logging.RootLevel,
		LiteralLevels: cfg.Logging.LiteralLevels,
		RegexLevels:   cfg.Logging.RegexLevels,
		PrettyPrint:   cfg.Logging.PrettyPrint,
	})
	if err != nil {
		panic(fmt.Sprintf("Error creating logger factory: %+v", err))
	}
	logger := loggerFactory.Child("main")

	cfgBytes, err := yaml.Marshal(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to marshal config")
	}
	logger.Info().Msgf("Using config:\n%s", string(cfgBytes))

	// 4. Load TLS configs
	tlsConfigs, err := security.LoadTLSConfigs(&security.TLSConfigSources{
		Global: security.TLSMaterialPaths{
			Truststore:  string(cfg.Application.Truststore),
			Certificate: string(cfg.Application.Certificate),
			Key:         string(cfg.Application.Key),
		},
		Services: map[string]security.TLSServiceOptions{
			elasticsearch.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore: string(cfg.Elasticsearch.Truststore),
				},
			},
			kafka.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore: string(cfg.Kafka.Truststore),
				},
			},
			neo4j.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore:  string(cfg.Neo4j.Truststore),
					Certificate: string(cfg.Neo4j.Certificate),
					Key:         string(cfg.Neo4j.Key),
				},
				Policy: security.TLSPolicy{
					RequireMutualTLS: true,
				},
			},
			etcd.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore:  string(cfg.Etcd.Truststore),
					Certificate: string(cfg.Etcd.Certificate),
					Key:         string(cfg.Etcd.Key),
				},
				Policy: security.TLSPolicy{
					RequireMutualTLS: true,
				},
			},
			postgresql.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore: string(cfg.PostgreSQL.Truststore),
				},
			},
			redis.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore:  string(cfg.Redis.Truststore),
					Certificate: string(cfg.Redis.Certificate),
					Key:         string(cfg.Redis.Key),
				},
				Policy: security.TLSPolicy{
					RequireMutualTLS: true,
				},
			},
			scylla.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore:  string(cfg.ScyllaDB.Truststore),
					Certificate: string(cfg.ScyllaDB.Certificate),
					Key:         string(cfg.ScyllaDB.Key),
				},
				Policy: security.TLSPolicy{
					RequireMutualTLS: true,
				},
			},
			nats.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore:  string(cfg.Nats.Truststore),
					Certificate: string(cfg.Nats.Certificate),
					Key:         string(cfg.Nats.Key),
				},
				Policy: security.TLSPolicy{
					RequireMutualTLS: true,
				},
			},
			email.PingTargetName: {
				Paths: security.TLSMaterialPaths{
					Truststore: string(cfg.Email.Truststore),
				},
			},
		},
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to load tls configs")
	}

	clients, err := state.CreateClients(cfg, tlsConfigs.Services, loggerFactory)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create clients")
	}

	clientsLifecycleController, err := lifecycle.NewController(&lifecycle.ControllerOptions{
		Services: map[string]lifecycle.ServiceLifecycle{
			elasticsearch.PingTargetName: clients.Elasticsearch,
			kafka.AdminClientName:        clients.Kafka.Admin,
			kafka.DataClientName:         clients.Kafka.Data,
			neo4j.PingTargetName:         clients.Neo4j,
			etcd.PingTargetName:          clients.Etcd,
			postgresql.PingTargetName:    clients.PostgreSQL,
			redis.PingTargetName:         clients.Redis,
			scylla.PingTargetName:        clients.ScyllaDB,
			nats.PingTargetName:          clients.Nats,
			email.PingTargetName:         clients.Email,
		},
		Logger: loggerFactory.Child("lifecycle.clients"),
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create clients lifecycle controller")
	}
	if err := clientsLifecycleController.Start(context.Background()); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start clients lifecycle controller")
	}
	defer clientsLifecycleController.Stop(context.Background())

	healthController, err := health.NewController(&health.ControllerConfig{
		Dependencies: map[string]health.Pingable{
			elasticsearch.PingTargetName: clients.Elasticsearch,
			kafka.AdminClientName:        clients.Kafka.Admin,
			kafka.DataClientName:         clients.Kafka.Data,
			neo4j.PingTargetName:         clients.Neo4j,
			etcd.PingTargetName:          clients.Etcd,
			postgresql.PingTargetName:    clients.PostgreSQL,
			redis.PingTargetName:         clients.Redis,
			scylla.PingTargetName:        clients.ScyllaDB,
			nats.PingTargetName:          clients.Nats,
			email.PingTargetName:         clients.Email,
		},
		Logger: loggerFactory.Child("health.controller"),
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create heath controller")
	}
	healthController.Start()
	defer healthController.Stop()

	kafkaConsumerRouter, err := routing.NewConsumerRouter(&routing.ConsumerRouterOptions{
		Client: clients.Kafka.Data,
		Logger: loggerFactory.ChildPtr("kafka.consumer.router"),
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create kafka consumer router")
	}

	services := state.Services{
		Presence: presence.NewService(clients.Redis, clients.Nats, loggerFactory.ChildPtr("services.presence")),
		Email: emailsvc.NewService(&emailsvc.ServiceOptions{
			Clients: emailsvc.ServiceClientsOptions{
				Email: clients.Email,
				Kafka: clients.Kafka.Data,
			},
			EmailBuild: emailsvc.ServiceEmailBuildOptions{
				From:         cfg.Email.From,
				Organization: cfg.Email.Organization,
				UserAgent:    cfg.Email.UserAgent,
				DKIMCert:     &tlsConfigs.Global.Certificates[0],
			},
			KafkaDelivery: emailsvc.ServiceKafkaDeliveryOptions{
				Topic:  cfg.Kafka.Topics.EmailDelivery,
				Router: kafkaConsumerRouter,
			},
			TemplatesLocation: cfg.Email.TemplatesLocation,
			Logger:            loggerFactory.ChildPtr("services.email"),
		}),
	}

	servicesLifecycleController, err := lifecycle.NewController(&lifecycle.ControllerOptions{
		Services: map[string]lifecycle.ServiceLifecycle{
			"presence": services.Presence,
			"email":    services.Email,
		},
		Logger: loggerFactory.Child("lifecycle.services"),
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create services lifecycle controller")
	}
	if err := servicesLifecycleController.Start(context.Background()); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start services lifecycle controller")
	}
	defer servicesLifecycleController.Stop(context.Background())

	if err := kafkaConsumerRouter.Start(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start kafka consumer router")
	}
	defer kafkaConsumerRouter.Stop()

	//	@fixme	remove me
	/*if err := routing.OrchestrateKafkaTest(
		loggerFactory.ChildPtr("clients.kafka.example"), clients.Kafka.Admin, clients.Kafka.Data,
	); err != nil {
		panic(err)
	}*/

	emailRequest := emailv1.SendEmailRequest{
		MessageId: uuid.New().String(),
		CreatedAt: timestamppb.New(time.Now().UTC()),
		Source: &emailv1.Source{
			Service:     "auth-service",
			Environment: "prod",
			TraceId:     "a1b2c3",
		},
		Email: &emailv1.Email{
			To: []*emailv1.EmailAddress{
				{Email: "user@example.com"},
			},
			Subject:     "Welcome to Example App",
			ContentMode: emailv1.ContentMode_CONTENT_MODE_TEMPLATE,
			/*Raw: &emailv1.RawContent{
				Text: "Welcome to Example App!",
				Html: "<p>Welcome to <strong>Example App</strong>!</p>",
			},*/
			Template: &emailv1.TemplateContent{
				TemplateId: "message",
				// Locale:     proto.String("fr"),
				Vars: map[string]string{
					"NAME": "Alice",
				},
			},
			InteractionMode: emailv1.InteractionMode_INTERACTION_MODE_AUTOMATED,
			Importance:      emailv1.ImportanceLevel_IMPORTANCE_LEVEL_NORMAL,
		},
	}

	for i := 0; i < int(cfg.Email.QueueSize*2); i++ {
		err := services.Email.Send(context.Background(), &emailRequest)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to enqueue email")
		}
	}
	//	@fixme	remove me

	blockOnSignal(syscall.SIGINT, syscall.SIGTERM)
}

func blockOnSignal(signals ...os.Signal) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals...)
	<-sigChan
}
