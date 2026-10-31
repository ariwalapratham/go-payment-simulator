package loadtest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/loadtest"
	"github.com/google/uuid"
)

func TestValidateRefundCallsSharedAndUnique(t *testing.T) {
	t.Parallel()

	sharedPay := uuid.New()
	uniquePay := uuid.New()
	refundID := uuid.New()
	calls := []loadtest.RefundCall{
		{PaymentID: sharedPay, SharedKey: true, Status: http.StatusCreated, RefundID: refundID},
		{PaymentID: sharedPay, SharedKey: true, Status: http.StatusOK, RefundID: refundID},
		{PaymentID: uniquePay, SharedKey: false, Status: http.StatusCreated, RefundID: uuid.New()},
		{PaymentID: uniquePay, SharedKey: false, Status: http.StatusUnprocessableEntity},
		{PaymentID: uniquePay, SharedKey: false, Status: http.StatusUnprocessableEntity},
	}
	if err := loadtest.ValidateRefundCalls(calls); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRefundCallsRejectsMixedSharedIDs(t *testing.T) {
	t.Parallel()

	pay := uuid.New()
	err := loadtest.ValidateRefundCalls([]loadtest.RefundCall{
		{PaymentID: pay, SharedKey: true, Status: http.StatusCreated, RefundID: uuid.New()},
		{PaymentID: pay, SharedKey: true, Status: http.StatusCreated, RefundID: uuid.New()},
	})
	if err == nil {
		t.Fatal("expected mixed id error")
	}
}

func TestRefundBurstAgainstHTTPTest(t *testing.T) {
	t.Parallel()

	srv := newRefundServer(t)
	t.Cleanup(srv.Close)

	api, err := loadtest.NewClient(srv.URL, "sk_test_x", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls, err := loadtest.RefundBurst(context.Background(), api, instantWait{}, loadtest.RefundConfig{
		Payments:   4,
		RefundsPer: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 16 {
		t.Fatalf("calls %d", len(calls))
	}
}

type instantWait struct{}

func (instantWait) WaitAuthorized(context.Context, []uuid.UUID) error { return nil }

type hangWait struct{}

func (hangWait) WaitAuthorized(ctx context.Context, _ []uuid.UUID) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestRefundBurstWaitTimeout(t *testing.T) {
	t.Parallel()

	srv := newRefundServer(t)
	t.Cleanup(srv.Close)

	api, err := loadtest.NewClient(srv.URL, "sk_test_x", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadtest.RefundBurst(context.Background(), api, hangWait{}, loadtest.RefundConfig{
		Payments:    2,
		RefundsPer:  2,
		WaitTimeout: 50 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline exceeded, got %v", err)
	}
}

type refundServer struct {
	mu       sync.Mutex
	payments int
	byKey    map[string]string
}

func newRefundServer(t *testing.T) *httptest.Server {
	t.Helper()
	st := &refundServer{byKey: make(map[string]string)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/payments/{id}/capture", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") == "" {
			http.Error(w, "missing key", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": r.PathValue("id"), "status": "CAPTURED"})
	})
	mux.HandleFunc("POST /v1/payments/{id}/refund", func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		pay := r.PathValue("id")
		st.mu.Lock()
		defer st.mu.Unlock()
		slot := pay + "/" + key
		if id, ok := st.byKey[slot]; ok {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "payment_id": pay})
			return
		}
		for existing := range st.byKey {
			if strings.HasPrefix(existing, pay+"/") {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": "refund_exceeds_balance"})
				return
			}
		}
		id := uuid.NewString()
		st.byKey[slot] = id
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "payment_id": pay})
	})
	mux.HandleFunc("POST /v1/payments", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		st.payments++
		st.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":     uuid.NewString(),
			"status": "PENDING",
		})
	})
	return httptest.NewServer(mux)
}
