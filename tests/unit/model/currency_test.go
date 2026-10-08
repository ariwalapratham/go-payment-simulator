package model_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
)

func TestNormalizeCurrency(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"USD", "EUR", "GBP", "INR", "JPY", "CAD", "AUD"} {
		got, ok := model.NormalizeCurrency(code)
		if !ok || got != code {
			t.Fatalf("%s: got %q ok=%v", code, got, ok)
		}
	}
	got, ok := model.NormalizeCurrency("inr")
	if !ok || got != "INR" {
		t.Fatalf("inr: got %q ok=%v", got, ok)
	}
	if _, ok := model.NormalizeCurrency("XYZ"); ok {
		t.Fatal("XYZ should be rejected")
	}
	if _, ok := model.NormalizeCurrency(""); ok {
		t.Fatal("empty should be rejected")
	}
}
