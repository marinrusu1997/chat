package lifecycle

import (
	error2 "chat/src/platform/error"
	"chat/src/platform/validation"
	"chat/src/util"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creasty/defaults"
	"github.com/quipo/dependencysolver"
	"github.com/rs/zerolog"
	"github.com/samber/oops"
)

type ServiceLifecycle interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context)
}

type Controller struct {
	services map[string]ServiceLifecycle
	layers   [][]string
	timeouts ControllerTimeoutsOptions
	logger   zerolog.Logger
}

type ControllerTimeoutsOptions struct {
	Startup            time.Duration            `default:"15s" validate:"required,min=1000000000,max=60000000000"`                            // 1s to 60s
	StartupPerService  map[string]time.Duration `validate:"dive,keys,min=1,max=50,alphanum,lowercase,endkeys,min=1000000000,max=60000000000"` // 1s to 60s
	Shutdown           time.Duration            `default:"15s" validate:"required,min=1000000000,max=60000000000"`                            // 1s to 60s
	ShutdownPerService map[string]time.Duration `validate:"dive,keys,min=1,max=50,alphanum,lowercase,endkeys,min=1000000000,max=60000000000"` // 1s to 60s
}

type ControllerOptions struct {
	Services     map[string]ServiceLifecycle `validate:"required,min=1,max=50,dive,keys,min=1,max=50,alphanum,lowercase,endkeys,required"`
	Dependencies map[string][]string         `validate:"omitempty,min=1,max=50,dive,keys,min=1,max=50,alphanum,lowercase,endkeys,required,dive,min=1,max=50,alphanum,lowercase"`
	Timeouts     ControllerTimeoutsOptions   `validate:"required"`
	Logger       zerolog.Logger              `validate:"required"`
}

func NewController(options ControllerOptions) (*Controller, error) {
	errorb := oops.In("Lifecycle Controller constructor")

	if err := options.setup(); err != nil {
		return nil, errorb.Wrap(err)
	}

	graph := make([]dependencysolver.Entry, 0, len(options.Services))
	for svcName := range options.Services {
		svcDependencies := make([]string, 0)
		if options.Dependencies != nil {
			if dependencies, ok := options.Dependencies[svcName]; ok {
				svcDependencies = dependencies
			}
		}
		graph = append(graph, dependencysolver.Entry{ID: svcName, Deps: svcDependencies})
	}
	if dependencysolver.HasCircularDependency(graph) {
		return nil, errorb.Errorf("circular dependency detected in dependencies services: %v", graph)
	}

	return &Controller{
		services: options.Services,
		layers:   dependencysolver.LayeredTopologicalSort(graph),
		timeouts: options.Timeouts,
		logger:   options.Logger,
	}, nil
}

func (lc *Controller) Start(ctx context.Context) error {
	var startedLayers [][]string

	for layerIdx, layer := range lc.layers {
		var (
			wg        sync.WaitGroup
			succeeded = make([]string, len(layer), len(layer))
			failed    atomic.Bool
		)

		for svcIdx, svcName := range layer {
			svc := lc.services[svcName]
			wg.Add(1)

			go func() {
				defer wg.Done()

				svcCtx, cancel := context.WithTimeout(ctx, lc.startupTimeout(svcName))
				defer cancel()

				if err := svc.Start(svcCtx); err != nil {
					lc.logger.Error().Err(err).Msgf("'%s' failed to start", svcName)
					failed.Store(true)
					return
				}

				succeeded[svcIdx] = svcName
				lc.logger.Info().Msgf("Started service '%s'", svcName)
			}()
		}
		wg.Wait()

		if failed.Load() {
			rollbackCtx := context.Background()

			lc.rollbackLayer(rollbackCtx, succeeded)
			lc.rollback(rollbackCtx, startedLayers)

			return fmt.Errorf("startup failed in layer %d; rollback performed", layerIdx)
		}

		startedLayers = append(startedLayers, layer)
	}

	lc.logger.Info().Msg("All services started successfully")
	return nil
}

func (lc *Controller) Stop(ctx context.Context) {
	lc.rollback(ctx, lc.layers) // clever reuse of rollback logic
}

func (lc *Controller) rollback(ctx context.Context, startedLayers [][]string) {
	for i := len(startedLayers) - 1; i >= 0; i-- {
		lc.rollbackLayer(ctx, startedLayers[i])
	}
}

func (lc *Controller) rollbackLayer(ctx context.Context, layer []string) {
	if len(layer) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, svcName := range layer {
		svc, ok := lc.services[svcName] // sometimes layer might contain "holes" for services that failed to start
		if !ok {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			svcCtx, cancel := context.WithTimeout(ctx, lc.shutdownTimeout(svcName))
			defer cancel()

			svc.Stop(svcCtx)
			lc.logger.Info().Msgf("Stopped service '%s'", svcName)
		}()
	}
	wg.Wait()
}

func (lc *Controller) startupTimeout(service string) time.Duration {
	if lc.timeouts.StartupPerService != nil {
		if timeout, ok := lc.timeouts.StartupPerService[service]; ok {
			return timeout
		}
	}
	return lc.timeouts.Startup
}

func (lc *Controller) shutdownTimeout(service string) time.Duration {
	if lc.timeouts.ShutdownPerService != nil {
		if timeout, ok := lc.timeouts.ShutdownPerService[service]; ok {
			return timeout
		}
	}
	return lc.timeouts.Shutdown
}

func (co *ControllerOptions) setup() error {
	errorb := oops.
		In(util.GetFunctionName()).
		Code(error2.ECONFIG)

	if err := defaults.Set(co); err != nil {
		return errorb.Wrapf(err, "failed to set defaults")
	}

	if err := validation.Instance.Struct(co); err != nil {
		return errorb.Wrapf(err, "failed to validate")
	}

	if co.Dependencies != nil {
		for svcName, svcDependencies := range co.Dependencies {
			if _, ok := co.Services[svcName]; !ok {
				return errorb.
					Errorf(
						"invalid dependencies configuration: service '%s' in dependencies is not defined in 'Services'", svcName,
					)
			}
			for _, svcDependency := range svcDependencies {
				if _, ok := co.Services[svcDependency]; !ok {
					return errorb.
						Errorf(
							"invalid dependencies configuration: dependency '%s' for service '%s' is not defined in 'Services'", svcDependency, svcName,
						)
				}
			}
		}
	}

	if co.Timeouts.StartupPerService != nil {
		for svcName := range co.Timeouts.StartupPerService {
			if _, ok := co.Services[svcName]; !ok {
				return errorb.
					Errorf(
						"invalid startup timeouts configuration: service '%s' in 'StartupPerService' is not defined in 'Services'", svcName,
					)
			}
		}
	}

	if co.Timeouts.ShutdownPerService != nil {
		for svcName := range co.Timeouts.ShutdownPerService {
			if _, ok := co.Services[svcName]; !ok {
				return errorb.
					Errorf(
						"invalid shutdown timeouts configuration: service '%s' in 'ShutdownPerService' is not defined in 'Services'", svcName,
					)
			}
		}
	}

	return nil
}
