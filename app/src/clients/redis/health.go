package redis

import (
	"chat/src/platform/health"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	PingTargetName               = "redis"
	pingShallowAcceptableLatency = 25 * time.Millisecond
	pingDeepAcceptableLatency    = 50 * time.Second
)

func (c *Client) PingShallow(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthShallow)

	err := c.Driver.Ping(ctx).Err()
	pingResult.StoreComputedLatency(pingShallowAcceptableLatency)

	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("redis ping failed: %v", err),
		)
		return pingResult
	}

	return pingResult
}

func (c *Client) PingDeep(ctx context.Context) health.PingResult {
	pingResult := health.NewHealthyPingResult(PingTargetName, health.PingDepthDeep)

	testKey := uniquePingKey()
	const testValue = "ok"

	err := c.Driver.Set(ctx, testKey, testValue, 1*time.Second).Err()
	pingResult.StoreComputedLatency(pingDeepAcceptableLatency)
	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("redis set test key '%s' failed: %v", testKey, err),
		)
		return pingResult
	}

	val, err := c.Driver.Get(ctx, testKey).Result()
	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("redis get test key '%s' failed: %v", testKey, err),
		)
		return pingResult
	}
	if val != testValue {
		pingResult.SetPingOutput(
			health.PingCauseBadResponse,
			fmt.Sprintf("redis get test key '%s' returned unexpected value '%s' (expected '%s')", testKey, val, testValue),
		)
		return pingResult
	}

	info, err := c.Driver.Info(ctx, "server").Result()
	if err != nil {
		pingResult.SetPingOutput(
			health.PingCauseFromRequestError(err),
			fmt.Sprintf("redis get server info failed: %v", err),
		)
		return pingResult
	}
	if info != "" && strings.Contains(info, "loading:1") {
		pingResult.SetPingOutput(
			health.PingCauseUnstable,
			"redis is loading dataset into memory",
		)
		return pingResult
	}

	return pingResult
}

func uniquePingKey() string {
	t := time.Now()
	entropy := ulid.Monotonic(rand.New(rand.NewSource(t.UnixNano())), 0) //nolint:gosec // We use weak random for lightweight checks
	return "ping:" + ulid.MustNew(ulid.Timestamp(t), entropy).String()
}
