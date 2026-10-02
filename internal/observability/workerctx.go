package observability

import "context"

type workerIDKey struct{}

// WithWorkerID marks ctx as owned by a payment-worker goroutine (0 .. poolSize-1).
func WithWorkerID(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, workerIDKey{}, id)
}

// WorkerIDFrom returns the id set by WithWorkerID.
func WorkerIDFrom(ctx context.Context) (int, bool) {
	id, ok := ctx.Value(workerIDKey{}).(int)
	return id, ok
}
