package main

import (
	"chat/src/clients"
	"chat/src/clients/kafka"
	"chat/src/platform/config"
	"chat/src/platform/logging"
	"fmt"

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

	_, err = clients.BootstrapStorageClients(cfg, loggerFactory)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to bootstrap storage clients")
	}
	defer clients.Shutdown()

	// @fixme Kafka
	err = kafka.OrchestrateKafkaTest(&logger)
	if err != nil {
		panic(err)
	}
}
