package observability

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
)

const DefaultQueueInterval = 15 * time.Second

// AuthorizeMetrics is a hook for authorize outcomes.
type AuthorizeMetrics interface {
	ObserveAuthorizeAttempt(bankOutcome, toStatus string)
	ObserveAuthorizeRetry()
	ObserveClaimLost()
}

// WebhookMetrics is a hook for outbound delivery results.
type WebhookMetrics interface {
	ObserveWebhookDelivered()
	ObserveWebhookRetry()
	ObserveWebhookFailed()
}

// NopAuthorizeMetrics discards observations.
type NopAuthorizeMetrics struct{}

func (NopAuthorizeMetrics) ObserveAuthorizeAttempt(string, string) {}
func (NopAuthorizeMetrics) ObserveAuthorizeRetry()                 {}
func (NopAuthorizeMetrics) ObserveClaimLost()                      {}

// NopWebhookMetrics discards observations.
type NopWebhookMetrics struct{}

func (NopWebhookMetrics) ObserveWebhookDelivered() {}
func (NopWebhookMetrics) ObserveWebhookRetry()     {}
func (NopWebhookMetrics) ObserveWebhookFailed()    {}

// Metrics records Custom/* values on a New Relic app. A nil app is a no-op.
type Metrics struct {
	record func(string, float64)
}

var (
	_ AuthorizeMetrics = (*Metrics)(nil)
	_ WebhookMetrics   = (*Metrics)(nil)
)

// NewMetrics uses app.RecordCustomMetric. Nil app or nil *Metrics is a no-op.
func NewMetrics(app *newrelic.Application) *Metrics {
	if app == nil {
		return &Metrics{}
	}
	return &Metrics{record: app.RecordCustomMetric}
}

// NewRecorderMetrics records to record. Used by tests.
func NewRecorderMetrics(record func(string, float64)) *Metrics {
	return &Metrics{record: record}
}

func (m *Metrics) ObserveAuthorizeAttempt(bankOutcome, toStatus string) {
	m.emit("Custom/Authorize/Attempt/"+bankOutcome+"/"+toStatus, 1)
}

func (m *Metrics) ObserveAuthorizeRetry() { m.emit("Custom/Authorize/Retry", 1) }

func (m *Metrics) ObserveClaimLost() { m.emit("Custom/Authorize/ClaimLost", 1) }

func (m *Metrics) ObserveWebhookDelivered() { m.emit("Custom/Webhook/Delivered", 1) }

func (m *Metrics) ObserveWebhookRetry() { m.emit("Custom/Webhook/Retry", 1) }

func (m *Metrics) ObserveWebhookFailed() { m.emit("Custom/Webhook/Failed", 1) }

func (m *Metrics) emit(name string, value float64) {
	if m == nil || m.record == nil {
		return
	}
	m.record(name, value)
}

// QueueSnapshot returns payment and webhook_delivery counts keyed by status.
type QueueSnapshot func(context.Context) (payments, webhooks map[string]int64, err error)

// StartQueueReporter records status gauges until ctx is canceled. No-op if metrics are disabled.
func StartQueueReporter(ctx context.Context, m *Metrics, snap QueueSnapshot, interval time.Duration) {
	if m == nil || m.record == nil || snap == nil {
		return
	}
	if interval <= 0 {
		interval = DefaultQueueInterval
	}
	go func() {
		m.recordQueue(ctx, snap)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.recordQueue(ctx, snap)
			}
		}
	}()
}

func (m *Metrics) recordQueue(ctx context.Context, snap QueueSnapshot) {
	c, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	payments, webhooks, err := snap(c)
	if err != nil {
		return
	}
	for status, n := range payments {
		m.emit("Custom/Payments/Status/"+status, float64(n))
	}
	for status, n := range webhooks {
		m.emit("Custom/Webhook/Queue/"+status, float64(n))
	}
}
