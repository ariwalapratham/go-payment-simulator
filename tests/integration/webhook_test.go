//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/ariwalapratham/go-payment-simulator/internal/webhook"
	"github.com/ariwalapratham/go-payment-simulator/internal/worker"
	"github.com/rs/zerolog"
)

type webhookCapture struct {
	mu     sync.Mutex
	events []receivedWebhook
}

type receivedWebhook struct {
	EventID   string
	Type      string
	Body      []byte
	Signature string
}

func (c *webhookCapture) handler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		sig := r.Header.Get("X-Webhook-Signature")
		if !webhook.VerifySignature(secret, body, sig) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		eventID := r.Header.Get("X-Webhook-Event-Id")
		var doc struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(body, &doc)
		c.mu.Lock()
		c.events = append(c.events, receivedWebhook{
			EventID:   eventID,
			Type:      doc.Type,
			Body:      body,
			Signature: sig,
		})
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func (c *webhookCapture) waitForType(t *testing.T, want string, timeout time.Duration) receivedWebhook {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, ev := range c.events {
			if ev.Type == want {
				c.mu.Unlock()
				return ev
			}
		}
		c.mu.Unlock()
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for webhook type %s", want)
	return receivedWebhook{}
}

func setMerchantWebhookURL(t *testing.T, url string) {
	t.Helper()
	tag, err := testPool.Exec(context.Background(),
		`UPDATE merchants SET webhook_url = $2 WHERE public_id = $1`,
		model.SeedMerchantPublicID, url)
	if err != nil {
		t.Fatalf("set webhook url: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("set webhook url: rows=%d", tag.RowsAffected())
	}
}

func startWebhookWorker(t *testing.T) {
	t.Helper()
	requireDB(t)
	log := zerolog.Nop()
	repo := repository.NewPaymentRepository(testPool)
	proc, err := service.NewWebhookProcessor(repo, time.Second, 5*time.Second, &log, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := worker.NewWebhookWorker(proc, 2, 10*time.Millisecond, &log)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() {
		cancel()
		w.Wait()
	})
}

func TestWebhookAuthorizedAndCaptured(t *testing.T) {
	resetDB(t)
	const secret = "dev-webhook-secret"
	cap := &webhookCapture{}
	srv := httptest.NewServer(cap.handler(secret))
	defer srv.Close()
	setMerchantWebhookURL(t, srv.URL)

	gw, err := bank.NewSimulator(bank.SimulatorConfig{SuccessPct: 100})
	if err != nil {
		t.Fatal(err)
	}
	startAuthorizeWorker(t, gw, fastRetry())
	startWebhookWorker(t)
	r := testRouter(t)

	rec := postPayment(t, r, seedMerchantID(), randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	paymentID, _ := decodePayment(t, rec)["id"].(string)
	waitPaymentStatus(t, r, paymentID, "AUTHORIZED")

	authEv := cap.waitForType(t, "payment.authorized", 3*time.Second)
	var authBody map[string]any
	if err := json.Unmarshal(authEv.Body, &authBody); err != nil {
		t.Fatal(err)
	}
	data, _ := authBody["data"].(map[string]any)
	if data["payment_id"] != paymentID || data["status"] != "AUTHORIZED" {
		t.Fatalf("authorized data: %v", data)
	}
	if authEv.EventID != authBody["id"] {
		t.Fatalf("event id header/body mismatch: %s vs %v", authEv.EventID, authBody["id"])
	}

	captureRec := postPaymentAction(t, r, seedMerchantID(), paymentID, "capture")
	if captureRec.Code != http.StatusOK {
		t.Fatalf("capture: %d %s", captureRec.Code, captureRec.Body.String())
	}

	capEv := cap.waitForType(t, "payment.captured", 3*time.Second)
	var capBody map[string]any
	if err := json.Unmarshal(capEv.Body, &capBody); err != nil {
		t.Fatal(err)
	}
	capData, _ := capBody["data"].(map[string]any)
	if capData["payment_id"] != paymentID || capData["status"] != "CAPTURED" {
		t.Fatalf("captured data: %v", capData)
	}
	if !webhook.VerifySignature(secret, capEv.Body, capEv.Signature) {
		t.Fatal("captured signature invalid")
	}
}

func TestWebhookRefunded(t *testing.T) {
	resetDB(t)
	const secret = "dev-webhook-secret"
	cap := &webhookCapture{}
	srv := httptest.NewServer(cap.handler(secret))
	defer srv.Close()
	setMerchantWebhookURL(t, srv.URL)

	gw, err := bank.NewSimulator(bank.SimulatorConfig{SuccessPct: 100})
	if err != nil {
		t.Fatal(err)
	}
	startAuthorizeWorker(t, gw, fastRetry())
	startWebhookWorker(t)
	r := testRouter(t)

	rec := postPayment(t, r, seedMerchantID(), randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	paymentID, _ := decodePayment(t, rec)["id"].(string)
	waitPaymentStatus(t, r, paymentID, "AUTHORIZED")
	cap.waitForType(t, "payment.authorized", 3*time.Second)

	captureRec := postPaymentAction(t, r, seedMerchantID(), paymentID, "capture")
	if captureRec.Code != http.StatusOK {
		t.Fatalf("capture: %d %s", captureRec.Code, captureRec.Body.String())
	}
	cap.waitForType(t, "payment.captured", 3*time.Second)

	refundRec := postRefund(t, r, seedMerchantID(), paymentID, randomKey(), refundJSON(5000))
	if refundRec.Code != http.StatusCreated {
		t.Fatalf("refund: %d %s", refundRec.Code, refundRec.Body.String())
	}

	refEv := cap.waitForType(t, "payment.refunded", 3*time.Second)
	var body map[string]any
	if err := json.Unmarshal(refEv.Body, &body); err != nil {
		t.Fatal(err)
	}
	data, _ := body["data"].(map[string]any)
	if data["payment_id"] != paymentID || data["status"] != "REFUNDED" {
		t.Fatalf("refunded data: %v", data)
	}
	if !webhook.VerifySignature(secret, refEv.Body, refEv.Signature) {
		t.Fatal("refunded signature invalid")
	}
}

func TestWebhookSkippedWithoutURL(t *testing.T) {
	resetDB(t)
	gw, err := bank.NewSimulator(bank.SimulatorConfig{SuccessPct: 100})
	if err != nil {
		t.Fatal(err)
	}
	startAuthorizeWorker(t, gw, fastRetry())
	startWebhookWorker(t)
	r := testRouter(t)

	rec := postPayment(t, r, seedMerchantID(), randomKey(), paymentJSON(1000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	id, _ := decodePayment(t, rec)["id"].(string)
	waitPaymentStatus(t, r, id, "AUTHORIZED")

	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM webhook_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected no webhook events, got %d", n)
	}
}
