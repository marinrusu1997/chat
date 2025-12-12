package kafka

import (
	"math"
	"sort"
	"sync"
	"time"
)

type Percentile float32

type TimeoutEstimator struct {
	mtx sync.Mutex

	durationsHead           int
	durationsCircularBuffer []time.Duration

	smoothingFactor     float64
	smoothedPrevTimeout time.Duration

	minTimeout time.Duration
	maxTimeout time.Duration
}

func NewTimeoutEstimator(capacity int, alpha float64, minTimeout, maxTimeout time.Duration) *TimeoutEstimator {
	return &TimeoutEstimator{
		durationsCircularBuffer: make([]time.Duration, capacity),
		smoothingFactor:         alpha,
		minTimeout:              minTimeout,
		maxTimeout:              maxTimeout,
		smoothedPrevTimeout:     (minTimeout + maxTimeout) / 2, // reasonable initial value
	}
}

func (t *TimeoutEstimator) AddSample(duration time.Duration) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	t.durationsCircularBuffer[t.durationsHead] = duration
	t.durationsHead = (t.durationsHead + 1) % cap(t.durationsCircularBuffer)
}

func (t *TimeoutEstimator) EstimateTimeout(percentile Percentile) time.Duration {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	// Copy circular buffer into a temp array in linear order.
	durations := make([]time.Duration, cap(t.durationsCircularBuffer))
	copy(durations, t.durationsCircularBuffer)

	// Sort by duration to compute exact percentiles.
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Compute percentile index.
	idx := percentileIndex(percentile, cap(durations))
	raw := durations[idx]

	// Clamp into min/max bounds.
	clamped := raw
	if clamped < t.minTimeout {
		clamped = t.minTimeout
	}
	if clamped > t.maxTimeout {
		clamped = t.maxTimeout
	}

	// EWMA smoothing.
	smoothed := time.Duration(t.smoothingFactor*float64(t.smoothedPrevTimeout) + (1-t.smoothingFactor)*float64(clamped))
	t.smoothedPrevTimeout = smoothed

	return smoothed
}

func percentileIndex(percentile Percentile, capacity int) int {
	if percentile <= 0 {
		return 0
	}
	if percentile >= 100 {
		return capacity - 1
	}
	rank := (float64(percentile) / 100.0) * float64(capacity-1)
	return int(math.Round(rank))
}
