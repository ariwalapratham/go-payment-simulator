package model_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
)

func TestPaymentStatusCanTransitionTo(t *testing.T) {
	if !model.PaymentStatusPending.CanTransitionTo(model.PaymentStatusAuthorized) {
		t.Fatal("PENDING -> AUTHORIZED")
	}
	if model.PaymentStatusPending.CanTransitionTo(model.PaymentStatusCaptured) {
		t.Fatal("PENDING -> CAPTURED should be false")
	}
	if model.PaymentStatusFailed.CanTransitionTo(model.PaymentStatusPending) {
		t.Fatal("terminal FAILED should not transition")
	}
}

func TestParsePaymentStatus(t *testing.T) {
	s, err := model.ParsePaymentStatus("PENDING")
	if err != nil || s != model.PaymentStatusPending {
		t.Fatalf("parse PENDING: %v %q", err, s)
	}
	if _, err := model.ParsePaymentStatus("bogus"); err == nil {
		t.Fatal("expected error for bogus status")
	}
}
