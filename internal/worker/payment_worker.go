// Package worker runs background loops. The payment worker polls Postgres as the authorize queue.
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

const defaultPollInterval = 200 * time.Millisecond

// Processor claims and authorizes one due payment per call.
type Processor interface {
	ProcessNext(ctx context.Context) error
}

// PaymentWorker is a bounded pool of goroutines that drain PENDING payments.
type PaymentWorker struct {
	proc         Processor
	poolSize     int
	pollInterval time.Duration
	log          zerolog.Logger
	wg           sync.WaitGroup
}

// NewPaymentWorker starts nothing; call Start with a cancelable context.
func NewPaymentWorker(proc Processor, poolSize int, pollInterval time.Duration, log *zerolog.Logger) *PaymentWorker {
	if poolSize < 1 {
		poolSize = 1
	}
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	w := &PaymentWorker{
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

// Start runs poolSize loops until ctx is canceled. Wait after cancel to drain in-flight work.
func (w *PaymentWorker) Start(ctx context.Context) {
	for id := range w.poolSize {
		w.wg.Add(1)
		go func(workerID int) {
			defer w.wg.Done()
			w.run(observability.WithWorkerID(ctx, workerID))
		}(id)
	}
}

// Wait blocks until all loops started by Start have exited.
func (w *PaymentWorker) Wait() {
	w.wg.Wait()
}

func (w *PaymentWorker) run(ctx context.Context) {
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
		if errors.Is(err, service.ErrNoJob) {
			if waitErr := waitCtx(ctx, w.pollInterval); waitErr != nil {
				return
			}
			continue
		}
		e := w.log.Error().Err(err).Str("component", "payment_worker")
		if id, ok := observability.WorkerIDFrom(ctx); ok {
			e = e.Int("worker_id", id)
		}
		e.Msg("payment worker")
		if waitErr := waitCtx(ctx, w.pollInterval); waitErr != nil {
			return
		}
	}
}

func waitCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
