//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
)

func TestCaptureAuthorizedPayment(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)

	rec := postPaymentAction(t, r, seedAPIKey(), id, "capture")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	if decodePayment(t, rec)["status"] != "CAPTURED" {
		t.Fatalf("status: %v", decodePayment(t, rec)["status"])
	}
}

func TestCaptureRejectedWhenPending(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)

	rec := postPaymentAction(t, r, seedAPIKey(), id, "capture")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	code, msg := errorCode(t, rec)
	if code != "invalid_state_transition" {
		t.Fatalf("code: %s", code)
	}
	if msg == "" {
		t.Fatal("empty conflict message")
	}
}

func TestCaptureTwiceConflicts(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)

	first := postPaymentAction(t, r, seedAPIKey(), id, "capture")
	if first.Code != http.StatusOK {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	second := postPaymentAction(t, r, seedAPIKey(), id, "capture")
	if second.Code != http.StatusConflict {
		t.Fatalf("second: %d %s", second.Code, second.Body.String())
	}
	_, msg := errorCode(t, second)
	if !strings.Contains(msg, "CAPTURED") {
		t.Fatalf("message %q should include current status", msg)
	}
}

func TestCancelPendingPayment(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)

	rec := postPaymentAction(t, r, seedAPIKey(), id, "cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	if decodePayment(t, rec)["status"] != "CANCELLED" {
		t.Fatalf("status: %v", decodePayment(t, rec)["status"])
	}
}

func TestCancelAuthorizedPayment(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)

	rec := postPaymentAction(t, r, seedAPIKey(), id, "cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	if decodePayment(t, rec)["status"] != "CANCELLED" {
		t.Fatalf("status: %v", decodePayment(t, rec)["status"])
	}
}

func TestCancelAfterCaptureConflicts(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)
	cap := postPaymentAction(t, r, seedAPIKey(), id, "capture")
	if cap.Code != http.StatusOK {
		t.Fatalf("capture: %d %s", cap.Code, cap.Body.String())
	}

	rec := postPaymentAction(t, r, seedAPIKey(), id, "cancel")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	code, _ := errorCode(t, rec)
	if code != "invalid_state_transition" {
		t.Fatalf("code: %s", code)
	}
}

func TestCaptureOtherMerchantNotFound(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)

	_, otherKey := createMerchant(t, "other-merchant")
	rec := postPaymentAction(t, r, otherKey, id, "capture")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
}
