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

	"github.com/ariwalapratham/go-payment-simulator/internal/database"
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

const testAdminAPIKey = "test-admin-key"

//nolint:gochecknoglobals // set by testRouter after creating the seed merchant
var (
	testMerchantAPIKey        string
	testMerchantPublicID      string
	testMerchantWebhookSecret string
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
	_, err := testPool.Exec(context.Background(), `
TRUNCATE TABLE webhook_deliveries, webhook_events, idempotency_keys, refunds, payments, merchants
RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	testMerchantAPIKey = ""
	testMerchantPublicID = ""
	testMerchantWebhookSecret = ""
}

func testRouter(t *testing.T) *gin.Engine {
	t.Helper()
	requireDB(t)
	gin.SetMode(gin.TestMode)
	log := zerolog.Nop()
	cfg := *testCfg
	cfg.Admin.APIKey = testAdminAPIKey
	s := &server.Server{
		Config: &cfg,
		Logger: &log,
		DB:     &database.Database{Pool: testPool},
	}
	repo := repository.NewPaymentRepository(testPool)
	merchantSvc := service.NewMerchantService(repository.NewMerchantRepository(testPool))
	created, err := merchantSvc.Create(context.Background(), "dev-merchant", nil)
	if err != nil {
		t.Fatalf("seed merchant: %v", err)
	}
	testMerchantAPIKey = created.APIKey
	testMerchantPublicID = created.PublicID.String()
	testMerchantWebhookSecret = created.WebhookSecret

	payments := handler.NewPaymentHandler(service.NewPaymentService(repo), &log)
	refunds := handler.NewRefundHandler(service.NewRefundService(repo), &log)
	admin := handler.NewAdminMerchantHandler(merchantSvc, &log)
	merchants := handler.NewMerchantHandler(merchantSvc, &log)
	return handler.NewRouter(s, middleware.NewMiddlewares(s, merchantSvc.PublicIDByAPIKey), payments, refunds, admin, merchants)
}

func postPayment(t *testing.T, r *gin.Engine, apiKey, idempotencyKey, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.HeaderAPIKey, apiKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func getPayment(t *testing.T, r *gin.Engine, apiKey, paymentID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/payments/"+paymentID, nil)
	req.Header.Set(middleware.HeaderAPIKey, apiKey)
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

func seedAPIKey() string {
	return testMerchantAPIKey
}

func createMerchant(t *testing.T, name string) (publicID, apiKey string) {
	t.Helper()
	requireDB(t)
	created, err := service.NewMerchantService(repository.NewMerchantRepository(testPool)).
		Create(context.Background(), name, nil)
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	return created.PublicID.String(), created.APIKey
}

func paymentJSON(amount int64, currency string) string {
	return fmt.Sprintf(`{"amount":%d,"currency":%q}`, amount, currency)
}

func randomKey() string {
	return uuid.NewString()
}

func postPaymentAction(t *testing.T, r *gin.Engine, apiKey, paymentID, action string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/"+paymentID+"/"+action, nil)
	req.Header.Set(middleware.HeaderAPIKey, apiKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func forcePaymentStatus(t *testing.T, publicID string, status model.PaymentStatus) {
	t.Helper()
	tag, err := testPool.Exec(context.Background(),
		`UPDATE payments SET status = $2, updated_at = now() WHERE public_id = $1`,
		publicID, status)
	if err != nil {
		t.Fatalf("force status: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("force status: rows=%d", tag.RowsAffected())
	}
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) (code, message string) {
	t.Helper()
	body := decodePayment(t, rec)
	errObj, _ := body["error"].(map[string]any)
	code, _ = errObj["code"].(string)
	message, _ = errObj["message"].(string)
	return code, message
}

func createPayment(t *testing.T, r *gin.Engine) string {
	t.Helper()
	rec := postPayment(t, r, seedAPIKey(), randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id, _ := decodePayment(t, rec)["id"].(string)
	if id == "" {
		t.Fatal("missing payment id")
	}
	return id
}

func capturedPayment(t *testing.T, r *gin.Engine) string {
	t.Helper()
	id := createPayment(t, r)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)
	rec := postPaymentAction(t, r, seedAPIKey(), id, "capture")
	if rec.Code != http.StatusOK {
		t.Fatalf("capture: %d %s", rec.Code, rec.Body.String())
	}
	return id
}

func refundJSON(amount int64) string {
	return fmt.Sprintf(`{"amount":%d}`, amount)
}

func postRefund(t *testing.T, r *gin.Engine, apiKey, paymentID, idempotencyKey, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/"+paymentID+"/refund", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.HeaderAPIKey, apiKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func getRefund(t *testing.T, r *gin.Engine, apiKey, refundID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/refunds/"+refundID, nil)
	req.Header.Set(middleware.HeaderAPIKey, apiKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func countRefunds(t *testing.T) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM refunds`).Scan(&n); err != nil {
		t.Fatalf("count refunds: %v", err)
	}
	return n
}

func succeededRefundSum(t *testing.T, paymentPublicID string) int64 {
	t.Helper()
	var sum int64
	err := testPool.QueryRow(context.Background(), `
SELECT COALESCE(SUM(r.amount), 0)::bigint
FROM refunds r
JOIN payments p ON p.id = r.payment_id
WHERE p.public_id = $1 AND r.status = 'SUCCEEDED'`, paymentPublicID).Scan(&sum)
	if err != nil {
		t.Fatalf("sum refunds: %v", err)
	}
	return sum
}
