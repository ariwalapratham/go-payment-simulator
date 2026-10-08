//go:build integration

package integration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/gin-gonic/gin"
)

func TestPaymentRoutesUnauthorizedWithoutAPIKey(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := "11111111-1111-1111-1111-111111111111"

	cases := []struct {
		method, path, body string
		idempotency        bool
	}{
		{http.MethodPost, "/v1/payments", paymentJSON(5000, "USD"), true},
		{http.MethodGet, "/v1/payments/" + id, "", false},
		{http.MethodPost, "/v1/payments/" + id + "/capture", "", false},
		{http.MethodPost, "/v1/payments/" + id + "/cancel", "", false},
		{http.MethodPost, "/v1/payments/" + id + "/refund", refundJSON(1000), true},
		{http.MethodGet, "/v1/refunds/" + id, "", false},
	}
	for _, tc := range cases {
		rec := serveAuth(t, r, tc.method, tc.path, tc.body, "", tc.idempotency)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: got %d want 401 body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestPaymentRoutesUnauthorizedBadAPIKey(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	rec := postPayment(t, r, "sk_test_unknown", randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("create: got %d want 401 body=%s", rec.Code, rec.Body.String())
	}
	rec = getPayment(t, r, "sk_test_unknown", "11111111-1111-1111-1111-111111111111")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("get: got %d want 401 body=%s", rec.Code, rec.Body.String())
	}
	rec = postPaymentAction(t, r, "sk_test_unknown", "11111111-1111-1111-1111-111111111111", "capture")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("capture: got %d want 401 body=%s", rec.Code, rec.Body.String())
	}
}

func TestPaymentsScopedToAPIKeyMerchant(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	ownerKey := seedAPIKey()
	_, otherKey := createMerchant(t, "other-merchant")

	created := postPayment(t, r, ownerKey, randomKey(), paymentJSON(5000, "USD"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	id, _ := decodePayment(t, created)["id"].(string)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)

	if rec := getPayment(t, r, otherKey, id); rec.Code != http.StatusNotFound {
		t.Fatalf("other get: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postPaymentAction(t, r, otherKey, id, "capture"); rec.Code != http.StatusNotFound {
		t.Fatalf("other capture: %d %s", rec.Code, rec.Body.String())
	}

	got := getPayment(t, r, ownerKey, id)
	if got.Code != http.StatusOK {
		t.Fatalf("owner get: %d %s", got.Code, got.Body.String())
	}
	if decodePayment(t, got)["merchant_id"] != testMerchantPublicID {
		t.Fatalf("merchant_id: %+v", decodePayment(t, got))
	}
	cap := postPaymentAction(t, r, ownerKey, id, "capture")
	if cap.Code != http.StatusOK {
		t.Fatalf("owner capture: %d %s", cap.Code, cap.Body.String())
	}
}

func TestPaymentRoutesRejectRotatedAPIKey(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	oldKey := seedAPIKey()

	rotated := adminJSON(t, r, http.MethodPost, "/v1/admin/merchants/"+testMerchantPublicID+"/rotate-key", "")
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rotated.Code, rotated.Body.String())
	}
	newKey, _ := decodePayment(t, rotated)["api_key"].(string)
	if newKey == "" || newKey == oldKey {
		t.Fatalf("rotate body %s", rotated.Body.String())
	}

	if rec := postPayment(t, r, oldKey, randomKey(), paymentJSON(5000, "USD")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old key create: %d %s", rec.Code, rec.Body.String())
	}
	if rec := getPayment(t, r, oldKey, "11111111-1111-1111-1111-111111111111"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old key get: %d %s", rec.Code, rec.Body.String())
	}

	created := postPayment(t, r, newKey, randomKey(), paymentJSON(5000, "USD"))
	if created.Code != http.StatusCreated {
		t.Fatalf("new key create: %d %s", created.Code, created.Body.String())
	}
}

func serveAuth(t *testing.T, r *gin.Engine, method, path, body, apiKey string, idempotency bool) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set(middleware.HeaderAPIKey, apiKey)
	}
	if idempotency {
		req.Header.Set(middleware.HeaderIdempotencyKey, randomKey())
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
