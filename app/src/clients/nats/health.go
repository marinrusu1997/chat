package nats

import (
	"chat/src/platform/health"
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	PingTargetName            = "nats"
	pingDeepAcceptableLatency = 150 * time.Millisecond
)

func (c *Client) PingShallow(_ context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	status := c.Driver.Status()

	switch status {
	case nats.DISCONNECTED, nats.CLOSED:
		pingResult.SetPingOutput(
			health.PingCauseInternal,
			fmt.Sprintf("NATS client '%s' is not connected: status=%s", c.Driver.ConnectedUrlRedacted(), status.String()),
		)
	case nats.RECONNECTING, nats.CONNECTING:
		pingResult.SetPingOutput(
			health.PingCauseUnstable,
			fmt.Sprintf("NATS client '%s' is not fully connected: status=%s", c.Driver.ConnectedUrlRedacted(), status.String()),
		)
	case nats.DRAINING_SUBS, nats.DRAINING_PUBS:
		pingResult.SetPingOutput(
			health.PingCauseOverloaded,
			fmt.Sprintf("NATS client '%s' is draining: status=%s", c.Driver.ConnectedUrlRedacted(), status.String()),
		)
	case nats.CONNECTED:
		fallthrough
	default:
		pingResult.SetPingOutput(
			health.PingCauseOk,
			fmt.Sprintf("NATS client '%s' is connected: status=%s", c.Driver.ConnectedUrlRedacted(), status.String()),
		)
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthDeep)

	err := c.Driver.FlushWithContext(ctx)
	pingResult.StoreComputedLatency(pingDeepAcceptableLatency)
	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to flush: %v", err),
		)
		return pingResult
	}

	return pingResult
}
