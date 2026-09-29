package service_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/service"
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
