package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
)

var ErrWorkerPoolNotRunning = errors.New("worker pool is not running")

type Request struct {
	SendOptions SendEmailOptions
	Response    chan error
}

type worker struct {
	id     uint8
	health chan healthRequest
	client *smtpClient
	logger *zerolog.Logger
}

type healthRequest struct {
	response chan error
}

type workerPool struct {
	requestsQueue chan Request
	workers       []*worker
	logger        *zerolog.Logger
	running       atomic.Bool
	runningWg     sync.WaitGroup
}

type WorkerPoolOptions struct {
	SMTPClientOptions *SMTPClientOptions
	Logger            *zerolog.Logger
	NumWorkers        uint8
	QueueSize         uint16
}

func newWorkerPool(opts WorkerPoolOptions) *workerPool {
	opts.SMTPClientOptions.Logger = opts.Logger
	opts.SMTPClientOptions.TLSConfig.ServerName = opts.SMTPClientOptions.Host

	pool := &workerPool{
		requestsQueue: make(chan Request, opts.QueueSize),
		workers:       make([]*worker, opts.NumWorkers),
		logger:        opts.Logger,
	}

	for i := uint8(0); i < opts.NumWorkers; i++ { //nolint:intrange // uint8 is sufficient for number of workers
		pool.workers[i] = &worker{
			id:     i,
			health: make(chan healthRequest),
			client: newSMTPClient(opts.SMTPClientOptions),
			logger: opts.Logger,
		}
	}

	return pool
}

func (p *workerPool) Start(ctx context.Context) error {
	if p.running.Load() {
		p.logger.Warn().Msg("worker pool is already started")
		return nil
	}

	// Initialization: establish SMTP connections for all workers
	for i, worker := range p.workers {
		if err := worker.client.Connect(ctx); err != nil {
			// rollback
			for j := i - 1; j >= 0; j-- {
				if err := p.workers[j].client.Disconnect(); err != nil {
					p.logger.Error().Err(err).Msgf("failed to close SMTP client for worker '%d' during cleanup", p.workers[j].id)
				}
			}
			// return error
			return fmt.Errorf("failed to connect SMTP client for worker '%d': %w", worker.id, err)
		}
	}

	// Assign job processing goroutines
	for _, worker := range p.workers {
		p.runningWg.Go(func() {
			worker.drainRequestsQueue(p.requestsQueue)
		})
	}

	p.running.Store(true)
	return nil
}

func (p *workerPool) Stop() {
	if !p.running.Swap(false) {
		p.logger.Warn().Msg("worker pool is already stopped")
		return
	}
	close(p.requestsQueue)
	p.runningWg.Wait()
}

func (p *workerPool) Submit(request Request) error {
	if !p.running.Load() {
		return ErrWorkerPoolNotRunning
	}

	p.requestsQueue <- request

	if request.Response == nil {
		return nil
	}
	return <-request.Response
}

func (p *workerPool) Healthy(ctx context.Context) error {
	if !p.running.Load() {
		return ErrWorkerPoolNotRunning
	}

	herrors := make([]error, len(p.workers)) //nolint:makezero // concurrently write safely to different indexes

	var wg sync.WaitGroup
	for i, worker := range p.workers {
		wg.Go(func() {
			herrors[i] = worker.healthy(ctx)
		})
	}
	wg.Wait()

	var builder strings.Builder
	for i, err := range herrors {
		if err == nil {
			continue
		}

		msg := fmt.Sprintf("failed healthcheck of worker '%d': %v; ", i, err)
		if _, err := builder.WriteString(msg); err != nil {
			p.logger.Error().Err(err).Msgf("failed to write message into error message string builder: '%s'", msg)
		}
	}

	if builder.Len() == 0 {
		return nil
	}

	return errors.New(builder.String()) //nolint:err113 // we are good here
}

func (w *worker) drainRequestsQueue(requests <-chan Request) {
	for {
		select {
		case request := <-w.health:
			request.response <- w.client.driver.Noop()
			close(request.response)

		case request, ok := <-requests:
			if !ok {
				w.shutdown() // Job channel closed and drained → clean shutdown
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), w.client.opts.SendTimeout)
			err := w.client.SendEmail(ctx, request.SendOptions)
			cancel()

			if err != nil {
				err = fmt.Errorf("worker '%d' failed to send email: %w", w.id, err)
			}

			if request.Response != nil {
				request.Response <- err
				close(request.Response)
			} else if err != nil {
				w.logger.Error().Err(err).Msgf("failed to send email")
			}
		}
	}
}

func (w *worker) healthy(ctx context.Context) error {
	req := healthRequest{
		response: make(chan error, 1),
	}

	// ask worker to do a health check
	select {
	case w.health <- req:
		// OK
	case <-ctx.Done():
		return ctx.Err() //nolint:wrapcheck // upper layer will handle wrapping
	}

	// wait for response or timeout
	select {
	case err := <-req.response:
		return err
	case <-ctx.Done():
		return ctx.Err() //nolint:wrapcheck // upper layer will handle wrapping
	}
}

func (w *worker) shutdown() {
	if err := w.client.Disconnect(); err != nil {
		w.logger.Error().Err(err).Msgf("failed to close SMTP client of worker '%d' during shutdown", w.id)
	}
}
