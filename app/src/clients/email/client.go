package email

import (
	"context"
	"fmt"
)

type Client struct {
	pool *workerPool
}

type ClientOptions struct {
	WorkerPoolOptions WorkerPoolOptions
}

func NewClient(options *ClientOptions) *Client {
	return &Client{
		pool: newWorkerPool(options.WorkerPoolOptions),
	}
}

func (c *Client) Start(ctx context.Context) error {
	if err := c.pool.Start(ctx); err != nil {
		return fmt.Errorf("starting worker pool failed: %w", err)
	}
	return nil
}

func (c *Client) Stop(_ context.Context) {
	c.pool.Stop()
}

func (c *Client) Send(request Request) error {
	if err := c.pool.Submit(request); err != nil {
		return fmt.Errorf("submitting email request to worker pool failed: %w", err)
	}
	return nil
}
