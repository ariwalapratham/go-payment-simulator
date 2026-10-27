package observability

import (
	"context"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/rs/zerolog"
)

const DefaultQueueInterval = 15 * time.Second

// AuthorizeMetrics is a hook for authorize outcomes.
type AuthorizeMetrics interface {
	ObserveAuthorizeAttempt(bankOutcome, toStatus, merchantID, paymentID string)
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

func (NopAuthorizeMetrics) ObserveAuthorizeAttempt(string, string, string, string) {}
func (NopAuthorizeMetrics) ObserveAuthorizeRetry()                                 {}
func (NopAuthorizeMetrics) ObserveClaimLost()                                      {}

// NopWebhookMetrics discards observations.
type NopWebhookMetrics struct{}

func (NopWebhookMetrics) ObserveWebhookDelivered() {}
func (NopWebhookMetrics) ObserveWebhookRetry()     {}
func (NopWebhookMetrics) ObserveWebhookFailed()    {}

// Metrics records Custom/* values on a New Relic app. A nil app is a no-op.
type Metrics struct {
	record func(string, float64)
	event  func(string, map[string]interface{})
}

var (
	_ AuthorizeMetrics = (*Metrics)(nil)
	_ WebhookMetrics   = (*Metrics)(nil)
)

// NewMetrics uses app.RecordCustomMetric / RecordCustomEvent. Nil app is a no-op.
func NewMetrics(app *newrelic.Application) *Metrics {
	if app == nil {
		return &Metrics{}
	}
	return &Metrics{record: app.RecordCustomMetric, event: app.RecordCustomEvent}
}

// NewRecorderMetrics records to record/event. Used by tests.
func NewRecorderMetrics(record func(string, float64), event func(string, map[string]interface{})) *Metrics {
	return &Metrics{record: record, event: event}
}

func (m *Metrics) ObserveAuthorizeAttempt(bankOutcome, toStatus, merchantID, paymentID string) {
	m.emit("Custom/Authorize/Attempt", 1)
	m.emitEvent("AuthorizeAttempt", map[string]interface{}{
		"bank_outcome": bankOutcome,
		"to_status":    toStatus,
		"merchant_id":  merchantID,
		"payment_id":   paymentID,
	})
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

func (m *Metrics) emitEvent(name string, attrs map[string]interface{}) {
	if m == nil || m.event == nil {
		return
	}
	m.event(name, attrs)
}

// QueueSnapshot returns pending payment and webhook_delivery counts.
type QueueSnapshot func(context.Context) (payments, webhooks int64, err error)

// StartQueueReporter records queue gauges until ctx is canceled. Wait after cancel before closing the DB.
func StartQueueReporter(
	ctx context.Context,
	m *Metrics,
	snap QueueSnapshot,
	interval time.Duration,
	log *zerolog.Logger,
) (wait func()) {
	if m == nil || m.record == nil || snap == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = DefaultQueueInterval
	}
	logger := zerolog.Nop()
	if log != nil {
		logger = *log
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.recordQueue(ctx, snap, logger)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.recordQueue(ctx, snap, logger)
			}
		}
	}()
	return wg.Wait
}

func (m *Metrics) recordQueue(ctx context.Context, snap QueueSnapshot, log zerolog.Logger) {
	c, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	payments, webhooks, err := snap(c)
	if err != nil {
		log.Warn().Err(err).Str("component", "queue_reporter").Msg("queue snapshot failed")
		return
	}
	m.emit("Custom/Payments/Queue", float64(payments))
	m.emit("Custom/Webhook/Queue", float64(webhooks))
}
