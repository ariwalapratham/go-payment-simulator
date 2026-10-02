package service_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/google/uuid"
)

func TestRequestHashStable(t *testing.T) {
	t.Parallel()

	a := service.RequestHash(5000, "usd")
	b := service.RequestHash(5000, "USD")
	if a != b {
		t.Fatalf("hash should ignore currency case: %s vs %s", a, b)
	}
	if a == "" {
		t.Fatal("empty hash")
	}
}

func TestRequestHashDiffersForPayload(t *testing.T) {
	t.Parallel()

	if service.RequestHash(5000, "USD") == service.RequestHash(5001, "USD") {
		t.Fatal("different amounts should hash differently")
	}
	if service.RequestHash(5000, "USD") == service.RequestHash(5000, "EUR") {
		t.Fatal("different currencies should hash differently")
	}
}

func TestRefundRequestHashStableAndDistinct(t *testing.T) {
	t.Parallel()

	pay := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	other := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	a := service.RefundRequestHash(pay, 3000)
	if a != service.RefundRequestHash(pay, 3000) {
		t.Fatal("hash should be stable")
	}
	if a == service.RefundRequestHash(pay, 3001) {
		t.Fatal("different amounts should hash differently")
	}
	if a == service.RefundRequestHash(other, 3000) {
		t.Fatal("different payments should hash differently")
	}
	if a == service.RequestHash(3000, "USD") {
		t.Fatal("refund hash must not match payment-create hash")
	}
}
