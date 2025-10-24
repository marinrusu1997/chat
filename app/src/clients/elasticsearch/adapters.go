package elasticsearch

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// driverLoggerAdapter adapts zerolog to the elastictransport.Logger interface.
type driverLoggerAdapter struct {
	logger             zerolog.Logger
	shouldLogRequests  bool
	shouldLogResponses bool
}

func (adapter *driverLoggerAdapter) RequestBodyEnabled() bool  { return adapter.shouldLogRequests }
func (adapter *driverLoggerAdapter) ResponseBodyEnabled() bool { return adapter.shouldLogResponses }
func (adapter *driverLoggerAdapter) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	var event *zerolog.Event

	// Log errors regardless of the configuration
	if err != nil {
		event = adapter.logger.Error().Err(err)
	} else if adapter.shouldLogRequests || adapter.shouldLogResponses {
		event = adapter.logger.Info()
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
	if adapter.shouldLogRequests && req.Body != nil {
		reqBody, err := io.ReadAll(req.Body)
		if err != nil {
			adapter.logger.Warn().
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
	if adapter.shouldLogResponses && res != nil {
		resDict := zerolog.Dict().Int("status_code", res.StatusCode)
		if res.Body != nil {
			resBody, err := io.ReadAll(res.Body)
			if err != nil {
				adapter.logger.Warn().
					Err(err).
					Str("method", req.Method).
					Str("url", req.URL.String()).
					Msg("Failed to read response body")
			} else {
				res.Body = io.NopCloser(bytes.NewBuffer(resBody)) // Restore the body
				resDict.Bytes("body", resBody)
			}
		}
		event.Dict("response", resDict)
	}

	// Finalize the log entry
	event.Msg("Elasticsearch request completed")

	// The method must return the original error.
	return err
}
