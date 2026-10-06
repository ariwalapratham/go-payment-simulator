package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	idempotencyMaxAttempts = 8
	idempotencyRetryWait   = 5 * time.Millisecond
)

type retryError struct{}

func (retryError) Error() string { return "retry idempotent create" }

var (
	ErrMerchantNotFound     = errors.New("merchant not found")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrRefundNotFound       = errors.New("refund not found")
	ErrIdempotencyKeyReused = errors.New("idempotency key reused with a different payload")
	ErrRefundExceedsBalance = errors.New("refund exceeds remaining balance")
	ErrDuplicateAPIKeyHash  = errors.New("duplicate api key hash")
)

// RemainingRefundable is captured amount minus succeeded refunds.
func RemainingRefundable(captured, succeededSum int64) int64 {
	return captured - succeededSum
}

type CreatePaymentInput struct {
	MerchantPublicID uuid.UUID
	IdempotencyKey   string
	RequestHash      string
	Amount           int64
	Currency         string
}

type PaymentRecord struct {
	Payment          model.DBPayment
	MerchantPublicID uuid.UUID
	WebhookURL       *string
}

type PaymentRepository struct {
	pool *pgxpool.Pool
}

// NewPaymentRepository is the payments + idempotency SQL layer.
func NewPaymentRepository(pool *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{pool: pool}
}

// MerchantByPublicID loads a merchant by API-facing UUID.
func (r *PaymentRepository) MerchantByPublicID(ctx context.Context, publicID uuid.UUID) (*model.DBMerchant, error) {
	const q = `
SELECT ` + merchantCols + `
FROM merchants
WHERE public_id = $1`

	row := r.pool.QueryRow(ctx, q, publicID)
	m, err := scanMerchant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMerchantNotFound
		}
		return nil, fmt.Errorf("merchant by public id: %w", err)
	}
	return m, nil
}

// GetPaymentByPublicID loads a payment only if it belongs to the merchant.
func (r *PaymentRepository) GetPaymentByPublicID(
	ctx context.Context,
	merchantPublicID, paymentPublicID uuid.UUID,
) (*PaymentRecord, error) {
	const q = `
SELECT p.id, p.public_id, p.merchant_id, p.amount, p.currency, p.status,
       p.attempt_count, p.next_attempt_at, p.last_error, p.created_at, p.updated_at,
       m.public_id, m.webhook_url
FROM payments p
JOIN merchants m ON m.id = p.merchant_id
WHERE p.public_id = $1 AND m.public_id = $2`

	row := r.pool.QueryRow(ctx, q, paymentPublicID, merchantPublicID)
	rec, err := scanPaymentRecord(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("payment by public id: %w", err)
	}
	return rec, nil
}

