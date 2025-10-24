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
	logger zerolog.Logger
	Driver *redis.ClusterClient
}

type ClientOptions struct {
	TLSConfig  *tls.Config
	Addresses  []string
	ClientName string
	Username   string
	Password   string
	Logger     zerolog.Logger
}

func NewClient(options ClientOptions) (*Client, error) {
	client := redis.NewClusterClient(&redis.ClusterOptions{
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
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %v", err)
	}

	return &Client{
		logger: options.Logger,
		Driver: client,
	}, nil
}

func (client *Client) Close() {
	err := client.Driver.Close()
	if err != nil {
		client.logger.Error().Err(err).Msg("Failed to close Redis client")
	} else {
		client.logger.Info().Msg("Redis client closed")
	}
}
