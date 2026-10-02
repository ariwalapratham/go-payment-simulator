package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/google/uuid"
)

func TestErrIfOverRefund(t *testing.T) {
	t.Parallel()

	if err := service.ErrIfOverRefund(5000, 2000, 3000); err != nil {
		t.Fatalf("exact remaining: %v", err)
	}
	if err := service.ErrIfOverRefund(5000, 0, 5000); err != nil {
		t.Fatalf("full refund: %v", err)
	}
	if err := service.ErrIfOverRefund(5000, 3000, 3000); !errors.Is(err, service.ErrRefundExceedsBalance) {
		t.Fatalf("over refund: %v", err)
	}
}

func TestRemainingRefundableZeroMeansFullyRefunded(t *testing.T) {
	t.Parallel()

	if got := service.RemainingRefundable(5000, 5000); got != 0 {
		t.Fatalf("remaining %d", got)
	}
}

type fakeRefunds struct {
	status    model.PaymentStatus
	captured  int64
	succeeded int64
	created   *repository.RefundRecord
	createErr error
	getRec    *repository.RefundRecord
	getErr    error
	lastInput repository.CreateRefundInput
}

func (f *fakeRefunds) CreateRefundIdempotent(_ context.Context, in repository.CreateRefundInput) (*repository.RefundRecord, bool, error) {
	f.lastInput = in
	if f.createErr != nil {
		return nil, false, f.createErr
	}
	if f.status != model.PaymentStatusCaptured {
		return nil, false, model.Transition(f.status, model.PaymentStatusRefunded)
	}
	if err := service.ErrIfOverRefund(f.captured, f.succeeded, in.Amount); err != nil {
		return nil, false, repository.ErrRefundExceedsBalance
	}
	rec := f.created
	if rec == nil {
		rec = &repository.RefundRecord{
			PaymentPublicID:  in.PaymentPublicID,
			MerchantPublicID: in.MerchantPublicID,
			Refund: model.DBRefund{
				PublicID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
				Amount:   in.Amount,
				Status:   model.RefundStatusSucceeded,
			},
		}
	}
	return rec, true, nil
}

func (f *fakeRefunds) GetRefundByPublicID(context.Context, uuid.UUID, uuid.UUID) (*repository.RefundRecord, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getRec == nil {
		return nil, repository.ErrRefundNotFound
	}
	return f.getRec, nil
}

func TestRefundCreateFromCaptured(t *testing.T) {
	t.Parallel()

	payID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	store := &fakeRefunds{status: model.PaymentStatusCaptured, captured: 5000}
	svc := service.NewRefundService(store)
	got, created, err := svc.Create(context.Background(), uuid.Nil, payID, "k1", 3000)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if got.Amount != 3000 || got.Status != model.RefundStatusSucceeded {
		t.Fatalf("%+v", got)
	}
	if store.lastInput.RequestHash != service.RefundRequestHash(payID, 3000) {
		t.Fatal("hash not bound to payment and amount")
	}
}

func TestRefundCreateRejectedWhenNotCaptured(t *testing.T) {
	t.Parallel()

	store := &fakeRefunds{status: model.PaymentStatusAuthorized, captured: 5000}
	svc := service.NewRefundService(store)
	_, _, err := svc.Create(context.Background(), uuid.Nil, uuid.Nil, "k1", 1000)
	if !errors.Is(err, model.ErrInvalidTransition) {
		t.Fatalf("got %v", err)
	}
}

func TestRefundCreateOverBalance(t *testing.T) {
	t.Parallel()

	store := &fakeRefunds{status: model.PaymentStatusCaptured, captured: 5000, succeeded: 3000}
	svc := service.NewRefundService(store)
	_, _, err := svc.Create(context.Background(), uuid.Nil, uuid.Nil, "k1", 3000)
	if !errors.Is(err, service.ErrRefundExceedsBalance) {
		t.Fatalf("got %v", err)
	}
}

func TestRefundGetNotFound(t *testing.T) {
	t.Parallel()

	svc := service.NewRefundService(&fakeRefunds{})
	_, err := svc.Get(context.Background(), uuid.Nil, uuid.Nil)
	if !errors.Is(err, service.ErrRefundNotFound) {
		t.Fatalf("got %v", err)
	}
}
