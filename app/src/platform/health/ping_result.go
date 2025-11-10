package health

import (
	"encoding/json"
	"fmt"
	"time"
)

type PingResult struct {
	Target    string     `json:"target"`
	Depth     PingDepth  `json:"depth"`
	Status    PingStatus `json:"status"`
	Cause     PingCause  `json:"cause"`
	Details   string     `json:"details"`
	Latency   string     `json:"latency"`
	CheckedAt time.Time  `json:"checked_at"`
}

func NewHealthyPingResult(target string, depth PingDepth) PingResult {
	result := PingResult{
		Target:    target,
		Depth:     depth,
		CheckedAt: time.Now(),
	}
	result.SetPingOutput(PingCauseOk, "ok")
	return result
}

func (r *PingResult) SetPingOutput(cause PingCause, details string) {
	r.Status = cause.ToStatus()
	r.Cause = cause
	r.Details = details
}

func (r *PingResult) StoreComputedLatency(acceptableLatency time.Duration) {
	latency := time.Since(r.CheckedAt)
	r.Latency = latency.String()

	if latency > acceptableLatency {
		r.SetPingOutput(
			PingCauseUnstable,
			fmt.Sprintf("ping latency %s exceeds acceptable latency %s", r.Latency, acceptableLatency),
		)
	}
}

func (r *PingResult) PrettyJSON() string {
	bytes, err := json.MarshalIndent(r, "", "\t")
	if err != nil {
		return ""
	}
	return string(bytes)
}

func (r *PingResult) Healthy() bool {
	return r.Status == PingStatusHealthy
}

func (r *PingResult) Degraded() bool {
	return r.Status == PingStatusDegraded
}
