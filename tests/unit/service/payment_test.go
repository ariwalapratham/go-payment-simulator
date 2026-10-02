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

type fakePayments struct {
	rec *repository.PaymentRecord
}

func (f *fakePayments) CreatePaymentIdempotent(
	context.Context, repository.CreatePaymentInput,
) (*repository.PaymentRecord, bool, error) {
	return nil, false, errors.New("unused")
}

func (f *fakePayments) GetPaymentByPublicID(context.Context, uuid.UUID, uuid.UUID) (*repository.PaymentRecord, error) {
	return nil, repository.ErrPaymentNotFound
}

func (f *fakePayments) TransitionPayment(
	_ context.Context, _, _ uuid.UUID, next model.PaymentStatus,
) (*repository.PaymentRecord, error) {
	if f.rec == nil {
		return nil, repository.ErrPaymentNotFound
	}
	if err := model.Transition(f.rec.Payment.Status, next); err != nil {
		return nil, err
	}
	out := *f.rec
	out.Payment.Status = next
	return &out, nil
}

func paymentRec(status model.PaymentStatus) *repository.PaymentRecord {
	return &repository.PaymentRecord{
		MerchantPublicID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Payment: model.DBPayment{
			ID:       1,
			PublicID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Amount:   5000,
			Currency: "USD",
			Status:   status,
		},
	}
}

func TestCaptureFromAuthorized(t *testing.T) {
	t.Parallel()

	svc := service.NewPaymentService(&fakePayments{rec: paymentRec(model.PaymentStatusAuthorized)})
	got, err := svc.Capture(context.Background(), uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.PaymentStatusCaptured {
		t.Fatalf("status %s", got.Status)
	}
}

func TestCaptureRejectedFromPendingAndCaptured(t *testing.T) {
	t.Parallel()

	for _, from := range []model.PaymentStatus{model.PaymentStatusPending, model.PaymentStatusCaptured} {
		svc := service.NewPaymentService(&fakePayments{rec: paymentRec(from)})
		_, err := svc.Capture(context.Background(), uuid.Nil, uuid.Nil)
		if !errors.Is(err, model.ErrInvalidTransition) {
			t.Fatalf("from %s: got %v", from, err)
		}
	}
}

func TestCancelFromPendingAndAuthorized(t *testing.T) {
	t.Parallel()

	for _, from := range []model.PaymentStatus{model.PaymentStatusPending, model.PaymentStatusAuthorized} {
		svc := service.NewPaymentService(&fakePayments{rec: paymentRec(from)})
		got, err := svc.Cancel(context.Background(), uuid.Nil, uuid.Nil)
		if err != nil {
			t.Fatalf("from %s: %v", from, err)
		}
		if got.Status != model.PaymentStatusCancelled {
			t.Fatalf("from %s: status %s", from, got.Status)
		}
	}
}

func TestCancelRejectedFromCaptured(t *testing.T) {
	t.Parallel()

	svc := service.NewPaymentService(&fakePayments{rec: paymentRec(model.PaymentStatusCaptured)})
	_, err := svc.Cancel(context.Background(), uuid.Nil, uuid.Nil)
	if !errors.Is(err, model.ErrInvalidTransition) {
		t.Fatalf("got %v", err)
	}
}

func TestCaptureNotFound(t *testing.T) {
	t.Parallel()

	svc := service.NewPaymentService(&fakePayments{})
	_, err := svc.Capture(context.Background(), uuid.Nil, uuid.Nil)
	if !errors.Is(err, service.ErrPaymentNotFound) {
		t.Fatalf("got %v", err)
	}
}
