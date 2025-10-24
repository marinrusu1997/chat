package neo4j

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
	"github.com/rs/zerolog"
)

const closeTimeout = 5 * time.Second

type clientOptions struct {
	databaseName   string
	connectTimeout time.Duration
}

type clientLoggers struct {
	client  zerolog.Logger
	session sessionLoggerAdapter
}

type Client struct {
	logger      clientLoggers
	options     clientOptions
	pingSession neo4j.Session
	Driver      neo4j.Driver
}

type ClientLoggerOptions struct {
	Client  zerolog.Logger
	Driver  zerolog.Logger
	Session zerolog.Logger
}

type ClientOptions struct {
	Logger         ClientLoggerOptions
	Uri            string
	Username       string
	Password       string
	ConnectTimeout time.Duration
	DatabaseName   string
	TlsConfig      *tls.Config
}

func NewClient(options ClientOptions) (*Client, error) {
	driver, err := neo4j.NewDriver(
		options.Uri,
		neo4j.BasicAuth(options.Username, options.Password, ""),
		func(config *config.Config) {
			config.TlsConfig = options.TlsConfig
			config.Log = &driverLoggerAdapter{logger: options.Logger.Driver}
			config.MaxTransactionRetryTime = 5 * time.Second
			config.MaxConnectionPoolSize = 200
			config.ConnectionAcquisitionTimeout = 10 * time.Second
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Neo4j driver: %w", err)
	}

	err = driver.VerifyConnectivity(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to verify connectivity to Neo4j database at '%s': %w", options.Uri, err)
	}

	client := &Client{
		logger: clientLoggers{
			client:  options.Logger.Client,
			session: sessionLoggerAdapter{logger: options.Logger.Session},
		},
		options: clientOptions{
			databaseName:   options.DatabaseName,
			connectTimeout: options.ConnectTimeout,
		},
		Driver: driver,
	}
	client.pingSession = client.NewSession(neo4j.AccessModeRead)

	return client, nil
}

func (c *Client) NewSession(accessMode neo4j.AccessMode) neo4j.Session {
	ctx, cancel := context.WithTimeout(context.Background(), c.options.connectTimeout)
	defer cancel()

	return c.Driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode:   accessMode,
		DatabaseName: c.options.databaseName,
		FetchSize:    neo4j.FetchDefault,
		BoltLogger:   &c.logger.session,
	})
}

func (c *Client) Close() {
	sessionCtx, sessionCtxCancel := context.WithTimeout(context.Background(), closeTimeout)
	defer sessionCtxCancel()

	if err := c.pingSession.Close(sessionCtx); err != nil {
		c.logger.client.Error().Err(err).Msg("Failed to close Neo4j ping session")
	} else {
		c.logger.client.Info().Msg("Neo4j ping session closed")
	}

	driverCtx, driverCtxCancel := context.WithTimeout(context.Background(), closeTimeout)
	defer driverCtxCancel()

	if err := c.Driver.Close(driverCtx); err != nil {
		c.logger.client.Error().Err(err).Msg("Failed to close Neo4j driver")
	}

	c.logger.client.Info().Msg("Neo4j client closed")
}
