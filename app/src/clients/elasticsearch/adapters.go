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

func (a *driverLoggerAdapter) RequestBodyEnabled() bool  { return a.shouldLogRequests }
func (a *driverLoggerAdapter) ResponseBodyEnabled() bool { return a.shouldLogResponses }
func (a *driverLoggerAdapter) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error { //nolint:revive // interface compliance
	var event *zerolog.Event

	// Log errors regardless of the configuration
	if err != nil {
		event = a.logger.Error().Err(err)
	} else if a.shouldLogRequests || a.shouldLogResponses {
		event = a.logger.Info()
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
	if a.shouldLogRequests && req.Body != nil {
		reqBody, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			a.logger.Warn().
				Err(readErr).
				Str("method", req.Method).
				Str("url", req.URL.String()).
				Msg("Failed to read request body")
		} else {
			req.Body = io.NopCloser(bytes.NewBuffer(reqBody)) // Restore the body
			event.Dict("request", zerolog.Dict().RawJSON("body", reqBody))
		}
	}

	// Conditionally log the response body
	if a.shouldLogResponses && res != nil {
		resDict := zerolog.Dict().Int("status_code", res.StatusCode)
		if res.Body != nil {
			resBody, readErr := io.ReadAll(res.Body)
			if readErr != nil {
				a.logger.Warn().
					Err(readErr).
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
