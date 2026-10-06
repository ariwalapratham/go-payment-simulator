package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxWebhookLastError = 512

var (
	// ErrNoWebhookJob means no webhook delivery is due.
	ErrNoWebhookJob = errors.New("no due webhook delivery")
)

// WebhookEnqueueParams describes one outbox row to insert inside a business transaction.
type WebhookEnqueueParams struct {
	MerchantID      int64
	PaymentID       int64
	PaymentPublicID uuid.UUID
	EventType       model.WebhookEventType
	Status          model.PaymentStatus
	Amount          int64
	Currency        string
	WebhookURL      *string
}

// WebhookDeliveryJob is a leased delivery ready for HTTP POST.
type WebhookDeliveryJob struct {
	DeliveryID   int64
	AttemptCount int
	Event        model.DBWebhookEvent
	TargetURL    string
	TargetSecret string
}

// EnqueueWebhookIfConfigured inserts webhook_events + webhook_deliveries when the merchant has a URL.
func EnqueueWebhookIfConfigured(ctx context.Context, tx pgx.Tx, p WebhookEnqueueParams) error {
	if p.WebhookURL == nil || strings.TrimSpace(*p.WebhookURL) == "" {
		return nil
	}

	payload, err := json.Marshal(model.WebhookEventData{
		PaymentID: p.PaymentPublicID.String(),
		Status:    p.Status.String(),
		Amount:    p.Amount,
		Currency:  p.Currency,
	})
	if err != nil {
		return fmt.Errorf("webhook data payload: %w", err)
	}

	const insEvent = `
INSERT INTO webhook_events (payment_id, type, payload)
VALUES ($1, $2, $3)
RETURNING id`

	var eventID int64
	if err := tx.QueryRow(ctx, insEvent, p.PaymentID, p.EventType.String(), payload).Scan(&eventID); err != nil {
		return fmt.Errorf("insert webhook event: %w", err)
	}

	const insDelivery = `
INSERT INTO webhook_deliveries (webhook_event_id, status, attempt_count, next_attempt_at)
VALUES ($1, $2, 0, now())`

	if _, err := tx.Exec(ctx, insDelivery, eventID, model.WebhookDeliveryStatusPending.String()); err != nil {
		return fmt.Errorf("insert webhook delivery: %w", err)
	}
	return nil
}

// ClaimDueWebhookDelivery leases one PENDING row due for delivery.
func (r *PaymentRepository) ClaimDueWebhookDelivery(ctx context.Context, lease time.Duration) (*WebhookDeliveryJob, error) {
	leaseMS := lease.Milliseconds()
	if leaseMS < 1 {
		leaseMS = 1
	}

	const q = `
WITH next AS (
  SELECT d.id
  FROM webhook_deliveries d
  WHERE d.status = $2
    AND d.next_attempt_at <= now()
  ORDER BY d.next_attempt_at, d.id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE webhook_deliveries d
SET next_attempt_at = now() + ($1 * interval '1 millisecond'),
    updated_at = now()
FROM next n
WHERE d.id = n.id
RETURNING d.id, d.webhook_event_id, d.attempt_count`

	var delID, eventID int64
	var attemptCount int
	err := r.pool.QueryRow(ctx, q, leaseMS, model.WebhookDeliveryStatusPending.String()).Scan(
		&delID, &eventID, &attemptCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoWebhookJob
		}
		return nil, fmt.Errorf("claim webhook delivery: %w", err)
	}

	const load = `
SELECT e.id, e.public_id, e.payment_id, e.type, e.payload, e.created_at,
       m.webhook_url, m.webhook_secret
FROM webhook_events e
JOIN payments p ON p.id = e.payment_id
JOIN merchants m ON m.id = p.merchant_id
WHERE e.id = $1`

	var ev model.DBWebhookEvent
	var url *string
	var secret string
	if err := r.pool.QueryRow(ctx, load, eventID).Scan(
		&ev.ID, &ev.PublicID, &ev.PaymentID, &ev.Type, &ev.Payload, &ev.CreatedAt,
		&url, &secret,
	); err != nil {
		return nil, fmt.Errorf("load webhook event for delivery: %w", err)
	}

	target := ""
	if url != nil {
		target = strings.TrimSpace(*url)
	}
	return &WebhookDeliveryJob{
		DeliveryID:   delID,
		AttemptCount: attemptCount,
		Event:        ev,
		TargetURL:    target,
		TargetSecret: secret,
	}, nil
}

// MarkWebhookDelivered sets status DELIVERED after a successful HTTP POST.
func (r *PaymentRepository) MarkWebhookDelivered(ctx context.Context, deliveryID int64) error {
	const q = `
UPDATE webhook_deliveries
SET status = $2, updated_at = now(), last_error = NULL
WHERE id = $1 AND status = $3`

	tag, err := r.pool.Exec(ctx, q, deliveryID, model.WebhookDeliveryStatusDelivered.String(), model.WebhookDeliveryStatusPending.String())
	if err != nil {
		return fmt.Errorf("mark webhook delivered: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mark webhook delivered: row not pending")
	}
	return nil
}

// RecordWebhookFailure increments attempts and either schedules another try or marks FAILED.
func (r *PaymentRepository) RecordWebhookFailure(ctx context.Context, deliveryID int64, lastError string, failed bool) error {
	if len(lastError) > maxWebhookLastError {
		lastError = lastError[:maxWebhookLastError]
	}
	status := model.WebhookDeliveryStatusPending
	if failed {
		status = model.WebhookDeliveryStatusFailed
	}
	const q = `
UPDATE webhook_deliveries
SET status = $2, attempt_count = attempt_count + 1, last_error = $3, updated_at = now()
WHERE id = $1 AND status = $4`

	tag, err := r.pool.Exec(ctx, q, deliveryID, status.String(), lastError, model.WebhookDeliveryStatusPending.String())
	if err != nil {
		return fmt.Errorf("record webhook failure: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("record webhook failure: row not pending")
	}
	return nil
}

// ReleaseWebhookDeliveryLease makes a claimed row immediately eligible again (shutdown / skip).
func (r *PaymentRepository) ReleaseWebhookDeliveryLease(ctx context.Context, deliveryID int64) error {
	const q = `
UPDATE webhook_deliveries
SET next_attempt_at = now(), updated_at = now()
WHERE id = $1 AND status = $2`

	_, err := r.pool.Exec(ctx, q, deliveryID, model.WebhookDeliveryStatusPending.String())
	if err != nil {
		return fmt.Errorf("release webhook lease: %w", err)
	}
	return nil
}