// TransitionPayment locks the merchant-scoped payment and applies next if the state machine allows it.
// Concurrent capture/cancel serialize on FOR UPDATE; the loser gets ErrInvalidTransition.
func (r *PaymentRepository) TransitionPayment(
	ctx context.Context,
	merchantPublicID, paymentPublicID uuid.UUID,
	next model.PaymentStatus,
) (*PaymentRecord, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transition payment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lockQ = `
SELECT p.id, p.public_id, p.merchant_id, p.amount, p.currency, p.status,
       p.attempt_count, p.next_attempt_at, p.last_error, p.created_at, p.updated_at,
       m.public_id, m.webhook_url
FROM payments p
JOIN merchants m ON m.id = p.merchant_id
WHERE p.public_id = $1 AND m.public_id = $2
FOR UPDATE OF p`

	rec, err := scanPaymentRecord(tx.QueryRow(ctx, lockQ, paymentPublicID, merchantPublicID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("lock payment for transition: %w", err)
	}
	if err := model.Transition(rec.Payment.Status, next); err != nil {
		return nil, err
	}

	const upd = `
UPDATE payments
SET status = $2, updated_at = now()
WHERE id = $1 AND status = $3
RETURNING id, public_id, merchant_id, amount, currency, status,
          attempt_count, next_attempt_at, last_error, created_at, updated_at`

	p, err := scanPayment(tx.QueryRow(ctx, upd, rec.Payment.ID, next, rec.Payment.Status))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s -> %s", model.ErrInvalidTransition, rec.Payment.Status, next)
		}
		return nil, fmt.Errorf("transition payment: %w", err)
	}
	rec.Payment = *p
	if eventType, ok := webhookEventForPaymentStatus(next); ok {
		if err := EnqueueWebhookIfConfigured(ctx, tx, WebhookEnqueueParams{
			MerchantID:      rec.Payment.MerchantID,
			PaymentID:       rec.Payment.ID,
			PaymentPublicID: rec.Payment.PublicID,
			EventType:       eventType,
			Status:          next,
			Amount:          rec.Payment.Amount,
			Currency:        rec.Payment.Currency,
			WebhookURL:      rec.WebhookURL,
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transition payment: %w", err)
	}
	return rec, nil
}

// CreatePaymentIdempotent inserts once per (merchant, key). Concurrent losers retry then replay.
func (r *PaymentRepository) CreatePaymentIdempotent(
	ctx context.Context,
	in CreatePaymentInput,
) (*PaymentRecord, bool, error) {
	var last error
	for range idempotencyMaxAttempts {
		rec, created, err := r.createPaymentOnce(ctx, in)
		if err == nil {
			return rec, created, nil
		}
		if !isRetry(err) {
			return nil, false, err
		}
		last = err
		time.Sleep(idempotencyRetryWait)
	}
	return nil, false, fmt.Errorf("idempotent create: %w", last)
}

// createPaymentOnce is one txn: claim key → insert PENDING, or lock existing claim and replay.
func (r *PaymentRepository) createPaymentOnce(
	ctx context.Context,
	in CreatePaymentInput,
) (*PaymentRecord, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin create payment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	merchant, err := merchantByPublicIDTx(ctx, tx, in.MerchantPublicID)
	if err != nil {
		return nil, false, err
	}

	claim, won, err := claimIdempotency(ctx, tx, merchant.ID, model.IdempotencyScopePaymentCreate, in.IdempotencyKey, in.RequestHash)
	if err != nil {
		return nil, false, err
	}

	if won {
		payment, err := insertPendingPayment(ctx, tx, merchant.ID, in.Amount, in.Currency)
		if err != nil {
			return nil, false, err
		}
		if err := attachPaymentToClaim(ctx, tx, claim.ID, payment.ID); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("commit create payment: %w", err)
		}
		return &PaymentRecord{Payment: *payment, MerchantPublicID: merchant.PublicID}, true, nil
	}

	payment, err := paymentFromExistingClaim(ctx, tx, merchant.ID, model.IdempotencyScopePaymentCreate, in.IdempotencyKey, in.RequestHash)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit replay payment: %w", err)
	}
	return &PaymentRecord{Payment: *payment, MerchantPublicID: merchant.PublicID}, false, nil
}

// merchantByPublicIDTx is the same lookup inside an open txn.
func merchantByPublicIDTx(ctx context.Context, tx pgx.Tx, publicID uuid.UUID) (*model.DBMerchant, error) {
	const q = `
SELECT ` + merchantCols + `
FROM merchants
WHERE public_id = $1`

	m, err := scanMerchant(tx.QueryRow(ctx, q, publicID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMerchantNotFound
		}
		return nil, fmt.Errorf("merchant lookup: %w", err)
	}
	return m, nil
}

// claimIdempotency INSERT … ON CONFLICT DO NOTHING. won=true if this txn created the row.
func claimIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	merchantID int64,
	scope model.IdempotencyScope,
	key, requestHash string,
) (model.DBIdempotencyKey, bool, error) {
	const q = `
INSERT INTO idempotency_keys (merchant_id, idempotency_key, scope, request_hash, payment_id)
VALUES ($1, $2, $3, $4, NULL)
ON CONFLICT (merchant_id, scope, idempotency_key) DO NOTHING
RETURNING id, merchant_id, idempotency_key, scope, payment_id, refund_id, request_hash, created_at`

	claim, err := scanIdempotencyKey(tx.QueryRow(ctx, q, merchantID, key, scope, requestHash))
	if err == nil {
		return *claim, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return model.DBIdempotencyKey{}, false, nil
	}
	return model.DBIdempotencyKey{}, false, fmt.Errorf("claim idempotency key: %w", err)
}

