package elasticsearch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/rs/zerolog"
)

// @FIXME When a message is deleted, your application performs an update in Elasticsearch to add a deleted_at timestamp to the document.
//   All search queries from your application must be modified to filter out these documents (e.g., must_not: { exists: { field: "deleted_at" } }).
// @FIXME When storing messages, use ?_source_excludes=content parameter in the URL to avoid leaking cleartext content in _source field.
//   Also, use message_id "https://es-coordinating-1:9200/chat-messages/_doc/${message_id}"

var ErrAlreadyStarted = errors.New("elasticsearch client already started")

type Client struct {
	logger zerolog.Logger
	config elasticsearch.Config
	Driver *elasticsearch.Client
}

type ClientLoggerOptions struct {
	Client zerolog.Logger
	Driver zerolog.Logger
}

type ClientOptions struct {
	Logger       ClientLoggerOptions
	TLSConfig    *tls.Config
	Username     string
	Password     string
	Addresses    []string
	ShouldLogReq bool
	ShouldLogRes bool
}

func NewClient(options *ClientOptions) *Client {
	// 1. Performance: Tune the underlying HTTP Transport
	transport := &http.Transport{
		TLSClientConfig:     options.TLSConfig,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	// 2. Resilience: Configure an exponential backoff for retries
	zerologAdapter := &driverLoggerAdapter{
		logger:             options.Logger.Driver,
		shouldLogRequests:  options.ShouldLogReq,
		shouldLogResponses: options.ShouldLogRes,
	}

	config := elasticsearch.Config{
		// --- Connection ---
		Addresses: options.Addresses,
		Username:  options.Username,
		Password:  options.Password,
		Transport: transport,

		// --- Resilience ---
		MaxRetries:    5,
		RetryOnStatus: []int{429, 502, 503, 504}, // Add 429 for "Too Many Requests"
		RetryBackoff: func(attempt int) time.Duration {
			duration := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			options.Logger.Driver.Warn().Int("attempt", attempt).Dur("backoff_duration", duration).Msg("Elasticsearch request failed, backing off")
			return duration
		},

		// --- Performance ---
		CompressRequestBody: true,

		// --- Observability ---
		EnableMetrics: true,
		Logger:        zerologAdapter,
	}

	return &Client{
		logger: options.Logger.Client,
		config: config,
		Driver: nil,
	}
}

func (c *Client) Start(_ context.Context) error {
	if c.Driver != nil {
		return ErrAlreadyStarted
	}

	client, err := elasticsearch.NewClient(c.config)
	if err != nil {
		return fmt.Errorf("failed to start elasticsearch client: %w", err)
	}

	c.Driver = client
	return nil
}

func (c *Client) Stop(_ context.Context) {
	if c.Driver == nil {
		c.logger.Warn().Msg("Elasticsearch client already stopped")
		return
	}

	c.Driver = nil
}
