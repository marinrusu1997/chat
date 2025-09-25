package elasticsearch

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/rs/zerolog"
)

// @FIXME When a message is deleted, your application performs an update in Elasticsearch to add a deleted_at timestamp to the document.
//   All search queries from your application must be modified to filter out these documents (e.g., must_not: { exists: { field: "deleted_at" } }).
// @FIXME When storing messages, use ?_source_excludes=content parameter in the URL to avoid leaking cleartext content in _source field.
//   Also, use message_id "https://es-coordinating-1:9200/chat-messages/_doc/${message_id}"

// ZerologAdapter adapts zerolog to the elastictransport.Logger interface.
type ZerologAdapter struct {
	Logger             zerolog.Logger
	ShouldLogRequests  bool
	ShouldLogResponses bool
}

func (l *ZerologAdapter) RequestBodyEnabled() bool  { return l.ShouldLogRequests }
func (l *ZerologAdapter) ResponseBodyEnabled() bool { return l.ShouldLogResponses }
func (l *ZerologAdapter) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	var event *zerolog.Event

	// Log errors regardless of the configuration
	if err != nil {
		event = l.Logger.Error().Err(err)
	} else if l.ShouldLogRequests || l.ShouldLogResponses {
		event = l.Logger.Info()
	}
	if event == nil {
		return err
	}

	// Add basic round trip info
	event.Time("start", start).
		Dur("duration", dur).
		Str("method", req.Method).
		Str("url", req.URL.String())

	// Conditionally log the request body
	if l.ShouldLogRequests && req.Body != nil {
		reqBody, err := io.ReadAll(req.Body)
		if err != nil {
			l.Logger.Warn().
				Err(err).
				Str("method", req.Method).
				Str("url", req.URL.String()).
				Msg("Failed to read request body")
		} else {
			req.Body = io.NopCloser(bytes.NewBuffer(reqBody)) // Restore the body
			event.Dict("request", zerolog.Dict().RawJSON("body", reqBody))
		}
	}

	// Conditionally log the response body
	if l.ShouldLogResponses && res != nil {
		resDict := zerolog.Dict().Int("status_code", res.StatusCode)
		if res.Body != nil {
			resBody, err := io.ReadAll(res.Body)
			if err != nil {
				l.Logger.Warn().
					Err(err).
					Str("method", req.Method).
					Str("url", req.URL.String()).
					Msg("Failed to read response body")
			} else {
				res.Body = io.NopCloser(bytes.NewBuffer(resBody)) // Restore the body
				resDict.RawJSON("body", resBody)
			}
		}
		event.Dict("response", resDict)
	}

	// Finalize the log entry
	event.Msg("Elasticsearch request completed")

	// The method must return the original error.
	return err
}

type Config struct {
	log            zerolog.Logger
	CACertFilePath string
	Username       string
	Password       string
	Addresses      []string
	ShouldLogReq   bool
	ShouldLogRes   bool
}

func CreateClient(config Config) (*elasticsearch.Client, error) {
	caCert, err := os.ReadFile(config.CACertFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate from path %s: %v", config.CACertFilePath, err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// 1. Performance: Tune the underlying HTTP Transport
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: caCertPool,
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	// 2. Resilience: Configure an exponential backoff for retries
	log := config.log
	zerologAdapter := &ZerologAdapter{
		Logger:             log,
		ShouldLogRequests:  config.ShouldLogReq,
		ShouldLogResponses: config.ShouldLogRes,
	}

	retryBackoff := func(attempt int) time.Duration {
		duration := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		log.Warn().Int("attempt", attempt).Dur("backoff_duration", duration).Msg("Elasticsearch request failed, backing off")
		return duration
	}

	cfg := elasticsearch.Config{
		// --- Connection ---
		Addresses: config.Addresses,
		Username:  config.Username,
		Password:  config.Password,
		Transport: transport,

		// --- Resilience ---
		MaxRetries:    5,
		RetryOnStatus: []int{429, 502, 503, 504}, // Add 429 for "Too Many Requests"
		RetryBackoff:  retryBackoff,

		// --- Performance ---
		CompressRequestBody: true,

		// --- Observability ---
		EnableMetrics: true,
		Logger:        zerologAdapter,
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new Elasticsearch client '%s': %v", config.Addresses[0], err)
	}

	res, err := es.Info()
	if err != nil {
		return nil, fmt.Errorf("error getting response from Elasticsearch '%s': %v", config.Addresses[0], err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Warn().Err(err).Msg("Failed to close Elasticsearch response body resulted from Info()")
		}
	}(res.Body)

	return es, nil
}
