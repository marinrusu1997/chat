package routing

import (
	"chat/src/platform/validation"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/creasty/defaults"
)

type percentile float32

type timeoutEstimator struct {
	mtx sync.Mutex

	durationsHead           int
	durationsCircularBuffer []time.Duration

	smoothingFactor     float64
	smoothedPrevTimeout time.Duration

	minTimeout time.Duration
	maxTimeout time.Duration
}

type timeoutEstimatorOptions struct {
	Capacity        int           `validate:"required,min=100,max=1000" default:"500"`
	SmoothingFactor float64       `validate:"required,gt=0,lt=1" default:"0.7"`
	MinTimeout      time.Duration `validate:"required,min=100000000,max=1000000000" default:"500ms"`                       // 100ms to 1s
	MaxTimeout      time.Duration `validate:"required,min=1000000000,max=10000000000,gtfield=MinTimeout" default:"5000ms"` // 1s to 10s
}

func newTimeoutEstimator(options *timeoutEstimatorOptions) (*timeoutEstimator, error) {
	if options == nil {
		options = &timeoutEstimatorOptions{}
	}

	if err := defaults.Set(options); err != nil {
		return nil, fmt.Errorf("failed to set config defaults: %w", err)
	}
	if err := validation.Instance.Struct(options); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	return &timeoutEstimator{
		durationsCircularBuffer: make([]time.Duration, options.Capacity),
		smoothingFactor:         options.SmoothingFactor,
		minTimeout:              options.MinTimeout,
		maxTimeout:              options.MaxTimeout,
		smoothedPrevTimeout:     (options.MinTimeout + options.MaxTimeout) / 2, // reasonable initial value
	}, nil
}

func (t *timeoutEstimator) AddSample(duration time.Duration) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	t.durationsCircularBuffer[t.durationsHead] = duration
	t.durationsHead = (t.durationsHead + 1) % cap(t.durationsCircularBuffer)
}

func (t *timeoutEstimator) EstimateTimeout(percentile percentile) time.Duration {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	// Copy circular buffer into a temp array in linear order.
	durations := make([]time.Duration, len(t.durationsCircularBuffer))
	copy(durations, t.durationsCircularBuffer)

	// Sort by duration to compute exact percentiles.
	slices.Sort(durations)

	// Compute percentile index.
	idx := t.percentileIndex(percentile)
	raw := durations[idx]

	// Clamp into min/max bounds.
	clamped := max(t.minTimeout, min(raw, t.maxTimeout))

	// EWMA smoothing.
	smoothed := time.Duration(t.smoothingFactor*float64(t.smoothedPrevTimeout) + (1-t.smoothingFactor)*float64(clamped))
	t.smoothedPrevTimeout = smoothed

	return smoothed
}

func (t *timeoutEstimator) percentileIndex(percentile percentile) int {
	if percentile <= 0 {
		return 0
	}
	if percentile >= 100 {
		return len(t.durationsCircularBuffer) - 1
	}
	rank := (float64(percentile) / 100.0) * float64(len(t.durationsCircularBuffer)-1)
	return int(math.Round(rank))
}
