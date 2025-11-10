package health

import (
	"context"
	"errors"
	"strings"

	ahocorasick "github.com/BobuSumisu/aho-corasick"
)

type Pingable interface {
	PingShallow(ctx context.Context) PingResult
	PingDeep(ctx context.Context) PingResult
}

type PingDepth string

const (
	PingDepthShallow PingDepth = "shallow"
	PingDepthDeep    PingDepth = "deep"
)

type PingStatus string

const (
	PingStatusHealthy   PingStatus = "healthy"
	PingStatusDegraded  PingStatus = "degraded"
	PingStatusUnhealthy PingStatus = "unhealthy"
)

type PingCause string

const (
	PingCauseOk PingCause = "ok"

	PingCauseUnstable   PingCause = "unstable"
	PingCauseOverloaded PingCause = "overloaded"

	PingCauseNetwork     PingCause = "network"
	PingCauseTLS         PingCause = "tls"
	PingCauseTimeout     PingCause = "timeout"
	PingCauseBadResponse PingCause = "bad_response"
	PingCauseAuthFailed  PingCause = "auth_failed"
	PingCauseBadState    PingCause = "bad_state"

	PingCauseInternal PingCause = "internal"

	PingCauseUnknown PingCause = "unknown"
)

var causeToStatus = map[PingCause]PingStatus{
	PingCauseOk:          PingStatusHealthy,
	PingCauseUnstable:    PingStatusDegraded,
	PingCauseOverloaded:  PingStatusDegraded,
	PingCauseNetwork:     PingStatusUnhealthy,
	PingCauseTLS:         PingStatusUnhealthy,
	PingCauseTimeout:     PingStatusUnhealthy,
	PingCauseBadResponse: PingStatusUnhealthy,
	PingCauseAuthFailed:  PingStatusUnhealthy,
	PingCauseBadState:    PingStatusUnhealthy,
	PingCauseInternal:    PingStatusUnhealthy,
	PingCauseUnknown:     PingStatusUnhealthy,
}

func (c *PingCause) ToStatus() PingStatus {
	if status, ok := causeToStatus[*c]; ok {
		return status
	}
	return PingStatusUnhealthy
}

type errorPatternCause struct {
	pattern string
	cause   PingCause
}

var patternCauses = []errorPatternCause{
	// Network-level failures
	{"connection refused", PingCauseNetwork},
	{"no route", PingCauseNetwork},
	{"connection reset", PingCauseNetwork},
	{"broken pipe", PingCauseNetwork},
	{"eof", PingCauseNetwork},
	{"dial tcp", PingCauseNetwork},

	// TLS / certificate issues
	{"tls", PingCauseTLS},
	{"x509", PingCauseTLS},
	{"certificate", PingCauseTLS},
	{"handshake", PingCauseTLS},

	// Authentication / authorization
	{"unauthorized", PingCauseAuthFailed},
	{"authentication failed", PingCauseAuthFailed},
	{"invalid password", PingCauseAuthFailed},
	{"access denied", PingCauseAuthFailed},
	{"permission denied", PingCauseAuthFailed},

	// Overload / resource exhaustion
	{"too many connections", PingCauseOverloaded},
	{"connection pool exhausted", PingCauseOverloaded},
	{"server is overloaded", PingCauseOverloaded},
	{"timeout acquiring connection", PingCauseOverloaded},

	// Database / protocol errors
	{"syntax error", PingCauseBadResponse},
	{"malformed query", PingCauseBadResponse},
	{"bad request", PingCauseBadResponse},
	{"invalid argument", PingCauseBadResponse},
	{"protocol error", PingCauseBadResponse},

	// Internal or application-specific
	{"panic", PingCauseInternal},
	{"internal server error", PingCauseInternal},
	{"unexpected error", PingCauseInternal},
}
var (
	causeByIndex []PingCause
	matcher      *ahocorasick.Trie
)

func init() {
	errorPatterns := make([]string, 0, len(patternCauses))
	causeByIndex = make([]PingCause, 0, len(patternCauses))

	for _, p := range patternCauses {
		errorPatterns = append(errorPatterns, p.pattern)
		causeByIndex = append(causeByIndex, p.cause)
	}

	matcher = ahocorasick.NewTrieBuilder().
		AddStrings(errorPatterns).
		Build()
}

func PingCauseFromRequestError(err error) PingCause {
	if err == nil {
		return PingCauseOk
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return PingCauseTimeout
	}
	if errors.Is(err, context.Canceled) {
		return PingCauseInternal
	}

	match := matcher.MatchFirstString(strings.ToLower(err.Error()))
	if match != nil {
		return causeByIndex[match.Pattern()]
	}

	return PingCauseUnknown
}
