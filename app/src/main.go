package main

import (
	"chat/src/clients/elasticsearch"
	"chat/src/clients/etcd"
	"chat/src/clients/neo4j"
	"chat/src/clients/postgresql"
	"chat/src/clients/redis"
	"chat/src/clients/scylla"
	"chat/src/platform/config"
	"chat/src/platform/health"
	"chat/src/platform/lifecycle"
	"chat/src/platform/logging"
	"chat/src/platform/state"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.yaml.in/yaml/v3"
)

// @FIXME: https://github.com/uber-go/guide/tree/master

func main() {
	cfg, err := config.Load(config.LoadConfigOptions{
		YamlFilePaths: []string{"/app/config/config.yaml"},
		EnvVarPrefix:  "CHAT_APP_",
	})
	if err != nil {
		panic(fmt.Sprintf("Error loading config: %+v", err))
	}

	loggerFactory, err := logging.NewFactory(logging.Options{
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

	storageClients, err := state.CreateStorageClients(cfg, loggerFactory)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create storage clients")
	}

	lifecycleController, err := lifecycle.NewController(lifecycle.ControllerOptions{
		Services: map[string]lifecycle.ServiceLifecycle{
			elasticsearch.PingTargetName: storageClients.Elasticsearch,
			// kafka.PingTargetName:         storageClients.Kafka.Admin, @fixme enable later
			neo4j.PingTargetName:      storageClients.Neo4j,
			etcd.PingTargetName:       storageClients.Etcd,
			postgresql.PingTargetName: storageClients.PostgreSQL,
			redis.PingTargetName:      storageClients.Redis,
			scylla.PingTargetName:     storageClients.ScyllaDB,
		},
		Logger: loggerFactory.Child("lifecycle.controller"),
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create lifecycle controller")
	}
	if err := lifecycleController.Start(context.Background()); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start lifecycle controller")
	}
	defer lifecycleController.Stop(context.Background())

	healthController, err := health.NewController(&health.ControllerConfig{
		Dependencies: map[string]health.Pingable{
			elasticsearch.PingTargetName: storageClients.Elasticsearch,
			// kafka.PingTargetName:         storageClients.Kafka.Admin, @fixme enable later
			neo4j.PingTargetName:      storageClients.Neo4j,
			etcd.PingTargetName:       storageClients.Etcd,
			postgresql.PingTargetName: storageClients.PostgreSQL,
			redis.PingTargetName:      storageClients.Redis,
			scylla.PingTargetName:     storageClients.ScyllaDB,
		},
		Logger: loggerFactory.Child("health.controller"),
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create heath controller")
	}
	healthController.Start()
	defer healthController.Stop()

	blockOnSignal(syscall.SIGINT, syscall.SIGTERM)
}

func blockOnSignal(signals ...os.Signal) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals...)
	<-sigChan
}
