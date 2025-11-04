package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type Client struct {
	logger zerolog.Logger
	config clientv3.Config
	Driver *clientv3.Client
}

type ClientLoggerOptions struct {
	Client zerolog.Logger
	Driver zerolog.Logger
}

type ClientOptions struct {
	Endpoints []string
	TLSConfig *tls.Config
	Logger    ClientLoggerOptions
}

func NewClient(options ClientOptions) *Client {
	return &Client{
		logger: options.Logger.Client,
		config: clientv3.Config{
			Endpoints:            options.Endpoints,
			AutoSyncInterval:     5 * time.Minute,
			DialTimeout:          5 * time.Second,
			DialKeepAliveTime:    10 * time.Second,
			DialKeepAliveTimeout: 10 * time.Second,
			MaxCallSendMsgSize:   2 * 1024 * 1024,
			MaxCallRecvMsgSize:   16 * 1024 * 1024,
			TLS:                  options.TLSConfig,
			Username:             "",
			Password:             "",
			RejectOldCluster:     true,
			DialOptions: []grpc.DialOption{
				grpc.WithBlock(),
				grpc.WithReturnConnectionError(),
				grpc.FailOnNonTempDialError(true),
				grpc.WithInitialWindowSize(1 << 20),
				grpc.WithInitialConnWindowSize(1 << 20),
				grpc.WithWriteBufferSize(64 * 1024),
				grpc.WithReadBufferSize(64 * 1024),
			},
			Context:               nil,
			Logger:                zap.New(&zapCoreBridge{logger: options.Logger.Driver}),
			LogConfig:             nil,
			PermitWithoutStream:   true,
			MaxUnaryRetries:       5,
			BackoffWaitBetween:    500 * time.Millisecond,
			BackoffJitterFraction: 0.1,
		},
		Driver: nil,
	}
}

func (c *Client) Start(_ context.Context) error {
	if c.Driver != nil {
		return fmt.Errorf("etcd driver already started")
	}

	client, err := clientv3.New(c.config)
	if err != nil {
		return fmt.Errorf("failed to start etcd client: %w", err)
	}

	c.Driver = client
	return nil
}

func (c *Client) Stop(_ context.Context) {
	if c.Driver == nil {
		c.logger.Warn().Msg("etcd client already stopped")
		return
	}

	if err := c.Driver.Close(); err != nil {
		c.logger.Error().Err(err).Msg("Failed to close etcd client")
	}
	c.Driver = nil
}