// insertPendingPayment creates the payment row in PENDING (bank capture comes later).
func insertPendingPayment(ctx context.Context, tx pgx.Tx, merchantID, amount int64, currency string) (*model.DBPayment, error) {
	const q = `
INSERT INTO payments (merchant_id, amount, currency, status)
VALUES ($1, $2, $3, $4)
RETURNING id, public_id, merchant_id, amount, currency, status,
          attempt_count, next_attempt_at, last_error, created_at, updated_at`

	p, err := scanPayment(tx.QueryRow(ctx, q, merchantID, amount, currency, model.PaymentStatusPending))
	if err != nil {
		return nil, fmt.Errorf("insert payment: %w", err)
	}
	return p, nil
}

// attachPaymentToClaim links the new payment so concurrent losers can replay it.
func attachPaymentToClaim(ctx context.Context, tx pgx.Tx, claimID, paymentID int64) error {
	const q = `UPDATE idempotency_keys SET payment_id = $1 WHERE id = $2`
	_, err := tx.Exec(ctx, q, paymentID, claimID)
	if err != nil {
		return fmt.Errorf("attach payment to idempotency key: %w", err)
	}
	return nil
}

// paymentFromExistingClaim FOR UPDATE waits until payment_id is set; mismatch → ErrIdempotencyKeyReused.
func paymentFromExistingClaim(
	ctx context.Context,
	tx pgx.Tx,
	merchantID int64,
	scope model.IdempotencyScope,
	key, requestHash string,
) (*model.DBPayment, error) {
	claim, err := lockIdempotencyClaim(ctx, tx, merchantID, scope, key, requestHash)
	if err != nil {
		return nil, err
	}
	if claim.PaymentID == nil {
		return nil, retryError{}
	}

	payment, err := paymentByID(ctx, tx, *claim.PaymentID)
	if err != nil {
		return nil, err
	}
	return payment, nil
}

// lockIdempotencyClaim FOR UPDATE waits for the winner; mismatch → ErrIdempotencyKeyReused.
func lockIdempotencyClaim(
	ctx context.Context,
	tx pgx.Tx,
	merchantID int64,
	scope model.IdempotencyScope,
	key, requestHash string,
) (*model.DBIdempotencyKey, error) {
	const q = `
SELECT id, merchant_id, idempotency_key, scope, payment_id, refund_id, request_hash, created_at
FROM idempotency_keys
WHERE merchant_id = $1 AND scope = $2 AND idempotency_key = $3
FOR UPDATE`

	claim, err := scanIdempotencyKey(tx.QueryRow(ctx, q, merchantID, scope, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, retryError{}
		}
		return nil, fmt.Errorf("lock idempotency key: %w", err)
	}
	if claim.RequestHash != requestHash {
		return nil, ErrIdempotencyKeyReused
	}
	return claim, nil
}

// paymentByID loads by bigint PK; missing row means the winner has not committed yet.
func paymentByID(ctx context.Context, tx pgx.Tx, id int64) (*model.DBPayment, error) {
	const q = `
SELECT id, public_id, merchant_id, amount, currency, status,
       attempt_count, next_attempt_at, last_error, created_at, updated_at
FROM payments
WHERE id = $1`

	p, err := scanPayment(tx.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, retryError{}
		}
		return nil, fmt.Errorf("payment by id: %w", err)
	}
	return p, nil
}

func isRetry(err error) bool {
	var r retryError
	return errors.As(err, &r)
}

func scanPayment(row pgx.Row) (*model.DBPayment, error) {
	var p model.DBPayment
	if err := row.Scan(
		&p.ID, &p.PublicID, &p.MerchantID, &p.Amount, &p.Currency, &p.Status,
		&p.AttemptCount, &p.NextAttemptAt, &p.LastError, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

func scanPaymentRecord(row pgx.Row) (*PaymentRecord, error) {
	var rec PaymentRecord
	p := &rec.Payment
	if err := row.Scan(
		&p.ID, &p.PublicID, &p.MerchantID, &p.Amount, &p.Currency, &p.Status,
		&p.AttemptCount, &p.NextAttemptAt, &p.LastError, &p.CreatedAt, &p.UpdatedAt,
		&rec.MerchantPublicID, &rec.WebhookURL,
	); err != nil {
		return nil, err
	}
	return &rec, nil
}

func scanIdempotencyKey(row pgx.Row) (*model.DBIdempotencyKey, error) {
	var k model.DBIdempotencyKey
	if err := row.Scan(
		&k.ID, &k.MerchantID, &k.IdempotencyKey, &k.Scope, &k.PaymentID, &k.RefundID, &k.RequestHash, &k.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &k, nil
}
