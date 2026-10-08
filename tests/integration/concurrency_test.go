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
			rec := postPayment(t, r, seedAPIKey(), key, body)
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
			rec := postPaymentAction(t, r, seedAPIKey(), id, "capture")
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
	got := getPayment(t, r, seedAPIKey(), id)
	if decodePayment(t, got)["status"] != "CAPTURED" {
		t.Fatalf("status %v", decodePayment(t, got)["status"])
	}
}

func TestRefund_ConcurrentDifferentKeysDoNotOverRefund(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)
	const n = 20

	codes := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			rec := postRefund(t, r, seedAPIKey(), id, randomKey(), refundJSON(3000))
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()

	if sum := succeededRefundSum(t, id); sum > 5000 {
		t.Fatalf("refunded %d exceeds captured 5000", sum)
	}
	if sum := succeededRefundSum(t, id); sum != 3000 {
		t.Fatalf("refunded %d want 3000", sum)
	}

	ok, unprocessable, conflict := 0, 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated, http.StatusOK:
			ok++
		case http.StatusUnprocessableEntity:
			unprocessable++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok != 1 {
		t.Fatalf("successes=%d unprocessable=%d conflict=%d", ok, unprocessable, conflict)
	}
}

func TestRefund_ConcurrentSameKeyOnce(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	id := capturedPayment(t, r)
	key := randomKey()
	const n = 20

	codes := make([]int, n)
	ids := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			rec := postRefund(t, r, seedAPIKey(), id, key, refundJSON(3000))
			codes[i] = rec.Code
			body := decodePayment(t, rec)
			ids[i], _ = body["id"].(string)
		}(i)
	}
	wg.Wait()

	if countRefunds(t) != 1 {
		t.Fatalf("refund rows: %d", countRefunds(t))
	}
	var winner string
	for i, c := range codes {
		if c != http.StatusCreated && c != http.StatusOK {
			t.Fatalf("unexpected status %d", c)
		}
		if ids[i] == "" {
			t.Fatal("missing refund id")
		}
		if winner == "" {
			winner = ids[i]
		}
		if ids[i] != winner {
			t.Fatalf("mixed ids: %s vs %s", winner, ids[i])
		}
	}
}
