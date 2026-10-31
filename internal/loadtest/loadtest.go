// Package loadtest drives the payment API for local capacity and invariant checks.
// It does not start the API or change bank settings; the caller supplies an API key
// for a merchant without webhook_url.
package loadtest

import (
	"net/http"
	"time"
)

const (
	// PaymentJSON is the create body. Storm runs reuse it so the request hash matches.
	PaymentJSON = `{"amount":1000,"currency":"USD"}`
	// RefundJSON is a full-amount refund of PaymentJSON.
	RefundJSON = `{"amount":1000}`
	// PaymentAmount is the minor-unit amount in PaymentJSON and RefundJSON.
	PaymentAmount int64 = 1000

	headerAPIKey         = "X-Api-Key" //nolint:gosec // G101: HTTP header name, not a secret
	headerIdempotencyKey = "Idempotency-Key"

	createPath  = "/v1/payments"
	capturePath = "/v1/payments/%s/capture"
	refundPath  = "/v1/payments/%s/refund"

	defaultClientTimeout = 15 * time.Second
	defaultDrainPoll     = 200 * time.Millisecond
	minIdleConns         = 64
	idleConnMultiplier   = 4
	minWorkers           = 10
	maxInitialWorkers    = 100
	maxWorkerMultiplier  = 2
	minMaxWorkers        = 256
	stuckPendingGrace    = 60 * time.Second
	defaultMaxAttempts   = 5
	defaultStoreConns    = 4
	defaultDBPoolLabel   = 25
	defaultWorkerLabel   = 4
	defaultDrainTimeout  = 10 * time.Minute
	drainSlack           = time.Minute
	storePingTimeout     = 5 * time.Second
	max5xxPercent        = 1
)

func newHeader(apiKey, idempotencyKey string) http.Header {
	return http.Header{
		"Content-Type":       []string{"application/json"},
		headerAPIKey:         []string{apiKey},
		headerIdempotencyKey: []string{idempotencyKey},
	}
}
