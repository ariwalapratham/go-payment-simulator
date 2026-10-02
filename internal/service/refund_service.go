package service

import (
	"context"
	"errors"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrRefundNotFound       = errors.New("refund not found")
	ErrRefundExceedsBalance = errors.New("refund exceeds remaining balance")
)

type Refund struct {
	PublicID        uuid.UUID
	PaymentPublicID uuid.UUID
	Amount          int64
	Status          model.RefundStatus
	CreatedAt       time.Time
}

type refundStore interface {
	CreateRefundIdempotent(context.Context, repository.CreateRefundInput) (*repository.RefundRecord, bool, error)
	GetRefundByPublicID(context.Context, uuid.UUID, uuid.UUID) (*repository.RefundRecord, error)
}

type RefundService struct {
	store refundStore
}

// NewRefundService orchestrates create/get refunds; no HTTP types.
func NewRefundService(store refundStore) *RefundService {
	return &RefundService{store: store}
}

// RemainingRefundable is captured amount minus succeeded refunds.
func RemainingRefundable(captured, succeededSum int64) int64 {
	return repository.RemainingRefundable(captured, succeededSum)
}

// ErrIfOverRefund is ErrRefundExceedsBalance when requested does not fit in remaining.
func ErrIfOverRefund(captured, succeededSum, requested int64) error {
	if requested > RemainingRefundable(captured, succeededSum) {
		return ErrRefundExceedsBalance
	}
	return nil
}

// Create inserts a SUCCEEDED refund or returns the existing one for the same idempotency key.
// created is false on replay.
func (s *RefundService) Create(
	ctx context.Context,
	merchantPublicID, paymentPublicID uuid.UUID,
	idempotencyKey string,
	amount int64,
) (Refund, bool, error) {
	rec, created, err := s.store.CreateRefundIdempotent(ctx, repository.CreateRefundInput{
		MerchantPublicID: merchantPublicID,
		PaymentPublicID:  paymentPublicID,
		IdempotencyKey:   idempotencyKey,
		RequestHash:      RefundRequestHash(paymentPublicID, amount),
		Amount:           amount,
	})
	if err != nil {
		return Refund{}, false, mapRefundErr(err)
	}
	return toRefund(rec), created, nil
}

// Get loads a refund by public id, scoped to the merchant that owns the payment.
func (s *RefundService) Get(
	ctx context.Context,
	merchantPublicID, refundPublicID uuid.UUID,
) (Refund, error) {
	rec, err := s.store.GetRefundByPublicID(ctx, merchantPublicID, refundPublicID)
	if err != nil {
		return Refund{}, mapRefundErr(err)
	}
	return toRefund(rec), nil
}

func toRefund(rec *repository.RefundRecord) Refund {
	r := rec.Refund
	return Refund{
		PublicID:        r.PublicID,
		PaymentPublicID: rec.PaymentPublicID,
		Amount:          r.Amount,
		Status:          r.Status,
		CreatedAt:       r.CreatedAt,
	}
}

func mapRefundErr(err error) error {
	switch {
	case errors.Is(err, repository.ErrMerchantNotFound):
		return ErrMerchantNotFound
	case errors.Is(err, repository.ErrPaymentNotFound):
		return ErrPaymentNotFound
	case errors.Is(err, repository.ErrRefundNotFound):
		return ErrRefundNotFound
	case errors.Is(err, repository.ErrIdempotencyKeyReused):
		return ErrIdempotencyKeyReused
	case errors.Is(err, repository.ErrRefundExceedsBalance):
		return ErrRefundExceedsBalance
	default:
		return err
	}
}
