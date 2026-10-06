package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/observability"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/ariwalapratham/go-payment-simulator/internal/webhook"
	"github.com/rs/zerolog"
)

// ErrNoWebhookJob means the webhook worker should idle until the next poll.
var ErrNoWebhookJob = repository.ErrNoWebhookJob

type webhookStore interface {
	ClaimDueWebhookDelivery(ctx context.Context, lease time.Duration) (*repository.WebhookDeliveryJob, error)
	MarkWebhookDelivered(ctx context.Context, deliveryID int64) error
	RecordWebhookFailure(ctx context.Context, deliveryID int64, lastError string, nextAttemptAt time.Time, failed bool) error
	ReleaseWebhookDeliveryLease(ctx context.Context, deliveryID int64) error
}

// WebhookProcessor delivers one leased webhook per ProcessNext call.
type WebhookProcessor struct {
	store       webhookStore
	httpClient  *http.Client
	retry       RetryConfig
	lease       time.Duration
	callTimeout time.Duration
	log         zerolog.Logger
	now         func() time.Time
}

// NewWebhookProcessor wires outbound delivery. httpClient may be nil (default with callTimeout).
func NewWebhookProcessor(
	store webhookStore,
	retry RetryConfig,
	callTimeout, lease time.Duration,
	log *zerolog.Logger,
	httpClient *http.Client,
) (*WebhookProcessor, error) {
	if store == nil {
		return nil, fmt.Errorf("webhook processor: store is required")
	}
	if err := retry.Validate(); err != nil {
		return nil, fmt.Errorf("webhook processor: %w", err)
	}
	if callTimeout <= 0 {
		return nil, fmt.Errorf("webhook processor: call timeout must be > 0")
	}
	if lease < callTimeout {
		return nil, fmt.Errorf("webhook processor: lease must be >= call timeout")
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: callTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	p := &WebhookProcessor{
		store:       store,
		httpClient:  httpClient,
		retry:       retry,
		lease:       lease,
		callTimeout: callTimeout,
		log:         zerolog.Nop(),
		now:         time.Now,
	}
	if log != nil {
		p.log = *log
	}
	return p, nil
}

// ProcessNext claims one due delivery and POSTs it to the merchant.
func (p *WebhookProcessor) ProcessNext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	job, err := p.store.ClaimDueWebhookDelivery(ctx, p.lease)
	if err != nil {
		if errors.Is(err, repository.ErrNoWebhookJob) {
			return ErrNoWebhookJob
		}
		return err
	}
	if job.TargetURL == "" {
		return p.persistDelivered(ctx, job)
	}
	return p.deliver(ctx, job)
}

func (p *WebhookProcessor) deliver(ctx context.Context, job *repository.WebhookDeliveryJob) error {
	body, err := webhook.MarshalOutbound(job.Event.PublicID, job.Event.Type, job.Event.CreatedAt, job.Event.Payload)
	if err != nil {
		return fmt.Errorf("marshal webhook %s: %w", job.Event.PublicID, err)
	}
	signature := webhook.SignBody(job.TargetSecret, body)

	callCtx, cancel := context.WithTimeout(ctx, p.callTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, job.TargetURL, bytes.NewReader(body))
	if err != nil {
		return p.persistFailure(ctx, job, "build request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-Event-Id", job.Event.PublicID.String())

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return p.releaseLease(job, ctx.Err())
		}
		return p.persistFailure(ctx, job, "http client error")
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return p.persistFailure(ctx, job, fmt.Sprintf("http %d", resp.StatusCode))
	}
	return p.persistDelivered(ctx, job)
}

func (p *WebhookProcessor) persistDelivered(ctx context.Context, job *repository.WebhookDeliveryJob) error {
	dbCtx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()
	if err := p.store.MarkWebhookDelivered(dbCtx, job.DeliveryID); err != nil {
		return fmt.Errorf("mark webhook delivered %s: %w", job.Event.PublicID, err)
	}
	e := p.log.Info().
		Str("component", "webhook_processor").
		Str("event", "webhook.delivered").
		Str("webhook_event_id", job.Event.PublicID.String()).
		Str("webhook_type", job.Event.Type.String())
	if id, ok := observability.WorkerIDFrom(ctx); ok {
		e = e.Int("worker_id", id)
	}
	e.Msg("webhook delivered")
	return nil
}

func (p *WebhookProcessor) persistFailure(ctx context.Context, job *repository.WebhookDeliveryJob, lastError string) error {
	next := job.AttemptCount + 1
	failed := next >= p.retry.MaxAttempts
	nextAt := p.now()
	if !failed {
		nextAt = nextAt.Add(backoff(job.AttemptCount, p.retry))
	}
	dbCtx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()
	if err := p.store.RecordWebhookFailure(dbCtx, job.DeliveryID, lastError, nextAt, failed); err != nil {
		return fmt.Errorf("record webhook failure %s: %w", job.Event.PublicID, err)
	}
	e := p.log.Warn().
		Str("component", "webhook_processor").
		Str("webhook_event_id", job.Event.PublicID.String()).
		Str("webhook_type", job.Event.Type.String()).
		Int("attempt", next).
		Str("last_error", lastError)
	if id, ok := observability.WorkerIDFrom(ctx); ok {
		e = e.Int("worker_id", id)
	}
	if failed {
		e.Str("event", "webhook.failed").Msg("webhook delivery exhausted")
	} else {
		e.Str("event", "webhook.retry_scheduled").Msg("webhook delivery retry scheduled")
	}
	return nil
}

func (p *WebhookProcessor) releaseLease(job *repository.WebhookDeliveryJob, cause error) error {
	dbCtx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()
	if err := p.store.ReleaseWebhookDeliveryLease(dbCtx, job.DeliveryID); err != nil {
		return fmt.Errorf("release webhook lease %s: %w", job.Event.PublicID, err)
	}
	return cause
}
