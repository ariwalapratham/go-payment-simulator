package worker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/ariwalapratham/go-payment-simulator/internal/worker"
)

type countingProc struct {
	n atomic.Int32
}

func (c *countingProc) ProcessNext(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.n.Add(1)
	return service.ErrNoJob
}

func TestPaymentWorkerStopsOnCancel(t *testing.T) {
	t.Parallel()

	proc := &countingProc{}
	w := worker.NewPaymentWorker(proc, 2, 10*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	time.Sleep(40 * time.Millisecond)
	cancel()
	w.Wait()
	if proc.n.Load() < 1 {
		t.Fatal("expected at least one ProcessNext")
	}
}
