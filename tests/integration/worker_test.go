//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/ariwalapratham/go-payment-simulator/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func startAuthorizeWorker(t *testing.T, gw bank.Gateway, retry service.RetryConfig) {
	t.Helper()
	requireDB(t)
	log := zerolog.Nop()
	repo := repository.NewPaymentRepository(testPool)
	proc, err := service.NewPaymentProcessor(
		repo, gw, retry, time.Second, 5*time.Second, &log, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	w := worker.NewPaymentWorker(proc, 2, 15*time.Millisecond, &log)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() {
		cancel()
		w.Wait()
	})
}

func waitPaymentStatus(t *testing.T, r *gin.Engine, paymentID, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		rec := getPayment(t, r, seedMerchantID(), paymentID)
		if rec.Code == http.StatusOK {
			body := decodePayment(t, rec)
			last, _ = body["status"].(string)
			if last == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("status=%s want %s", last, want)
}

func paymentAttemptCount(t *testing.T, publicID string) int {
	t.Helper()
	var n int
	err := testPool.QueryRow(context.Background(),
		`SELECT attempt_count FROM payments WHERE public_id = $1`, publicID).Scan(&n)
	if err != nil {
		t.Fatalf("attempt_count: %v", err)
	}
	return n
}

func fastRetry() service.RetryConfig {
	return service.RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Jitter:      0,
	}
}

func TestWorkerAuthorizesPayment(t *testing.T) {
	resetDB(t)
	gw, err := bank.NewSimulator(bank.SimulatorConfig{SuccessPct: 100})
	if err != nil {
		t.Fatal(err)
	}
	startAuthorizeWorker(t, gw, fastRetry())
	r := testRouter(t)

	rec := postPayment(t, r, seedMerchantID(), randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id, _ := decodePayment(t, rec)["id"].(string)
	waitPaymentStatus(t, r, id, "AUTHORIZED")
}

func TestWorkerDeclinesPayment(t *testing.T) {
	resetDB(t)
	gw, err := bank.NewSimulator(bank.SimulatorConfig{DeclinedPct: 100})
	if err != nil {
		t.Fatal(err)
	}
	startAuthorizeWorker(t, gw, fastRetry())
	r := testRouter(t)

	rec := postPayment(t, r, seedMerchantID(), randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id, _ := decodePayment(t, rec)["id"].(string)
	waitPaymentStatus(t, r, id, "FAILED")
	if n := paymentAttemptCount(t, id); n != 1 {
		t.Fatalf("attempt_count=%d", n)
	}
}

func TestWorkerRetriesTimeoutThenSucceeds(t *testing.T) {
	resetDB(t)
	gw, err := bank.NewDeterministicSimulator(bank.OutcomeTimeout, bank.OutcomeSuccess)
	if err != nil {
		t.Fatal(err)
	}
	startAuthorizeWorker(t, gw, fastRetry())
	r := testRouter(t)

	rec := postPayment(t, r, seedMerchantID(), randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id, _ := decodePayment(t, rec)["id"].(string)
	waitPaymentStatus(t, r, id, "AUTHORIZED")
	if n := paymentAttemptCount(t, id); n != 2 {
		t.Fatalf("attempt_count=%d", n)
	}
}
