//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
)

func TestCreatePayment_ConcurrentSameKey(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	key := randomKey()
	body := paymentJSON(5000, "USD")
	const n = 50

	type result struct {
		code int
		raw  []byte
	}
	results := make([]result, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			rec := postPayment(t, r, seedMerchantID(), key, body)
			results[i] = result{code: rec.Code, raw: rec.Body.Bytes()}
		}(i)
	}
	wg.Wait()

	if countPayments(t) != 1 {
		t.Fatalf("payment rows: %d want 1", countPayments(t))
	}

	var winner string
	for _, res := range results {
		if res.code != http.StatusCreated && res.code != http.StatusOK {
			t.Fatalf("unexpected status %d body=%s", res.code, res.raw)
		}
		var payload map[string]any
		if err := json.Unmarshal(res.raw, &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		id, _ := payload["id"].(string)
		if id == "" {
			t.Fatal("missing payment id")
		}
		if winner == "" {
			winner = id
		}
		if id != winner {
			t.Fatalf("mixed ids: %s vs %s", winner, id)
		}
	}
}

func TestCapture_ConcurrentOnce(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := createPayment(t, r)
	forcePaymentStatus(t, id, model.PaymentStatusAuthorized)
	const n = 20

	codes := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			rec := postPaymentAction(t, r, seedMerchantID(), id, "capture")
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()

	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok != 1 || conflict != n-1 {
		t.Fatalf("ok=%d conflict=%d want 1/%d", ok, conflict, n-1)
	}
	got := getPayment(t, r, seedMerchantID(), id)
	if decodePayment(t, got)["status"] != "CAPTURED" {
		t.Fatalf("status %v", decodePayment(t, got)["status"])
	}
}
