package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/jackc/pgx/v5"
)

var (
	// ErrNoJob means no PENDING payment is due for authorize.
	ErrNoJob = errors.New("no due payment")
	// ErrLostLease means another worker already moved the row out of this claim.
	ErrLostLease = errors.New("authorize claim lost")
)

// LostLeaseError is ErrLostLease with the status seen under FOR UPDATE.
// Cancel of PENDING during an in-flight bank call is the expected case.
type LostLeaseError struct {
	Status model.PaymentStatus
}

func (e LostLeaseError) Error() string {
	if e.Status == "" {
		return ErrLostLease.Error()
	}
	return fmt.Sprintf("%s: status is %s", ErrLostLease.Error(), e.Status)
}

func (e LostLeaseError) Unwrap() error { return ErrLostLease }

// ApplyAuthorizeInput is the row mutation after one bank attempt.
type ApplyAuthorizeInput struct {
	PaymentID            int64
	ExpectedAttemptCount int
	Status               model.PaymentStatus
	AttemptCount         int
	NextAttemptAt        time.Time
	LastError            *string
}

// ClaimDuePayment leases one PENDING row by pushing next_attempt_at forward.
// The bank call must happen after this transaction commits.
func (r *PaymentRepository) ClaimDuePayment(ctx context.Context, lease time.Duration) (*PaymentRecord, error) {
	leaseMS := lease.Milliseconds()
	if leaseMS < 1 {
		leaseMS = 1
	}

	const q = `
WITH next AS (
  SELECT p.id
  FROM payments p
  WHERE p.status = 'PENDING'
    AND p.next_attempt_at <= now()
  ORDER BY p.next_attempt_at, p.id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE payments p
SET next_attempt_at = now() + ($1 * interval '1 millisecond'),
    updated_at = now()
FROM next n, merchants m
WHERE p.id = n.id AND m.id = p.merchant_id
RETURNING p.id, p.public_id, p.merchant_id, p.amount, p.currency, p.status,
          p.attempt_count, p.next_attempt_at, p.last_error, p.created_at, p.updated_at,
          m.public_id`

	rec, err := scanPaymentRecord(r.pool.QueryRow(ctx, q, leaseMS))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoJob
		}
		return nil, fmt.Errorf("claim due payment: %w", err)
	}
	return rec, nil
}

// ApplyAuthorizeDecision writes the policy result if the row is still PENDING.
func (r *PaymentRepository) ApplyAuthorizeDecision(ctx context.Context, in ApplyAuthorizeInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin apply authorize: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lockQ = `
SELECT id, public_id, merchant_id, amount, currency, status,
       attempt_count, next_attempt_at, last_error, created_at, updated_at
FROM payments
WHERE id = $1
FOR UPDATE`

	p, err := scanPayment(tx.QueryRow(ctx, lockQ, in.PaymentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPaymentNotFound
		}
		return fmt.Errorf("lock payment for apply: %w", err)
	}
	if p.Status != model.PaymentStatusPending || p.AttemptCount != in.ExpectedAttemptCount {
		return LostLeaseError{Status: p.Status}
	}
	if in.Status != model.PaymentStatusPending {
		if err := model.Transition(p.Status, in.Status); err != nil {
			return err
		}
	}

	const upd = `
UPDATE payments
SET status = $2, attempt_count = $3, next_attempt_at = $4, last_error = $5, updated_at = now()
WHERE id = $1 AND status = $6 AND attempt_count = $7`

	tag, err := tx.Exec(
		ctx, upd,
		in.PaymentID, in.Status, in.AttemptCount, in.NextAttemptAt, in.LastError,
		model.PaymentStatusPending, in.ExpectedAttemptCount,
	)
	if err != nil {
		return fmt.Errorf("apply authorize decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return LostLeaseError{Status: p.Status}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit apply authorize: %w", err)
	}
	return nil
}
