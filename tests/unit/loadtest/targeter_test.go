package loadtest_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/loadtest"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestPaymentTargeterUniqueKeys(t *testing.T) {
	t.Parallel()

	tr, err := loadtest.PaymentTargeter(loadtest.TargetConfig{
		BaseURLs: []string{"http://localhost:8080"},
		APIKey:   "sk_test_x",
		Mode:     loadtest.KeyUnique,
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, 32)
	body := []byte(loadtest.PaymentJSON)
	for range 32 {
		var tgt vegeta.Target
		if tgtErr := tr(&tgt); tgtErr != nil {
			t.Fatal(tgtErr)
		}
		if tgt.Method != http.MethodPost || tgt.URL != "http://localhost:8080/v1/payments" {
			t.Fatalf("target %+v", tgt)
		}
		if string(tgt.Body) != string(body) {
			t.Fatalf("body %s", tgt.Body)
		}
		key := tgt.Header.Get("Idempotency-Key")
		if key == "" {
			t.Fatal("missing idempotency key")
		}
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate unique key %s", key)
		}
		seen[key] = struct{}{}
		if tgt.Header.Get("X-Api-Key") != "sk_test_x" {
			t.Fatal("missing api key")
		}
	}
}

func TestPaymentTargeterStormKeysAndBody(t *testing.T) {
	t.Parallel()

	tr, err := loadtest.PaymentTargeter(loadtest.TargetConfig{
		BaseURLs:    []string{"http://localhost:8080/"},
		APIKey:      "sk_test_x",
		Mode:        loadtest.KeyStorm,
		StormKeys:   4,
		StormPrefix: "storm-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	for range 20 {
		var tgt vegeta.Target
		if tgtErr := tr(&tgt); tgtErr != nil {
			t.Fatal(tgtErr)
		}
		if string(tgt.Body) != loadtest.PaymentJSON {
			t.Fatalf("storm body must be identical, got %s", tgt.Body)
		}
		key := tgt.Header.Get("Idempotency-Key")
		if !strings.HasPrefix(key, "storm-run-") {
			t.Fatalf("key %s", key)
		}
		counts[key]++
	}
	if len(counts) != 4 {
		t.Fatalf("want 4 keys, got %d %v", len(counts), counts)
	}
	for _, n := range counts {
		if n != 5 {
			t.Fatalf("uneven storm distribution %v", counts)
		}
	}
}

func TestPaymentTargeterRoundRobin(t *testing.T) {
	t.Parallel()

	tr, err := loadtest.PaymentTargeter(loadtest.TargetConfig{
		BaseURLs: []string{"http://localhost:8080", " http://localhost:8081/ "},
		APIKey:   "sk_test_x",
		Mode:     loadtest.KeyUnique,
	})
	if err != nil {
		t.Fatal(err)
	}
	var a, b int
	for range 10 {
		var tgt vegeta.Target
		if tgtErr := tr(&tgt); tgtErr != nil {
			t.Fatal(tgtErr)
		}
		switch tgt.URL {
		case "http://localhost:8080/v1/payments":
			a++
		case "http://localhost:8081/v1/payments":
			b++
		default:
			t.Fatalf("url %s", tgt.URL)
		}
	}
	if a != 5 || b != 5 {
		t.Fatalf("round robin a=%d b=%d", a, b)
	}
}

func TestPaymentTargeterRejectsEmpty(t *testing.T) {
	t.Parallel()

	if _, err := loadtest.PaymentTargeter(loadtest.TargetConfig{APIKey: "k"}); err == nil {
		t.Fatal("expected error for missing urls")
	}
	if _, err := loadtest.PaymentTargeter(loadtest.TargetConfig{BaseURLs: []string{"http://localhost:8080"}}); err == nil {
		t.Fatal("expected error for missing api key")
	}
	if _, err := loadtest.PaymentTargeter(loadtest.TargetConfig{
		BaseURLs:  []string{"http://localhost:8080"},
		APIKey:    "k",
		Mode:      loadtest.KeyStorm,
		StormKeys: 0,
	}); err == nil {
		t.Fatal("expected error for storm keys")
	}
}

func TestParseBaseURLs(t *testing.T) {
	t.Parallel()

	got := loadtest.ParseBaseURLs("http://localhost:8080, http://localhost:8081/")
	if len(got) != 2 || got[0] != "http://localhost:8080" || got[1] != "http://localhost:8081" {
		t.Fatalf("%v", got)
	}
}
