package kafka

import (
	"chat/src/platform/perr"
	"chat/src/util"
	"context"
	"errors"

	"github.com/rs/zerolog"
	"github.com/samber/oops"
	"github.com/twmb/franz-go/pkg/kgo"
)

var ErrAlreadyStarted = errors.New("kafka client already started")

const (
	AdminClientName = "kafka.admin"
	DataClientName  = "kafka.data"
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
			Code(perr.ECONFIG).
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
		return ErrAlreadyStarted
	}

	client, err := kgo.NewClient(c.options...)
	if err != nil {
		return oops.
			In(util.GetFunctionName()).
			Code(perr.EINIT).
			Wrapf(err, "can't create a new Kafka client")
	}

	c.Driver = client
	return nil
}

func (c *Client) Stop(_ context.Context) {
	if c.Driver == nil {
		c.logger.Warn().Msg("Kafka client already stopped")
		return
	}

	c.Driver.Close()
	c.Driver = nil
}
