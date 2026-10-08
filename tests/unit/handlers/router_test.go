package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/config"
	"github.com/ariwalapratham/go-payment-simulator/internal/handler"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/server"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const unitAPIKey = "sk_test_unit"

func unitAPIKeyLookup(_ context.Context, key string) (uuid.UUID, error) {
	if key == unitAPIKey {
		return uuid.MustParse("11111111-1111-1111-1111-111111111111"), nil
	}
	return uuid.Nil, service.ErrMerchantNotFound
}

func testRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	log := zerolog.Nop()
	s := &server.Server{
		Config: &config.Config{
			Primary: config.Primary{Env: "test"},
			Admin:   config.AdminConfig{APIKey: "test-admin-key"},
		},
		Logger: &log,
	}
	return handler.NewRouter(
		s,
		middleware.NewMiddlewares(s, unitAPIKeyLookup),
		handler.NewPaymentHandler(nil, &log),
		handler.NewRefundHandler(nil, &log),
		handler.NewAdminMerchantHandler(nil, &log),
		handler.NewMerchantHandler(nil, &log),
	)
}

func TestHealth(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status: %q", body["status"])
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyzWithoutDB(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestCreatePaymentInvalidAmount(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(`{"amount":0,"currency":"USD"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.HeaderAPIKey, unitAPIKey)
	req.Header.Set(middleware.HeaderIdempotencyKey, "k1")
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorField(t, rec, "INVALID_AMOUNT", "amount")
}

func TestCreatePaymentUnsupportedCurrency(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(`{"amount":5000,"currency":"XYZ"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.HeaderAPIKey, unitAPIKey)
	req.Header.Set(middleware.HeaderIdempotencyKey, "k1")
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorField(t, rec, "INVALID_CURRENCY", "currency")
}

func TestRefundInvalidAmount(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/11111111-1111-1111-1111-111111111111/refund", strings.NewReader(`{"amount":0}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.HeaderAPIKey, unitAPIKey)
	req.Header.Set(middleware.HeaderIdempotencyKey, "k1")
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorField(t, rec, "INVALID_AMOUNT", "amount")
}

func assertErrorField(t *testing.T, rec *httptest.ResponseRecorder, code, field string) {
	t.Helper()
	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	errObj := body["error"]
	if errObj["code"] != code {
		t.Fatalf("code: %q want %q", errObj["code"], code)
	}
	if errObj["field"] != field {
		t.Fatalf("field: %q want %q", errObj["field"], field)
	}
}

func TestPaymentAndRefundRoutesRequireAPIKey(t *testing.T) {
	t.Parallel()

	paymentID := "11111111-1111-1111-1111-111111111111"
	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/v1/payments", `{"amount":5000,"currency":"USD"}`},
		{http.MethodGet, "/v1/payments/" + paymentID, ""},
		{http.MethodPost, "/v1/payments/" + paymentID + "/capture", ""},
		{http.MethodPost, "/v1/payments/" + paymentID + "/cancel", ""},
		{http.MethodPost, "/v1/payments/" + paymentID + "/refund", `{"amount":1000}`},
		{http.MethodGet, "/v1/refunds/" + paymentID, ""},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path+" missing", func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			testRouter().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
			}
		})
		t.Run(tc.method+" "+tc.path+" bad key", func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set(middleware.HeaderAPIKey, "sk_test_bad")
			testRouter().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestCreatePaymentMissingHeaders(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(`{"amount":5000,"currency":"USD"}`))
	req.Header.Set("Content-Type", "application/json")
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCaptureInvalidPaymentID(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/not-a-uuid/capture", nil)
	req.Header.Set(middleware.HeaderAPIKey, unitAPIKey)
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRefundRequiresIdempotencyKey(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/11111111-1111-1111-1111-111111111111/refund", strings.NewReader(`{"amount":1000}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.HeaderAPIKey, unitAPIKey)
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPaymentsRejectMerchantIDHeader(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/11111111-1111-1111-1111-111111111111", nil)
	req.Header.Set(middleware.HeaderAPIKey, unitAPIKey)
	req.Header.Set(middleware.HeaderMerchantID, "11111111-1111-1111-1111-111111111111")
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}
