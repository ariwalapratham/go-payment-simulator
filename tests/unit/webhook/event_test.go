package webhook_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/webhook"
	"github.com/google/uuid"
)

func TestMarshalOutboundRoundTrip(t *testing.T) {
	pid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	evID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	created := time.Date(2026, 9, 20, 10, 15, 3, 123456789, time.UTC)

	raw, err := webhook.DataPayload(pid, model.PaymentStatusCaptured, 2500, "USD")
	if err != nil {
		t.Fatal(err)
	}

	body, err := webhook.MarshalOutbound(evID, model.WebhookEventPaymentCaptured, created, raw)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if string(doc["id"]) != `"22222222-2222-2222-2222-222222222222"` {
		t.Fatalf("id: %s", doc["id"])
	}
	if string(doc["type"]) != `"payment.captured"` {
		t.Fatalf("type: %s", doc["type"])
	}
	if string(doc["created_at"]) != `"2026-09-20T10:15:03Z"` {
		t.Fatalf("created_at: %s", doc["created_at"])
	}

	sig := webhook.SignBody("test-secret", body)
	if !webhook.VerifySignature("test-secret", body, sig) {
		t.Fatal("signature mismatch on marshaled body")
	}
}
