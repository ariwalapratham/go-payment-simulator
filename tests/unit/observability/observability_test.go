package observability_test

import (
	"context"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/observability"
)

func TestWorkerIDRoundTrip(t *testing.T) {
	t.Parallel()

	if _, ok := observability.WorkerIDFrom(context.Background()); ok {
		t.Fatal("expected missing worker id")
	}
	ctx := observability.WithWorkerID(context.Background(), 3)
	id, ok := observability.WorkerIDFrom(ctx)
	if !ok || id != 3 {
		t.Fatalf("got %d %v", id, ok)
	}
}
