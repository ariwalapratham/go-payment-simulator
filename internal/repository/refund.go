package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type CreateRefundInput struct {
	MerchantPublicID uuid.UUID
	PaymentPublicID  uuid.UUID
	IdempotencyKey   string
	RequestHash      string
	Amount           int64
}

type RefundRecord struct {
	Refund           model.DBRefund
	PaymentPublicID  uuid.UUID
	MerchantPublicID uuid.UUID
}

// GetRefundByPublicID loads a refund only if its payment belongs to the merchant.
func (r *PaymentRepository) GetRefundByPublicID(
	ctx context.Context,
	merchantPublicID, refundPublicID uuid.UUID,
) (*RefundRecord, error) {
	const q = `
SELECT r.id, r.public_id, r.payment_id, r.amount, r.status, r.created_at,
       p.public_id, m.public_id
FROM refunds r
JOIN payments p ON p.id = r.payment_id
JOIN merchants m ON m.id = p.merchant_id
WHERE r.public_id = $1 AND m.public_id = $2`

	rec, err := scanRefundRecord(r.pool.QueryRow(ctx, q, refundPublicID, merchantPublicID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefundNotFound
		}
		return nil, fmt.Errorf("refund by public id: %w", err)
	}
	return rec, nil
}

// CreateRefundIdempotent inserts once per (merchant, payment.refund, key). Concurrent losers replay.
func (r *PaymentRepository) CreateRefundIdempotent(
	ctx context.Context,
	in CreateRefundInput,
) (*RefundRecord, bool, error) {
	var last error
	for range idempotencyMaxAttempts {
		rec, created, err := r.createRefundOnce(ctx, in)
		if err == nil {
			return rec, created, nil
		}
		if !isRetry(err) {
			return nil, false, err
		}
		last = err
		time.Sleep(idempotencyRetryWait)
	}
	return nil, false, fmt.Errorf("idempotent refund: %w", last)
}

// createRefundOnce is one txn: claim key → lock payment, insert refund, or replay.
func (r *PaymentRepository) createRefundOnce(
	ctx context.Context,
	in CreateRefundInput,
) (*RefundRecord, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin create refund: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	merchant, err := merchantByPublicIDTx(ctx, tx, in.MerchantPublicID)
	if err != nil {
		return nil, false, err
	}

	claim, won, err := claimIdempotency(
		ctx, tx, merchant.ID, model.IdempotencyScopePaymentRefund, in.IdempotencyKey, in.RequestHash,
	)
	if err != nil {
		return nil, false, err
	}

	if won {
		rec, err := insertSucceededRefund(ctx, tx, merchant.PublicID, in.PaymentPublicID, in.Amount)
		if err != nil {
			return nil, false, err
		}
		if err := attachRefundToClaim(ctx, tx, claim.ID, rec.Refund.ID); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("commit create refund: %w", err)
		}
		return rec, true, nil
	}

	rec, err := refundFromExistingClaim(ctx, tx, merchant.ID, in.IdempotencyKey, in.RequestHash)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit replay refund: %w", err)
	}
	return rec, false, nil
}

