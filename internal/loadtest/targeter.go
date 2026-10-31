package loadtest

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

// KeyMode selects how Idempotency-Key is generated for each create.
type KeyMode int

const (
	// KeyUnique issues a new UUID per request.
	KeyUnique KeyMode = iota
	// KeyStorm cycles a small fixed set of keys so many requests share a claim.
	KeyStorm
)

// TargetConfig is the Vegeta create targeter.
type TargetConfig struct {
	BaseURLs    []string
	APIKey      string
	Mode        KeyMode
	StormKeys   int
	StormPrefix string
}

// PaymentTargeter builds POST /v1/payments targets. It is safe for concurrent use.
// Each call allocates a new Header; Body is the shared PaymentJSON slice (read-only).
func PaymentTargeter(cfg TargetConfig) (vegeta.Targeter, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	body := []byte(PaymentJSON)
	urls := normalizeURLs(cfg.BaseURLs)
	keys := cfg.StormKeys
	prefix := cfg.StormPrefix
	if prefix == "" {
		prefix = uuid.NewString()
	}
	var seq atomic.Uint64
	return func(t *vegeta.Target) error {
		i := seq.Add(1) - 1
		t.Method = http.MethodPost
		t.URL = urls[i%uint64(len(urls))] + createPath
		t.Body = body
		t.Header = newHeader(cfg.APIKey, idempotencyKey(cfg.Mode, prefix, keys, i))
		return nil
	}, nil
}

func (c TargetConfig) validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("api key is required")
	}
	if len(normalizeURLs(c.BaseURLs)) == 0 {
		return errors.New("at least one base url is required")
	}
	if c.Mode == KeyStorm && c.StormKeys < 1 {
		return errors.New("storm keys must be >= 1")
	}
	return nil
}

func idempotencyKey(mode KeyMode, prefix string, stormKeys int, i uint64) string {
	if mode != KeyStorm {
		return uuid.NewString()
	}
	n := uint64(stormKeys) //nolint:gosec // G115: storm key count is a small CLI flag
	return fmt.Sprintf("%s-%d", prefix, i%n)
}

func normalizeURLs(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, u := range raw {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		if u == "" {
			continue
		}
		out = append(out, u)
	}
	return out
}

// ParseBaseURLs splits a comma-separated list of API origins.
func ParseBaseURLs(csv string) []string {
	parts := strings.Split(csv, ",")
	return normalizeURLs(parts)
}
