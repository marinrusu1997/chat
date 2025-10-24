package neo4j

import (
	"chat/src/platform/health"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

const (
	pingTargetName               = "neo4j"
	pingShallowAcceptableLatency = 50 * time.Millisecond
	pingDeepAcceptableLatency    = 150 * time.Second
)

func (c *Client) PingShallow(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(pingTargetName, health.PingDepthShallow)

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
	pingResult := health.NewHealthyPingResult(pingTargetName, health.PingDepthDeep)

	clusterRole, err := c.pingSession.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, "CALL dbms.cluster.role() YIELD role RETURN role", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to obtain cluster role: %v", err)
		}
		if res.Next(ctx) {
			role, found := res.Record().Get("role")
			if !found {
				return nil, errors.New("cluster role not found in record")
			}
			return role, nil
		}
		return nil, res.Err()
	})
	pingResult.StoreComputedLatency(pingDeepAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("deep ping failed: %v", err),
		)
		return pingResult
	}

	if clusterRole == "" {
		pingResult.SetPingOutput(health.PingCauseBadState, "unable to determine cluster role")
		return pingResult
	}

	return pingResult
}
