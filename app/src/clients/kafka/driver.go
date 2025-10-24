package kafka

import (
	error2 "chat/src/platform/error"
	"chat/src/util"
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/samber/oops"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Client struct {
	logger zerolog.Logger
	Driver *kgo.Client
}

func NewClient(config ConfigurationBuilder) (*Client, error) {
	options, err := config.getOptions()
	if err != nil {
		return nil, oops.
			In(util.GetFunctionName()).
			Code(error2.ECONFIG).
			Wrapf(err, "can't create a new Kafka client because configuration is broken")
	}

	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, oops.
			In(util.GetFunctionName()).
			Code(error2.EINIT).
			Wrapf(err, "can't create a new Kafka client")
	}

	return &Client{
		logger: config.logger.Client,
		Driver: client,
	}, nil
}

func (c *Client) Close() {
	// @fixme do manual tests with commit offsets

	// 1. Final Synchronous Commit
	// Use a context with a timeout (e.g., 5 seconds) for the final commit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Driver.CommitUncommittedOffsets(ctx)
	if err != nil {
		c.logger.Error().Err(err).Msg("Final synchronous commit failed")
	} else {
		c.logger.Info().Msg("Successfully performed final synchronous commit.")
	}

	// 2. Close the Client
	c.Driver.Close()
	c.logger.Info().Msg("Kafka client closed.")
}
