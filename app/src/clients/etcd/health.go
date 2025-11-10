package etcd

import (
	"chat/src/platform/health"
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"google.golang.org/grpc/connectivity"
)

const (
	PingTargetName            = "etcd"
	pingDeepAcceptableLatency = 150 * time.Second
	acceptableDBUsageRatio    = 0.8
)

func (c *Client) PingShallow(_ context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	conn := c.Driver.ActiveConnection()
	if conn == nil {
		pingResult.SetPingOutput(health.PingCauseInternal, "no active gRPC connection")
		return pingResult
	}

	state := conn.GetState()
	if state != connectivity.Connecting && state != connectivity.Ready && state != connectivity.Idle {
		pingResult.SetPingOutput(
			health.PingCauseNetwork,
			"gRPC connection in bad state: "+state.String(),
		)
		return pingResult
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthDeep)

	conn := c.Driver.ActiveConnection()
	if conn == nil {
		pingResult.SetPingOutput(health.PingCauseInternal, "no active gRPC connection")
		return pingResult
	}

	target := c.config.Endpoints[rand.IntN(len(c.config.Endpoints))] //nolint:gosec // random selection for load distribution
	res, err := c.Driver.Status(ctx, target)
	pingResult.StoreComputedLatency(pingDeepAcceptableLatency)
	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("etcd status check failed: %v", err),
		)
		return pingResult
	}

	if len(res.Errors) > 0 {
		pingResult.SetPingOutput(
			health.PingCauseUnstable,
			fmt.Sprintf("etcd status contains these errors: %v", res.Errors),
		)
		return pingResult
	}

	dbUsage := float64(res.DbSizeInUse) / float64(res.DbSizeQuota)
	if dbUsage > acceptableDBUsageRatio {
		pingResult.SetPingOutput(
			health.PingCauseUnstable,
			fmt.Sprintf(
				"etcd database size in use is high: %d bytes used out of %d bytes quota (%.0f%%)",
				res.DbSizeInUse,
				res.DbSizeQuota,
				dbUsage*100,
			),
		)
		return pingResult
	}

	return pingResult
}
