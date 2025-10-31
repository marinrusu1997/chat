package health

import (
	error2 "chat/src/platform/error"
	"chat/src/platform/validation"
	"chat/src/util"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/creasty/defaults"
	"github.com/go-co-op/gocron/v2"
	"github.com/jellydator/ttlcache/v3"
	"github.com/rs/zerolog"
	"github.com/samber/oops"
)

const (
	shallowToPingDelta          = 1 * time.Second
	deepToShadowDeltaMultiplier = 2
)

type CheckFrequencyConfig struct {
	PingTimeout         time.Duration `default:"1s" validate:"min=100000000,max=3000000000"`                             // 100ms to 3s
	ShallowInterval     time.Duration `default:"10s" validate:"min=5000000000,max=60000000000,gtfield=PingTimeout"`      // 5s to 60s
	DeepInterval        time.Duration `default:"1m" validate:"min=30000000000,max=300000000000,gtfield=ShallowInterval"` // 30s to 5min
	DeepEveryNthShallow int8          `default:"5" validate:"gte=1,lte=10"`                                              // 1 to 10
}

type ControllerConfig struct {
	Dependencies   map[string]Pingable  `validate:"required,min=1,max=50,dive,keys,min=3,max=30,alpha,lowercase,endkeys,required"`
	CheckFrequency CheckFrequencyConfig `validate:"required,dive"`
	Logger         zerolog.Logger       `validate:"required"`
}

type pingingStats struct {
	mu               sync.Mutex
	lastDeepPingTime time.Time
	shallowCount     int8
	checkFrequency   CheckFrequencyConfig
}

type Controller struct {
	dependencies map[string]Pingable
	cache        *ttlcache.Cache[string, PingResult]
	stats        pingingStats
	scheduler    gocron.Scheduler
	logger       zerolog.Logger
}

func NewController(config *ControllerConfig) (*Controller, error) {
	if err := config.setup(); err != nil {
		return nil, fmt.Errorf("failed to create health controller because config setup failed: %v", err)
	}

	scheduler, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create health controller because scheduler creation failed: %v", err)
	}

	controller := &Controller{
		dependencies: config.Dependencies,
		cache:        ttlcache.New[string, PingResult](),
		scheduler:    scheduler,
		stats:        pingingStats{checkFrequency: config.CheckFrequency},
		logger:       config.Logger,
	}

	_, err = controller.scheduler.NewJob(
		gocron.DurationJob(controller.stats.checkFrequency.ShallowInterval),
		gocron.NewTask(func(c *Controller) {
			var depth = PingDepthShallow
			if c.stats.shouldDeepPing() {
				depth = PingDepthDeep
			}
			c.pingAndCache(depth)
		}, &controller))
	if err != nil {
		return nil, fmt.Errorf("failed to create health controller because scheduler job creation failed: %v", err)
	}

	return controller, nil
}

func (c *Controller) Start() {
	c.pingAndCache(PingDepthDeep)
	c.scheduler.Start()
}

func (c *Controller) Stop() {
	err := c.scheduler.Shutdown()
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to shutdown health controller")
	}
}

func (c *Controller) PingNow(depth PingDepth) *ttlcache.Cache[string, PingResult] {
	c.pingAndCache(depth)
	return c.cache
}

func (c *Controller) GetCurrentHealth() *ttlcache.Cache[string, PingResult] {
	return c.cache
}

func (c *Controller) GetDependencyHealth(name string) PingResult {
	return c.cache.Get(name).Value()
}

func (c *Controller) Healthy() bool {
	var healthy = true
	c.cache.Range(func(item *ttlcache.Item[string, PingResult]) bool {
		if item.Value().Status == PingStatusUnhealthy {
			healthy = false
			return false
		}
		return true
	})
	return healthy
}

func (c *Controller) pingAndCache(depth PingDepth) {
	ctx, cancel := context.WithTimeout(context.Background(), c.stats.checkFrequency.PingTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(c.dependencies))
	for name, dep := range c.dependencies {
		name, dep := name, dep

		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					c.logger.Error().Interface("recover", r).Msgf("panic in ping for %s", name)
				}
			}()

			ping := dep.PingShallow
			if depth == PingDepthDeep {
				ping = dep.PingDeep
			}

			result := ping(ctx)
			c.cache.Set(name, result, ttlcache.NoTTL)

			if !result.Healthy() {
				log := c.logger.Error
				if result.Degraded() {
					log = c.logger.Warn
				}
				log().Msgf("'%s' is unhealthy:\n%s", name, result.PrettyJSON())
			}
		}()
	}
	wg.Wait()

	c.stats.update(depth)
}

func (s *pingingStats) update(depth PingDepth) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if depth == PingDepthDeep {
		s.lastDeepPingTime = time.Now()
		s.shallowCount = 0
	} else {
		s.shallowCount++
	}
}

func (s *pingingStats) shouldDeepPing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.shallowCount >= s.checkFrequency.DeepEveryNthShallow ||
		time.Now().After(s.lastDeepPingTime.Add(s.checkFrequency.DeepInterval))
}

func (c *ControllerConfig) setup() error {
	errorb := oops.
		In(util.GetFunctionName()).
		Code(error2.ECONFIG)

	if err := defaults.Set(c); err != nil {
		return errorb.Wrapf(err, "failed to set defaults")
	}

	if err := validation.Instance.Struct(c); err != nil {
		return errorb.Wrapf(err, "failed to validate")
	}

	if c.CheckFrequency.ShallowInterval-c.CheckFrequency.PingTimeout < shallowToPingDelta {
		return errorb.
			Errorf("ShallowInterval (%s) must be at least %s greater than PingTimeout (%s)",
				c.CheckFrequency.ShallowInterval, shallowToPingDelta, c.CheckFrequency.PingTimeout)
	}
	if c.CheckFrequency.DeepInterval < c.CheckFrequency.ShallowInterval*deepToShadowDeltaMultiplier {
		return errorb.
			Errorf("DeepInterval (%s) must greater than %d x ShallowInterval (%s)",
				c.CheckFrequency.DeepInterval, deepToShadowDeltaMultiplier, c.CheckFrequency.ShallowInterval)
	}

	return nil
}
