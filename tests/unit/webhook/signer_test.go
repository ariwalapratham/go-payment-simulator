package webhook_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/webhook"
)

func TestSignAndVerify(t *testing.T) {
	secret := "a1b2c3d4"
	body := []byte(`{"id":"ev-1","type":"payment.captured"}`)

	sig := webhook.SignBody(secret, body)
	if sig == "" {
		t.Fatal("empty signature")
	}
	if !webhook.VerifySignature(secret, body, sig) {
		t.Fatal("verify failed")
	}
	if webhook.VerifySignature(secret, body, "deadbeef") {
		t.Fatal("expected invalid signature")
	}
	if webhook.VerifySignature("other", body, sig) {
		t.Fatal("wrong secret should not verify")
	}
}