func insertSucceededRefund(
	ctx context.Context,
	tx pgx.Tx,
	merchantPublicID, paymentPublicID uuid.UUID,
	amount int64,
) (*RefundRecord, error) {
	const lockQ = `
SELECT p.id, p.public_id, p.merchant_id, p.amount, p.currency, p.status,
       p.attempt_count, p.next_attempt_at, p.last_error, p.created_at, p.updated_at,
       m.public_id
FROM payments p
JOIN merchants m ON m.id = p.merchant_id
WHERE p.public_id = $1 AND m.public_id = $2
FOR UPDATE OF p`

	pay, err := scanPaymentRecord(tx.QueryRow(ctx, lockQ, paymentPublicID, merchantPublicID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("lock payment for refund: %w", err)
	}
	if pay.Payment.Status != model.PaymentStatusCaptured {
		return nil, fmt.Errorf("%w: %s -> %s", model.ErrInvalidTransition, pay.Payment.Status, model.PaymentStatusRefunded)
	}

	succeeded, err := succeededRefundSum(ctx, tx, pay.Payment.ID)
	if err != nil {
		return nil, err
	}
	remaining := RemainingRefundable(pay.Payment.Amount, succeeded)
	if amount > remaining {
		return nil, ErrRefundExceedsBalance
	}

	const ins = `
INSERT INTO refunds (payment_id, amount, status)
VALUES ($1, $2, $3)
RETURNING id, public_id, payment_id, amount, status, created_at`

	refund, err := scanRefund(tx.QueryRow(ctx, ins, pay.Payment.ID, amount, model.RefundStatusSucceeded))
	if err != nil {
		return nil, fmt.Errorf("insert refund: %w", err)
	}

	if remaining == amount {
		if err := model.Transition(pay.Payment.Status, model.PaymentStatusRefunded); err != nil {
			return nil, err
		}
		const upd = `
UPDATE payments
SET status = $2, updated_at = now()
WHERE id = $1 AND status = $3`
		tag, err := tx.Exec(ctx, upd, pay.Payment.ID, model.PaymentStatusRefunded, model.PaymentStatusCaptured)
		if err != nil {
			return nil, fmt.Errorf("mark payment refunded: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, fmt.Errorf("%w: %s -> %s", model.ErrInvalidTransition, pay.Payment.Status, model.PaymentStatusRefunded)
		}
	}

	return &RefundRecord{
		Refund:           *refund,
		PaymentPublicID:  pay.Payment.PublicID,
		MerchantPublicID: pay.MerchantPublicID,
	}, nil
}

func succeededRefundSum(ctx context.Context, tx pgx.Tx, paymentID int64) (int64, error) {
	const q = `
SELECT COALESCE(SUM(amount), 0)::bigint
FROM refunds
WHERE payment_id = $1 AND status = $2`

	var sum int64
	if err := tx.QueryRow(ctx, q, paymentID, model.RefundStatusSucceeded).Scan(&sum); err != nil {
		return 0, fmt.Errorf("sum succeeded refunds: %w", err)
	}
	return sum, nil
}

func attachRefundToClaim(ctx context.Context, tx pgx.Tx, claimID, refundID int64) error {
	const q = `UPDATE idempotency_keys SET refund_id = $1 WHERE id = $2`
	_, err := tx.Exec(ctx, q, refundID, claimID)
	if err != nil {
		return fmt.Errorf("attach refund to idempotency key: %w", err)
	}
	return nil
}

func refundFromExistingClaim(
	ctx context.Context,
	tx pgx.Tx,
	merchantID int64,
	key, requestHash string,
) (*RefundRecord, error) {
	claim, err := lockIdempotencyClaim(ctx, tx, merchantID, model.IdempotencyScopePaymentRefund, key, requestHash)
	if err != nil {
		return nil, err
	}
	if claim.RefundID == nil {
		return nil, retryError{}
	}

	const q = `
SELECT r.id, r.public_id, r.payment_id, r.amount, r.status, r.created_at,
       p.public_id, m.public_id
FROM refunds r
JOIN payments p ON p.id = r.payment_id
JOIN merchants m ON m.id = p.merchant_id
WHERE r.id = $1`

	rec, err := scanRefundRecord(tx.QueryRow(ctx, q, *claim.RefundID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, retryError{}
		}
		return nil, fmt.Errorf("refund by id: %w", err)
	}
	return rec, nil
}

func scanRefund(row pgx.Row) (*model.DBRefund, error) {
	var r model.DBRefund
	if err := row.Scan(&r.ID, &r.PublicID, &r.PaymentID, &r.Amount, &r.Status, &r.CreatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

func scanRefundRecord(row pgx.Row) (*RefundRecord, error) {
	var rec RefundRecord
	r := &rec.Refund
	if err := row.Scan(
		&r.ID, &r.PublicID, &r.PaymentID, &r.Amount, &r.Status, &r.CreatedAt,
		&rec.PaymentPublicID, &rec.MerchantPublicID,
	); err != nil {
		return nil, err
	}
	return &rec, nil
}
