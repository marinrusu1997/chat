package kafka

import (
	"chat/src/platform/health"
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kmsg"
)

const (
	PingTargetName               = "kafka"
	pingShallowAcceptableLatency = 50 * time.Millisecond
	pingDeepAcceptableLatency    = 150 * time.Millisecond
)

func (c *Client) PingShallow(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	err := c.Driver.Ping(ctx)
	pingResult.StoreComputedLatency(pingShallowAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to shallow ping: %v", err),
		)
		return pingResult
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthDeep)

	var req kmsg.MetadataRequest
	req.Default()
	req.Topics = make([]kmsg.MetadataRequestTopic, 0)
	metadata, err := c.Driver.RequestCachedMetadata(ctx, &req, -1)
	pingResult.StoreComputedLatency(pingDeepAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("failed to request cached metadata: %v", err),
		)
		return pingResult
	}

	if len(metadata.Brokers) < len(c.Driver.SeedBrokers()) {
		pingResult.SetPingOutput(
			health.PingCauseBadState,
			fmt.Sprintf("expected at least %d brokers, but got %d", len(c.Driver.SeedBrokers()), len(metadata.Brokers)),
		)
		return pingResult
	}

	if metadata.ControllerID < 0 {
		pingResult.SetPingOutput(
			health.PingCauseBadState,
			fmt.Sprintf("no controller assigned, got: %d", metadata.ControllerID),
		)
		return pingResult
	}

	if metadata.ErrorCode > 0 {
		pingResult.SetPingOutput(
			health.PingCauseBadResponse,
			fmt.Sprintf("metadata response contains error code: %d", metadata.ErrorCode),
		)
		return pingResult
	}

	return pingResult
}
