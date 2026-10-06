package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/observability"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/rs/zerolog"
)

// WebhookProcessor claims and delivers one due webhook per call.
type WebhookProcessor interface {
	ProcessNext(ctx context.Context) error
}

// WebhookWorker is a bounded pool that drains PENDING webhook_deliveries.
type WebhookWorker struct {
	proc         WebhookProcessor
	poolSize     int
	pollInterval time.Duration
	log          zerolog.Logger
	wg           sync.WaitGroup
}

// NewWebhookWorker starts nothing; call Start with a cancelable context.
func NewWebhookWorker(proc WebhookProcessor, poolSize int, pollInterval time.Duration, log *zerolog.Logger) *WebhookWorker {
	if poolSize < 1 {
		poolSize = 1
	}
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	w := &WebhookWorker{
		proc:         proc,
		poolSize:     poolSize,
		pollInterval: pollInterval,
		log:          zerolog.Nop(),
	}
	if log != nil {
		w.log = *log
	}
	return w
}

// Start runs poolSize loops until ctx is canceled.
func (w *WebhookWorker) Start(ctx context.Context) {
	for id := range w.poolSize {
		w.wg.Add(1)
		go func(workerID int) {
			defer w.wg.Done()
			w.run(observability.WithWorkerID(ctx, workerID))
		}(id)
	}
}

// Wait blocks until all loops started by Start have exited.
func (w *WebhookWorker) Wait() {
	w.wg.Wait()
}

func (w *WebhookWorker) run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		err := w.proc.ProcessNext(ctx)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, service.ErrNoWebhookJob) {
			if waitErr := waitCtx(ctx, w.pollInterval); waitErr != nil {
				return
			}
			continue
		}
		e := w.log.Error().Err(err).Str("component", "webhook_worker")
		if id, ok := observability.WorkerIDFrom(ctx); ok {
			e = e.Int("worker_id", id)
		}
		e.Msg("webhook worker")
		if waitErr := waitCtx(ctx, w.pollInterval); waitErr != nil {
			return
		}
	}
}
