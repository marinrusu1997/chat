package neo4j

import (
	"chat/src/platform/health"
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

const (
	PingTargetName               = "neo4j"
	pingShallowAcceptableLatency = 50 * time.Millisecond
	pingDeepAcceptableLatency    = 150 * time.Second
)

func (c *Client) PingShallow(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	_, err := c.pingSession.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		return tx.Run(ctx, "RETURN 1 AS test", nil)
	})
	pingResult.StoreComputedLatency(pingShallowAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to ping Neo4j database: %v", err),
		)
		return pingResult
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	return c.PingShallow(ctx)
}
