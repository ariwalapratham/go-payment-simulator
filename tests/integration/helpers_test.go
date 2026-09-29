//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/handler"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/ariwalapratham/go-payment-simulator/internal/server"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func requireDB(t *testing.T) {
	t.Helper()
	if testPool == nil || testCfg == nil {
		t.Skip("postgres not available")
	}
}

func resetDB(t *testing.T) {
	t.Helper()
	requireDB(t)
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `
TRUNCATE TABLE webhook_deliveries, webhook_events, idempotency_keys, refunds, payments, merchants
RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	_, err = testPool.Exec(ctx, `
INSERT INTO merchants (public_id, name, api_key_hash)
VALUES ($1, 'dev-merchant', 'dev-seed-hash')`, model.SeedMerchantPublicID)
	if err != nil {
		t.Fatalf("seed merchant: %v", err)
	}
}

func testRouter(t *testing.T) *gin.Engine {
	t.Helper()
	requireDB(t)
	gin.SetMode(gin.TestMode)
	log := zerolog.Nop()
	s := &server.Server{
		Config: testCfg,
		Logger: &log,
	}
	repo := repository.NewPaymentRepository(testPool)
	h := handler.NewPaymentHandler(service.NewPaymentService(repo), &log)
	return handler.NewRouter(s, middleware.NewMiddlewares(s), h)
}

func postPayment(t *testing.T, r *gin.Engine, merchantID, idempotencyKey, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Merchant-Id", merchantID)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func getPayment(t *testing.T, r *gin.Engine, merchantID, paymentID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/"+paymentID, nil)
	req.Header.Set("X-Merchant-Id", merchantID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodePayment(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	return body
}

func countPayments(t *testing.T) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM payments`).Scan(&n); err != nil {
		t.Fatalf("count payments: %v", err)
	}
	return n
}

func seedMerchantID() string {
	return model.SeedMerchantPublicID
}

func paymentJSON(amount int64, currency string) string {
	return fmt.Sprintf(`{"amount":%d,"currency":%q}`, amount, currency)
}

func randomKey() string {
	return uuid.NewString()
}
