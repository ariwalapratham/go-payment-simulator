package loadtest_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/loadtest"
	"github.com/google/uuid"
)

func TestAttackHitsHTTPTestWithHeaders(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	var mu sync.Mutex
	keys := make(map[string]struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payments" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Api-Key") != "sk_test_x" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != loadtest.PaymentJSON {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		mu.Lock()
		keys[key] = struct{}{}
		mu.Unlock()
		hits.Add(1)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": uuid.NewString(), "status": "PENDING"})
	}))
	t.Cleanup(srv.Close)

	tr, err := loadtest.PaymentTargeter(loadtest.TargetConfig{
		BaseURLs: []string{srv.URL},
		APIKey:   "sk_test_x",
		Mode:     loadtest.KeyUnique,
	})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := loadtest.Attack(context.Background(), tr, loadtest.AttackConfig{
		Rate:     20,
		Duration: time.Second,
		Timeout:  2 * time.Second,
		Name:     "unit",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := hits.Load()
	if got < 10 {
		t.Fatalf("hits %d stats %+v", got, stats)
	}
	mu.Lock()
	nkeys := len(keys)
	mu.Unlock()
	if nkeys < 10 {
		t.Fatalf("unique keys %d", nkeys)
	}
	if stats.StatusCodes["201"] == 0 {
		t.Fatalf("status %v", stats.StatusCodes)
	}
	if httpErr := loadtest.CheckAttackHTTP(stats, loadtest.KeyUnique); httpErr != nil {
		t.Fatal(httpErr)
	}
}

func TestCheckAttackHTTP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		stats   loadtest.AttackStats
		mode    loadtest.KeyMode
		wantErr bool
	}{
		{
			name:    "unique 201s",
			stats:   loadtest.AttackStats{Requests: 100, StatusCodes: map[string]int{"201": 100}},
			mode:    loadtest.KeyUnique,
			wantErr: false,
		},
		{
			name:    "unique zero 201",
			stats:   loadtest.AttackStats{Requests: 100, StatusCodes: map[string]int{"401": 100}},
			mode:    loadtest.KeyUnique,
			wantErr: true,
		},
		{
			name:    "storm 200s",
			stats:   loadtest.AttackStats{Requests: 100, StatusCodes: map[string]int{"201": 20, "200": 80}},
			mode:    loadtest.KeyStorm,
			wantErr: false,
		},
		{
			name:    "high 5xx",
			stats:   loadtest.AttackStats{Requests: 100, StatusCodes: map[string]int{"201": 90, "500": 10}},
			mode:    loadtest.KeyUnique,
			wantErr: true,
		},
		{
			name:    "no requests",
			stats:   loadtest.AttackStats{},
			mode:    loadtest.KeyUnique,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := loadtest.CheckAttackHTTP(tc.stats, tc.mode)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}
