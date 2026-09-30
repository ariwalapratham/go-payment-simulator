package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrMerchantNotFound     = errors.New("merchant not found")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrIdempotencyKeyReused = errors.New("idempotency key reused with a different payload")
)

type Payment struct {
	PublicID         uuid.UUID
	MerchantPublicID uuid.UUID
	Amount           int64
	Currency         string
	Status           model.PaymentStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type paymentRepository interface {
	CreatePaymentIdempotent(context.Context, repository.CreatePaymentInput) (*repository.PaymentRecord, bool, error)
	GetPaymentByPublicID(context.Context, uuid.UUID, uuid.UUID) (*repository.PaymentRecord, error)
}

type PaymentService struct {
	payments paymentRepository
}

// NewPaymentService orchestrates create/get; no HTTP types.
func NewPaymentService(payments paymentRepository) *PaymentService {
	return &PaymentService{payments: payments}
}

// Create inserts a PENDING payment or returns the existing one for the same idempotency key.
// created is false on replay.
func (s *PaymentService) Create(
	ctx context.Context,
	merchantPublicID uuid.UUID,
	idempotencyKey string,
	amount int64,
	currency string,
) (Payment, bool, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	rec, created, err := s.payments.CreatePaymentIdempotent(ctx, repository.CreatePaymentInput{
		MerchantPublicID: merchantPublicID,
		IdempotencyKey:   idempotencyKey,
		RequestHash:      RequestHash(amount, currency),
		Amount:           amount,
		Currency:         currency,
	})
	if err != nil {
		return Payment{}, false, mapRepoErr(err)
	}
	return toPayment(rec), created, nil
}

// Get loads a payment by public id, scoped to the merchant.
func (s *PaymentService) Get(
	ctx context.Context,
	merchantPublicID, paymentPublicID uuid.UUID,
) (Payment, error) {
	rec, err := s.payments.GetPaymentByPublicID(ctx, merchantPublicID, paymentPublicID)
	if err != nil {
		return Payment{}, mapRepoErr(err)
	}
	return toPayment(rec), nil
}

func toPayment(rec *repository.PaymentRecord) Payment {
	p := rec.Payment
	return Payment{
		PublicID:         p.PublicID,
		MerchantPublicID: rec.MerchantPublicID,
		Amount:           p.Amount,
		Currency:         p.Currency,
		Status:           p.Status,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

// mapRepoErr keeps HTTP mapping in the handler; service only uses domain sentinels.
func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, repository.ErrMerchantNotFound):
		return ErrMerchantNotFound
	case errors.Is(err, repository.ErrPaymentNotFound):
		return ErrPaymentNotFound
	case errors.Is(err, repository.ErrIdempotencyKeyReused):
		return ErrIdempotencyKeyReused
	default:
		return err
	}
}
