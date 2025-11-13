package nats

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

var ErrAlreadyStarted = errors.New("nats client already started")

type clientConfig struct {
	servers string
	options []nats.Option
}

type Client struct {
	logger zerolog.Logger
	config *clientConfig
	Driver *nats.Conn
}

type ClientOptions struct {
	Servers    []string
	TLSConfig  *tls.Config
	ClientName string
	Username   string
	Password   string
	Logger     zerolog.Logger
}

func NewClient(options *ClientOptions) *Client {
	return &Client{
		logger: options.Logger,
		config: &clientConfig{
			servers: strings.Join(options.Servers, ", "),
			options: []nats.Option{
				nats.Name(options.ClientName),
				nats.Secure(options.TLSConfig),
				nats.UserInfo(options.Username, options.Password),
				nats.DisconnectErrHandler(func(conn *nats.Conn, err error) {
					if err != nil {
						options.Logger.Err(err).Msgf("Connection disconnected with error from NATS server: %s", conn.ConnectedUrlRedacted())
					}
				}),
				nats.ReconnectHandler(func(conn *nats.Conn) {
					options.Logger.Info().Msgf("Successfully reconnected to NATS server: %s", conn.ConnectedUrlRedacted())
				}),
				nats.ReconnectErrHandler(func(conn *nats.Conn, err error) {
					options.Logger.Err(err).Msgf("Reconnect failed to NATS server: %s", conn.ConnectedUrlRedacted())
				}),
				nats.ErrorHandler(func(conn *nats.Conn, sub *nats.Subscription, err error) {
					options.Logger.Err(err).Msgf("NATS error on connection '%s' and subscription '%s'",
						conn.ConnectedUrlRedacted(), sub.Subject,
					)
				}),
				nats.LameDuckModeHandler(func(conn *nats.Conn) {
					options.Logger.Warn().Msgf("NATS server is in lame duck mode: %s", conn.ConnectedUrlRedacted())
				}),
				nats.RetryOnFailedConnect(false),
				nats.Compression(true),
				nats.SkipHostLookup(),
			},
		},
		Driver: nil,
	}
}

func (c *Client) Start(_ context.Context) error {
	if c.Driver != nil {
		return ErrAlreadyStarted
	}

	conn, err := nats.Connect(c.config.servers, c.config.options...)
	if err != nil {
		return fmt.Errorf("failed to start nats client: %w", err)
	}

	c.Driver = conn
	return nil
}

func (c *Client) Stop(_ context.Context) {
	if c.Driver == nil {
		c.logger.Warn().Msg("nats client already stopped")
		return
	}

	c.Driver.Close()
	c.Driver = nil
}
