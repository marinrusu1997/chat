package email

import (
	"chat/src/platform/health"
	"context"
	"fmt"
	"time"
)

const (
	PingTargetName               = "email"
	pingShallowAcceptableLatency = 100 * time.Millisecond
)

func (c *Client) PingShallow(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	err := c.pool.Healthy(ctx)
	pingResult.StoreComputedLatency(pingShallowAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseNetwork,
			fmt.Sprintf("failed to shallow ping: %v", err),
		)
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	return c.PingShallow(ctx)
}
