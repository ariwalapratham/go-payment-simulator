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
		rec := getPayment(t, r, seedAPIKey(), paymentID)
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

	rec := postPayment(t, r, seedAPIKey(), randomKey(), paymentJSON(5000, "USD"))
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

	rec := postPayment(t, r, seedAPIKey(), randomKey(), paymentJSON(5000, "USD"))
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

	rec := postPayment(t, r, seedAPIKey(), randomKey(), paymentJSON(5000, "USD"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id, _ := decodePayment(t, rec)["id"].(string)
	waitPaymentStatus(t, r, id, "AUTHORIZED")
	if n := paymentAttemptCount(t, id); n != 2 {
		t.Fatalf("attempt_count=%d", n)
	}
}

func waitAuthorizeClaimed(t *testing.T, publicID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		var next time.Time
		err := testPool.QueryRow(context.Background(),
			`SELECT status, next_attempt_at FROM payments WHERE public_id = $1`, publicID,
		).Scan(&status, &next)
		if err != nil {
			t.Fatalf("claim wait: %v", err)
		}
		if status != "PENDING" {
			t.Fatalf("status=%s before cancel window", status)
		}
		if next.After(time.Now().Add(2 * time.Second)) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("worker did not claim payment")
}

func TestWorkerCancelDuringAuthorizeWins(t *testing.T) {
	resetDB(t)
	gw, err := bank.NewSimulator(bank.SimulatorConfig{
		SuccessPct: 100,
		MinDelayMS: 400,
		MaxDelayMS: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	startAuthorizeWorker(t, gw, fastRetry())
	r := testRouter(t)

	id := createPayment(t, r)
	waitAuthorizeClaimed(t, id)

	rec := postPaymentAction(t, r, seedAPIKey(), id, "cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	if decodePayment(t, rec)["status"] != "CANCELLED" {
		t.Fatalf("status: %v", decodePayment(t, rec)["status"])
	}

	time.Sleep(700 * time.Millisecond)
	got := getPayment(t, r, seedAPIKey(), id)
	if decodePayment(t, got)["status"] != "CANCELLED" {
		t.Fatalf("worker overwrote cancel: %v", decodePayment(t, got)["status"])
	}
	if n := paymentAttemptCount(t, id); n != 0 {
		t.Fatalf("attempt_count=%d; authorize write should have been abandoned", n)
	}
}
