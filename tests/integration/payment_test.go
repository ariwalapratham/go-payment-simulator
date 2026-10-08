//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCreatePayment_CreatesPending(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	rec := postPayment(t, r, seedAPIKey(), randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodePayment(t, rec)
	if body["status"] != "PENDING" {
		t.Fatalf("status: %v", body["status"])
	}
	if body["id"] == nil || body["id"] == "" {
		t.Fatal("missing id")
	}
	if countPayments(t) != 1 {
		t.Fatalf("payment rows: %d", countPayments(t))
	}
}

func TestCreatePayment_IdempotentReplay(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	key := randomKey()
	body := paymentJSON(5000, "USD")

	first := postPayment(t, r, seedAPIKey(), key, body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: got %d body=%s", first.Code, first.Body.String())
	}
	second := postPayment(t, r, seedAPIKey(), key, body)
	if second.Code != http.StatusOK {
		t.Fatalf("replay: got %d body=%s", second.Code, second.Body.String())
	}

	a := decodePayment(t, first)
	b := decodePayment(t, second)
	if a["id"] != b["id"] {
		t.Fatalf("ids differ: %v vs %v", a["id"], b["id"])
	}
	if countPayments(t) != 1 {
		t.Fatalf("payment rows: %d", countPayments(t))
	}
}

func TestCreatePayment_KeyReuseDifferentBody(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	key := randomKey()

	first := postPayment(t, r, seedAPIKey(), key, paymentJSON(5000, "USD"))
	if first.Code != http.StatusCreated {
		t.Fatalf("first: got %d body=%s", first.Code, first.Body.String())
	}
	second := postPayment(t, r, seedAPIKey(), key, paymentJSON(6000, "USD"))
	if second.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatch: got %d body=%s", second.Code, second.Body.String())
	}
	decoded := decodePayment(t, second)
	errObj, _ := decoded["error"].(map[string]any)
	if errObj["code"] != "idempotency_key_reused_with_different_payload" {
		t.Fatalf("code: %v", errObj["code"])
	}
}

func TestGetPayment_NotFound(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	rec := getPayment(t, r, seedAPIKey(), uuid.NewString())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatePayment_InvalidAmount(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	rec := postPayment(t, r, seedAPIKey(), randomKey(), paymentJSON(0, "USD"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodePayment(t, rec)
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "INVALID_AMOUNT" || errObj["field"] != "amount" {
		t.Fatalf("error: %+v", errObj)
	}
}

func TestCreatePayment_UnsupportedCurrency(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	rec := postPayment(t, r, seedAPIKey(), randomKey(), paymentJSON(5000, "XYZ"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodePayment(t, rec)
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "INVALID_CURRENCY" || errObj["field"] != "currency" {
		t.Fatalf("error: %+v", errObj)
	}
}

func TestGetPayment_OK(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	created := postPayment(t, r, seedAPIKey(), randomKey(), paymentJSON(2500, "EUR"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: got %d body=%s", created.Code, created.Body.String())
	}
	id, _ := decodePayment(t, created)["id"].(string)
	got := getPayment(t, r, seedAPIKey(), id)
	if got.Code != http.StatusOK {
		t.Fatalf("get: got %d body=%s", got.Code, got.Body.String())
	}
}
