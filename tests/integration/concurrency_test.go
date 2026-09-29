//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
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
