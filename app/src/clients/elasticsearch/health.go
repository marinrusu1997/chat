package elasticsearch

import (
	"chat/src/platform/health"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const (
	PingTargetName                      = "elasticsearch"
	pingShallowAcceptableLatency        = 50 * time.Millisecond
	pingDeepAcceptableLatency           = 150 * time.Second
	degradedNumberOfPendingTasks        = 100
	degradedNumberOfInFlightFetch       = 50
	degradedTaskMaxWaitingInQueueMillis = 2000
)

type clusterHealthResponse struct {
	ClusterName                 string  `json:"cluster_name"`
	Status                      string  `json:"status"` // "green", "yellow", "red"
	TimedOut                    bool    `json:"timed_out"`
	NumberOfNodes               int     `json:"number_of_nodes"`
	NumberOfDataNodes           int     `json:"number_of_data_nodes"`
	ActivePrimaryShards         int     `json:"active_primary_shards"`
	ActiveShards                int     `json:"active_shards"`
	RelocatingShards            int     `json:"relocating_shards"`
	InitializingShards          int     `json:"initializing_shards"`
	UnassignedShards            int     `json:"unassigned_shards"`
	UnassignedPrimaryShards     int     `json:"unassigned_primary_shards"`
	DelayedUnassignedShards     int     `json:"delayed_unassigned_shards"`
	NumberOfPendingTasks        int     `json:"number_of_pending_tasks"`
	NumberOfInFlightFetch       int     `json:"number_of_in_flight_fetch"`
	TaskMaxWaitingInQueueMillis int64   `json:"task_max_waiting_in_queue_millis"`
	ActiveShardsPercent         float64 `json:"active_shards_percent_as_number"`
}

func (c *Client) PingShallow(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	res, err := c.Driver.Ping(c.Driver.Ping.WithContext(ctx))
	pingResult.StoreComputedLatency(pingShallowAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to shallow ping: %v", err),
		)
		return pingResult
	}

	if res.IsError() {
		pingResult.SetPingOutput(health.PingCauseBadResponse, res.String())
		return pingResult
	} else if res.Body != nil {
		if err := res.Body.Close(); err != nil {
			c.logger.Warn().Err(err).Msg("failed to close shallow ping response body")
		}
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthDeep)

	res, err := c.Driver.Cluster.Health(c.Driver.Cluster.Health.WithContext(ctx))
	pingResult.StoreComputedLatency(pingDeepAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to obtain cluster health: %v", err),
		)
		return pingResult
	}

	if res.IsError() || res.Body == nil {
		pingResult.SetPingOutput(
			health.PingCauseBadResponse,
			fmt.Sprintf("cluster health API returned error: %s", res.String()),
		)
		return pingResult
	}

	rawBody, err := io.ReadAll(res.Body)
	if err := res.Body.Close(); err != nil {
		c.logger.Warn().Err(err).Msg("failed to close cluster health response body")
	}

	var clusterHealth clusterHealthResponse
	if err = json.Unmarshal(rawBody, &clusterHealth); err != nil {
		pingResult.SetPingOutput(
			health.PingCauseBadResponse,
			fmt.Sprintf("cluster health API response can't be decoded: %v ; raw body %s", err, rawBody),
		)
		return pingResult
	}

	pingCause := clusterHealthToPingCause(&clusterHealth)
	if pingCause != health.PingCauseOk {
		pingResult.SetPingOutput(
			pingCause,
			fmt.Sprintf("cluster health status: %s", clusterHealth.Status),
		)
		return pingResult
	}

	return pingResult
}

func clusterHealthToPingCause(ch *clusterHealthResponse) health.PingCause {
	if ch.TimedOut {
		return health.PingCauseTimeout
	}

	if ch.UnassignedPrimaryShards > 0 || ch.Status == "red" {
		return health.PingCauseBadState
	}

	if ch.Status == "yellow" ||
		ch.RelocatingShards > 0 ||
		ch.InitializingShards > 0 ||
		ch.DelayedUnassignedShards > 0 ||
		ch.NumberOfPendingTasks > degradedNumberOfPendingTasks ||
		ch.NumberOfInFlightFetch > degradedNumberOfInFlightFetch ||
		ch.TaskMaxWaitingInQueueMillis > degradedTaskMaxWaitingInQueueMillis ||
		ch.ActiveShardsPercent < 100 ||
		ch.UnassignedShards > 0 {
		return health.PingCauseOverloaded
	}

	return health.PingCauseOk
}
