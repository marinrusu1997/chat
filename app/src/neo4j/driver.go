package neo4j

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
	"github.com/rs/zerolog"
)

type EcsZerologAdapter struct {
	logger zerolog.Logger
}

func (a *EcsZerologAdapter) Error(name string, id string, err error) {
	a.logger.Error().Err(err).Str("name", name).Str("id", id).Msg("Neo4j Driver Error")
}
func (a *EcsZerologAdapter) Warnf(name string, id string, msg string, args ...any) {
	a.logger.Warn().Str("name", name).Str("id", id).Msg(fmt.Sprintf(msg, args...))
}
func (a *EcsZerologAdapter) Infof(name string, id string, msg string, args ...any) {
	a.logger.Info().Str("name", name).Str("id", id).Msg(fmt.Sprintf(msg, args...))
}
func (a *EcsZerologAdapter) Debugf(name string, id string, msg string, args ...any) {
	a.logger.Debug().Str("name", name).Str("id", id).Msg(fmt.Sprintf(msg, args...))
}

type Config struct {
	logger     zerolog.Logger
	DbUri      string
	DbUser     string
	DbPassword string
}

func CreateDriver(driverConfig Config) (neo4j.Driver, error) {
	driver, err := neo4j.NewDriver(
		driverConfig.DbUri,
		neo4j.BasicAuth(driverConfig.DbUser, driverConfig.DbPassword, ""),
		func(config *config.Config) {
			config.Log = &EcsZerologAdapter{logger: driverConfig.logger}
			config.MaxTransactionRetryTime = 5 * time.Second
			config.MaxConnectionPoolSize = 200
			config.ConnectionAcquisitionTimeout = 10 * time.Second
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Neo4j driver with uri '%s': %w", driverConfig.DbUri, err)
	}

	err = driver.VerifyConnectivity(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to verify connectivity to Neo4j database at '%s': %w", driverConfig.DbUri, err)
	}

	return driver, nil
}
