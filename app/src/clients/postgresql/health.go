package postgresql

import (
	"chat/src/platform/health"
	"context"
	"fmt"
	"time"
)

const (
	PingTargetName                       = "postgresql"
	pingShallowAcceptableLatency         = 50 * time.Millisecond
	pingDeepAcceptableLatency            = 150 * time.Second
	replicationLagAcceptedSeconds        = 10.0
	numberOfConnectionsAcceptedThreshold = 200
)

func (c *Client) PingShallow(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	err := c.Driver.Ping(ctx)
	pingResult.StoreComputedLatency(pingShallowAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to ping PostgreSQL database: %v", err),
		)
		return pingResult
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthDeep)

	// # Run checks
	var (
		inRecovery     bool
		numConnections int
		xactCommit     int64
		xactRollback   int64
	)
	conn, err := c.Driver.Acquire(ctx)
	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseInternal,
			fmt.Sprintf("failed to acquire connection from pool: %v", err),
		)
		return pingResult
	}
	defer conn.Release()

	err = conn.QueryRow(ctx, "SELECT pg_is_in_recovery()").Scan(&inRecovery)
	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to query if database is in recovery: %v", err),
		)
		return pingResult
	}

	err = conn.QueryRow(ctx, `
		SELECT numbackends, xact_commit, xact_rollback 
		FROM pg_stat_database
		WHERE datname = current_database()
	`).Scan(&numConnections, &xactCommit, &xactRollback)
	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to query if database stats: %v", err),
		)
		return pingResult
	}

	var lagSeconds *float64
	err = conn.QueryRow(ctx, `
		SELECT EXTRACT(EPOCH FROM now() - pg_last_xact_replay_timestamp()) AS replication_lag
		FROM pg_stat_replication
		LIMIT 1
	`).Scan(&lagSeconds)
	if err != nil {
		// ignore error on primary (no replication)
		lagSeconds = nil
	}

	pingResult.StoreComputedLatency(pingDeepAcceptableLatency)

	// # Evaluate results
	if lagSeconds != nil && *lagSeconds > replicationLagAcceptedSeconds {
		pingResult.SetPingOutput(
			health.PingCauseUnstable,
			fmt.Sprintf("replication lag '%f' exceeds 10s", *lagSeconds),
		)
		return pingResult
	}

	if numConnections > numberOfConnectionsAcceptedThreshold {
		pingResult.SetPingOutput(
			health.PingCauseOverloaded,
			fmt.Sprintf("too many active connections: %d", numConnections),
		)
		return pingResult
	}

	if inRecovery {
		pingResult.SetPingOutput(
			health.PingCauseBadState,
			"node is read-only (in recovery mode)",
		)
		return pingResult
	}

	return pingResult
}
