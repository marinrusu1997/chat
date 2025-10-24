package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type Client struct {
	logger  zerolog.Logger
	options *redis.ClusterOptions
	Driver  *redis.ClusterClient
}

type ClientOptions struct {
	TLSConfig  *tls.Config
	Addresses  []string
	ClientName string
	Username   string
	Password   string
	Logger     zerolog.Logger
}

func NewClient(options ClientOptions) *Client {
	return &Client{
		logger: options.Logger,
		options: &redis.ClusterOptions{
			TLSConfig:  options.TLSConfig,
			Addrs:      options.Addresses,
			ClientName: options.ClientName,
			Username:   options.Username,
			Password:   options.Password,
			NewClient: func(opt *redis.Options) *redis.Client {
				opt.DB = 0
				opt.MaxRetries = 5
				opt.ReadTimeout = 2 * time.Second
				opt.WriteTimeout = 2 * time.Second
				opt.ContextTimeoutEnabled = true
				opt.PoolFIFO = true
				opt.MinIdleConns = 10
				opt.MaxIdleConns = 50
				opt.ConnMaxLifetime = 1 * time.Hour

				return redis.NewClient(opt)
			},
			ReadOnly:       true,
			RouteByLatency: true,
		},
		Driver: nil,
	}
}

func (c *Client) Start(_ context.Context) error {
	if c.Driver != nil {
		return fmt.Errorf("redis driver already started")
	}

	c.Driver = redis.NewClusterClient(c.options)
	return nil
}

func (c *Client) Stop(_ context.Context) {
	if c.Driver == nil {
		c.logger.Warn().Msg("Redis client already stopped")
		return
	}

	err := c.Driver.Close()
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to close Redis client")
	}
}
