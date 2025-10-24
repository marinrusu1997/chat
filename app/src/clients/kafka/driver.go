package kafka

import (
	error2 "chat/src/platform/error"
	"chat/src/util"
	"context"

	"github.com/rs/zerolog"
	"github.com/samber/oops"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Client struct {
	logger  zerolog.Logger
	options []kgo.Opt
	Driver  *kgo.Client
}

func NewClient(config ConfigurationBuilder) (*Client, error) {
	options, err := config.getOptions()
	if err != nil {
		return nil, oops.
			In(util.GetFunctionName()).
			Code(error2.ECONFIG).
			Wrapf(err, "can't create a new Kafka client because configuration is broken")
	}

	return &Client{
		logger:  config.logger.Client,
		options: options,
		Driver:  nil,
	}, nil
}

func (c *Client) Start(_ context.Context) error {
	if c.Driver != nil {
		return oops.
			In(util.GetFunctionName()).
			Code(error2.EINIT).
			New("Kafka client already started")
	}

	client, err := kgo.NewClient(c.options...)
	if err != nil {
		return oops.
			In(util.GetFunctionName()).
			Code(error2.EINIT).
			Wrapf(err, "can't create a new Kafka client")
	}

	c.Driver = client
	return nil
}

func (c *Client) Stop(ctx context.Context) {
	if c.Driver == nil {
		c.logger.Warn().Msg("Kafka client already stopped")
		return
	}

	// @fixme do manual tests with commit offsets
	err := c.Driver.CommitUncommittedOffsets(ctx)
	if err != nil {
		c.logger.Error().Err(err).Msg("Final synchronous commit failed")
	} else {
		c.logger.Info().Msg("Successfully performed final synchronous commit.")
	}

	c.Driver.Close()
	c.Driver = nil
}
