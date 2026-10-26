package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/webhook"
)

func TestSignAndVerify(t *testing.T) {
	t.Parallel()

	secret := "a1b2c3d4"
	body := []byte(`{"id":"ev-1","type":"payment.captured"}`)
	ts := int64(1_710_000_000)
	now := time.Unix(ts, 0)

	sig := webhook.SignBody(secret, ts, body)
	if sig == "" {
		t.Fatal("empty signature")
	}
	if !webhook.VerifySignature(secret, ts, body, sig, now, webhook.DefaultSignatureSkew) {
		t.Fatal("verify failed")
	}
	if webhook.VerifySignature(secret, ts, body, "deadbeef", now, webhook.DefaultSignatureSkew) {
		t.Fatal("expected invalid signature")
	}
	if webhook.VerifySignature("other", ts, body, sig, now, webhook.DefaultSignatureSkew) {
		t.Fatal("wrong secret should not verify")
	}
	if webhook.VerifySignature(secret, ts+1, body, sig, now, webhook.DefaultSignatureSkew) {
		t.Fatal("wrong timestamp should not verify")
	}
}

func TestVerifySignatureSkew(t *testing.T) {
	t.Parallel()

	secret := "a1b2c3d4"
	body := []byte(`{"id":"ev-1"}`)
	ts := int64(1_710_000_000)
	sig := webhook.SignBody(secret, ts, body)
	now := time.Unix(ts, 0)

	if !webhook.VerifySignature(secret, ts, body, sig, now.Add(4*time.Minute), webhook.DefaultSignatureSkew) {
		t.Fatal("4m skew should pass")
	}
	if webhook.VerifySignature(secret, ts, body, sig, now.Add(6*time.Minute), webhook.DefaultSignatureSkew) {
		t.Fatal("6m skew should fail")
	}
	if webhook.VerifySignature(secret, 0, body, sig, now, webhook.DefaultSignatureSkew) {
		t.Fatal("ts < 1 should fail")
	}

	old := hmac.New(sha256.New, []byte(secret))
	old.Write(body)
	oldSig := hex.EncodeToString(old.Sum(nil))
	if webhook.VerifySignature(secret, ts, body, oldSig, now, webhook.DefaultSignatureSkew) {
		t.Fatal("body-only MAC should not verify")
	}
}
