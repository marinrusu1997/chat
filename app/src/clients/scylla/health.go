package scylla

import (
	"chat/src/platform/health"
	"context"
	"fmt"
	"time"
)

const (
	PingTargetName               = "scylla"
	pingShallowAcceptableLatency = 50 * time.Millisecond
	pingDeepAcceptableLatency    = 150 * time.Millisecond
)

func (c *Client) PingShallow(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	err := c.Driver.Query("SELECT now() FROM system.local").WithContext(ctx).Exec()
	pingResult.StoreComputedLatency(pingShallowAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to ping ScyllaDB: %v", err),
		)
		return pingResult
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthDeep)

	var peersOfCurrentNode int
	err := c.Driver.Query("SELECT COUNT(*) FROM system.peers").WithContext(ctx).Scan(&peersOfCurrentNode)
	pingResult.StoreComputedLatency(pingDeepAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to select count of scylladb system peers: %v", err),
		)
		return pingResult
	}
	if peersOfCurrentNode < len(c.Driver.GetHosts())-1 {
		pingResult.SetPingOutput(
			health.PingCauseUnstable,
			fmt.Sprintf("not enough peers visible in system.peers (possible isolation): visible '%d' (expected %d)",
				peersOfCurrentNode, len(c.Driver.GetHosts()),
			),
		)
		return pingResult
	}

	return pingResult
}
