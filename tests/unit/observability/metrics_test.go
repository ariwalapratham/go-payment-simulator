package observability_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/observability"
	"github.com/rs/zerolog"
)

func TestMetricsNopDoesNotPanic(t *testing.T) {
	t.Parallel()

	var m *observability.Metrics
	m.ObserveAuthorizeAttempt("SUCCESS", "AUTHORIZED", "m", "p")
	m.ObserveAuthorizeRetry()
	m.ObserveClaimLost()
	m.ObserveWebhookDelivered()
	m.ObserveWebhookRetry()
	m.ObserveWebhookFailed()
	observability.NewMetrics(nil).ObserveAuthorizeRetry()
	wait := observability.StartQueueReporter(context.Background(), observability.NewMetrics(nil), nil, time.Millisecond, nil)
	wait()
}

func TestMetricsRecordsCustomNames(t *testing.T) {
	t.Parallel()

	var got []string
	var events []string
	var attrs map[string]interface{}
	m := observability.NewRecorderMetrics(func(name string, value float64) {
		if value != 1 {
			t.Errorf("%s value %v", name, value)
		}
		got = append(got, name)
	}, func(name string, a map[string]interface{}) {
		events = append(events, name)
		attrs = a
	})
	m.ObserveAuthorizeAttempt("SUCCESS", "AUTHORIZED", "merch", "pay")
	m.ObserveAuthorizeRetry()
	m.ObserveClaimLost()
	m.ObserveWebhookDelivered()
	m.ObserveWebhookRetry()
	m.ObserveWebhookFailed()

	want := []string{
		"Custom/Authorize/Attempt",
		"Custom/Authorize/Retry",
		"Custom/Authorize/ClaimLost",
		"Custom/Webhook/Delivered",
		"Custom/Webhook/Retry",
		"Custom/Webhook/Failed",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q want %q", got[i], want[i])
		}
	}
	if len(events) != 1 || events[0] != "AuthorizeAttempt" {
		t.Fatalf("events %v", events)
	}
	if attrs["bank_outcome"] != "SUCCESS" || attrs["to_status"] != "AUTHORIZED" ||
		attrs["merchant_id"] != "merch" || attrs["payment_id"] != "pay" {
		t.Fatalf("attrs %+v", attrs)
	}
}

func TestQueueReporterRecordsPendingGauges(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	got := map[string]float64{}
	m := observability.NewRecorderMetrics(func(name string, value float64) {
		mu.Lock()
		got[name] = value
		mu.Unlock()
	}, nil)
	snap := func(context.Context) (int64, int64, error) {
		return 2, 1, nil
	}
	wait := observability.StartQueueReporter(ctx, m, snap, time.Hour, nil)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		_, pay := got["Custom/Payments/Queue"]
		_, hook := got["Custom/Webhook/Queue"]
		mu.Unlock()
		if pay && hook {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	wait()

	mu.Lock()
	defer mu.Unlock()
	if got["Custom/Payments/Queue"] != 2 {
		t.Fatalf("payments: %+v", got)
	}
	if got["Custom/Webhook/Queue"] != 1 {
		t.Fatalf("webhooks: %+v", got)
	}
}

func TestQueueReporterSnapshotErrorDoesNotPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := zerolog.Nop()
	m := observability.NewRecorderMetrics(func(string, float64) {
		t.Error("should not emit on snapshot error")
	}, nil)
	snap := func(context.Context) (int64, int64, error) {
		return 0, 0, errors.New("db down")
	}
	wait := observability.StartQueueReporter(ctx, m, snap, time.Hour, &log)
	time.Sleep(20 * time.Millisecond)
	cancel()
	wait()
}
