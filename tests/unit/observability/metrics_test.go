package observability_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/observability"
)

func TestMetricsNopDoesNotPanic(t *testing.T) {
	t.Parallel()

	var m *observability.Metrics
	m.ObserveAuthorizeAttempt("SUCCESS", "AUTHORIZED")
	m.ObserveAuthorizeRetry()
	m.ObserveClaimLost()
	m.ObserveWebhookDelivered()
	m.ObserveWebhookRetry()
	m.ObserveWebhookFailed()
	observability.NewMetrics(nil).ObserveAuthorizeRetry()
	observability.StartQueueReporter(context.Background(), observability.NewMetrics(nil), nil, time.Millisecond)
}

func TestMetricsRecordsCustomNames(t *testing.T) {
	t.Parallel()

	var got []string
	m := observability.NewRecorderMetrics(func(name string, value float64) {
		if value != 1 {
			t.Errorf("%s value %v", name, value)
		}
		got = append(got, name)
	})
	m.ObserveAuthorizeAttempt("SUCCESS", "AUTHORIZED")
	m.ObserveAuthorizeRetry()
	m.ObserveClaimLost()
	m.ObserveWebhookDelivered()
	m.ObserveWebhookRetry()
	m.ObserveWebhookFailed()

	want := []string{
		"Custom/Authorize/Attempt/SUCCESS/AUTHORIZED",
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
}

func TestQueueReporterRecordsStatusGauges(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	got := map[string]float64{}
	m := observability.NewRecorderMetrics(func(name string, value float64) {
		mu.Lock()
		got[name] = value
		mu.Unlock()
	})
	snap := func(context.Context) (map[string]int64, map[string]int64, error) {
		return map[string]int64{"PENDING": 2}, map[string]int64{"PENDING": 1}, nil
	}
	observability.StartQueueReporter(ctx, m, snap, time.Hour)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		_, pay := got["Custom/Payments/Status/PENDING"]
		_, hook := got["Custom/Webhook/Queue/PENDING"]
		mu.Unlock()
		if pay && hook {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if got["Custom/Payments/Status/PENDING"] != 2 {
		t.Fatalf("payments: %+v", got)
	}
	if got["Custom/Webhook/Queue/PENDING"] != 1 {
		t.Fatalf("webhooks: %+v", got)
	}
}
