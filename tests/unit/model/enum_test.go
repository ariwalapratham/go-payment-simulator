package model_test

import (
	"errors"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
)

func TestPaymentStatusCanTransitionTo(t *testing.T) {
	t.Parallel()

	allowed := []struct {
		from, to model.PaymentStatus
	}{
		{model.PaymentStatusPending, model.PaymentStatusAuthorized},
		{model.PaymentStatusPending, model.PaymentStatusFailed},
		{model.PaymentStatusPending, model.PaymentStatusCancelled},
		{model.PaymentStatusAuthorized, model.PaymentStatusCaptured},
		{model.PaymentStatusAuthorized, model.PaymentStatusCancelled},
		{model.PaymentStatusCaptured, model.PaymentStatusRefunded},
	}
	for _, tc := range allowed {
		if !tc.from.CanTransitionTo(tc.to) {
			t.Fatalf("%s -> %s should be allowed", tc.from, tc.to)
		}
		if err := model.Transition(tc.from, tc.to); err != nil {
			t.Fatalf("Transition(%s, %s): %v", tc.from, tc.to, err)
		}
	}
}

func TestPaymentStatusIllegalTransitions(t *testing.T) {
	t.Parallel()

	illegal := []struct {
		from, to model.PaymentStatus
	}{
		{model.PaymentStatusPending, model.PaymentStatusCaptured},
		{model.PaymentStatusPending, model.PaymentStatusRefunded},
		{model.PaymentStatusAuthorized, model.PaymentStatusPending},
		{model.PaymentStatusAuthorized, model.PaymentStatusFailed},
		{model.PaymentStatusCaptured, model.PaymentStatusCancelled},
		{model.PaymentStatusFailed, model.PaymentStatusPending},
		{model.PaymentStatusFailed, model.PaymentStatusAuthorized},
		{model.PaymentStatusCancelled, model.PaymentStatusPending},
		{model.PaymentStatusRefunded, model.PaymentStatusCaptured},
	}
	for _, tc := range illegal {
		if tc.from.CanTransitionTo(tc.to) {
			t.Fatalf("%s -> %s should be rejected", tc.from, tc.to)
		}
		err := model.Transition(tc.from, tc.to)
		if !errors.Is(err, model.ErrInvalidTransition) {
			t.Fatalf("Transition(%s, %s): got %v", tc.from, tc.to, err)
		}
	}
}

func TestParsePaymentStatus(t *testing.T) {
	t.Parallel()

	s, err := model.ParsePaymentStatus("PENDING")
	if err != nil || s != model.PaymentStatusPending {
		t.Fatalf("parse PENDING: %v %q", err, s)
	}
	if _, err := model.ParsePaymentStatus("bogus"); err == nil {
		t.Fatal("expected error for bogus status")
	}
}
