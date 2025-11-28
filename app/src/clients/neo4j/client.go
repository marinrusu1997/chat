package neo4j

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
	"github.com/rs/zerolog"
)

var ErrAlreadyStarted = errors.New("neo4j client already started")

type clientOptions struct {
	uri          string
	auth         neo4j.AuthToken
	databaseName string
	configurer   func(*config.Config)
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
	Logger       ClientLoggerOptions
	URI          string
	Username     string
	Password     string
	DatabaseName string
	TLSConfig    *tls.Config
}

func NewClient(options *ClientOptions) *Client {
	return &Client{
		logger: clientLoggers{
			client:  options.Logger.Client,
			session: sessionLoggerAdapter{logger: options.Logger.Session},
		},
		options: clientOptions{
			uri:          options.URI,
			auth:         neo4j.BasicAuth(options.Username, options.Password, ""),
			databaseName: options.DatabaseName,
			configurer: func(config *config.Config) {
				config.TlsConfig = options.TLSConfig
				config.Log = &driverLoggerAdapter{logger: options.Logger.Driver}
				config.MaxTransactionRetryTime = 5 * time.Second
				config.MaxConnectionPoolSize = 200
				config.ConnectionAcquisitionTimeout = 10 * time.Second
			},
		},
		pingSession: nil,
		Driver:      nil,
	}
}

func (c *Client) Start(ctx context.Context) error {
	if c.Driver != nil {
		return ErrAlreadyStarted
	}

	driver, err := neo4j.NewDriver(c.options.uri, c.options.auth, c.options.configurer)
	if err != nil {
		return fmt.Errorf("failed to create Neo4j driver: %w", err)
	}

	err = driver.VerifyConnectivity(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify connectivity to Neo4j database at '%s': %w", c.options.uri, err)
	}

	c.Driver = driver
	c.pingSession = c.NewSession(ctx, neo4j.AccessModeRead)

	return nil
}

func (c *Client) Stop(ctx context.Context) {
	if c.Driver == nil {
		c.logger.client.Warn().Msg("Neo4j client already stopped")
		return
	}

	if err := c.pingSession.Close(ctx); err != nil {
		c.logger.client.Error().Err(err).Msg("Failed to close Neo4j ping session")
	}
	if err := c.Driver.Close(ctx); err != nil {
		c.logger.client.Error().Err(err).Msg("Failed to close Neo4j driver")
	}

	c.Driver = nil
}

func (c *Client) NewSession(ctx context.Context, accessMode neo4j.AccessMode) neo4j.Session {
	return c.Driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode:   accessMode,
		DatabaseName: c.options.databaseName,
		FetchSize:    neo4j.FetchDefault,
		BoltLogger:   &c.logger.session,
	})
}
