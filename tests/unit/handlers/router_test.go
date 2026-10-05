package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/config"
	"github.com/ariwalapratham/go-payment-simulator/internal/handler"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/server"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

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
		middleware.NewMiddlewares(s),
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

func TestCreatePaymentMissingHeaders(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", strings.NewReader(`{"amount":5000,"currency":"USD"}`))
	req.Header.Set("Content-Type", "application/json")
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCaptureRequiresMerchantID(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/11111111-1111-1111-1111-111111111111/capture", nil)
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCancelRequiresMerchantID(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/11111111-1111-1111-1111-111111111111/cancel", nil)
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCaptureInvalidPaymentID(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/not-a-uuid/capture", nil)
	req.Header.Set("X-Merchant-Id", "11111111-1111-1111-1111-111111111111")
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRefundRequiresMerchantID(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/11111111-1111-1111-1111-111111111111/refund", strings.NewReader(`{"amount":1000}`))
	req.Header.Set("Content-Type", "application/json")
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
	req.Header.Set("X-Merchant-Id", "11111111-1111-1111-1111-111111111111")
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetRefundRequiresMerchantID(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/refunds/11111111-1111-1111-1111-111111111111", nil)
	testRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}
