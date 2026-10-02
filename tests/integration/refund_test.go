//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/google/uuid"
)

func TestRefundFullAmount(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)

	rec := postRefund(t, r, seedMerchantID(), id, randomKey(), refundJSON(5000))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodePayment(t, rec)
	if body["status"] != "SUCCEEDED" || body["amount"] != float64(5000) {
		t.Fatalf("refund: %v", body)
	}
	got := getPayment(t, r, seedMerchantID(), id)
	if decodePayment(t, got)["status"] != "REFUNDED" {
		t.Fatalf("payment status: %v", decodePayment(t, got)["status"])
	}
}

func TestRefundPartialThenFullThenReject(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)

	first := postRefund(t, r, seedMerchantID(), id, randomKey(), refundJSON(3000))
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	if decodePayment(t, getPayment(t, r, seedMerchantID(), id))["status"] != "CAPTURED" {
		t.Fatal("partial refund must leave payment CAPTURED")
	}

	second := postRefund(t, r, seedMerchantID(), id, randomKey(), refundJSON(2000))
	if second.Code != http.StatusCreated {
		t.Fatalf("second: %d %s", second.Code, second.Body.String())
	}
	if decodePayment(t, getPayment(t, r, seedMerchantID(), id))["status"] != "REFUNDED" {
		t.Fatal("zero remaining must set REFUNDED")
	}

	third := postRefund(t, r, seedMerchantID(), id, randomKey(), refundJSON(1))
	if third.Code != http.StatusConflict {
		t.Fatalf("third: %d %s", third.Code, third.Body.String())
	}
	code, _ := errorCode(t, third)
	if code != "invalid_state_transition" {
		t.Fatalf("code: %s", code)
	}
}

func TestRefundExceedsRemaining(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)
	if rec := postRefund(t, r, seedMerchantID(), id, randomKey(), refundJSON(3000)); rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}

	rec := postRefund(t, r, seedMerchantID(), id, randomKey(), refundJSON(3000))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	code, _ := errorCode(t, rec)
	if code != "refund_exceeds_balance" {
		t.Fatalf("code: %s", code)
	}
	if decodePayment(t, getPayment(t, r, seedMerchantID(), id))["status"] != "CAPTURED" {
		t.Fatal("over-refund must leave payment CAPTURED")
	}
}

func TestRefundRejectedWhenAuthorized(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)

	rec := postRefund(t, r, seedMerchantID(), id, randomKey(), refundJSON(1000))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRefundIdempotentReplay(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)
	key := randomKey()
	body := refundJSON(3000)

	first := postRefund(t, r, seedMerchantID(), id, key, body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	second := postRefund(t, r, seedMerchantID(), id, key, body)
	if second.Code != http.StatusOK {
		t.Fatalf("replay: %d %s", second.Code, second.Body.String())
	}
	if decodePayment(t, first)["id"] != decodePayment(t, second)["id"] {
		t.Fatal("replay must return the same refund")
	}
	if countRefunds(t) != 1 {
		t.Fatalf("refund rows: %d", countRefunds(t))
	}
}

func TestRefundIdempotentReplayAfterFullyRefunded(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)
	key := randomKey()
	body := refundJSON(5000)

	first := postRefund(t, r, seedMerchantID(), id, key, body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	second := postRefund(t, r, seedMerchantID(), id, key, body)
	if second.Code != http.StatusOK {
		t.Fatalf("replay of full refund: %d %s", second.Code, second.Body.String())
	}
	if countRefunds(t) != 1 {
		t.Fatalf("refund rows: %d", countRefunds(t))
	}
}

func TestRefundKeyReuseDifferentAmount(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)
	key := randomKey()

	first := postRefund(t, r, seedMerchantID(), id, key, refundJSON(3000))
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	second := postRefund(t, r, seedMerchantID(), id, key, refundJSON(2000))
	if second.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatch: %d %s", second.Code, second.Body.String())
	}
	code, _ := errorCode(t, second)
	if code != "idempotency_key_reused_with_different_payload" {
		t.Fatalf("code: %s", code)
	}
}

func TestGetRefundOKAndNotFound(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)
	created := postRefund(t, r, seedMerchantID(), id, randomKey(), refundJSON(1000))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	refundID, _ := decodePayment(t, created)["id"].(string)

	got := getRefund(t, r, seedMerchantID(), refundID)
	if got.Code != http.StatusOK {
		t.Fatalf("get: %d %s", got.Code, got.Body.String())
	}

	missing := getRefund(t, r, seedMerchantID(), uuid.NewString())
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing: %d %s", missing.Code, missing.Body.String())
	}

	other := getRefund(t, r, uuid.NewString(), refundID)
	if other.Code != http.StatusNotFound {
		t.Fatalf("other merchant: %d %s", other.Code, other.Body.String())
	}
}

func TestRefundSameCreateKeyIsIndependentScope(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	key := randomKey()
	created := postPayment(t, r, seedMerchantID(), key, paymentJSON(5000, "USD"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	id, _ := decodePayment(t, created)["id"].(string)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)
	if rec := postPaymentAction(t, r, seedMerchantID(), id, "capture"); rec.Code != http.StatusOK {
		t.Fatalf("capture: %d %s", rec.Code, rec.Body.String())
	}

	rec := postRefund(t, r, seedMerchantID(), id, key, refundJSON(1000))
	if rec.Code != http.StatusCreated {
		t.Fatalf("scoped refund: %d %s", rec.Code, rec.Body.String())
	}
}
