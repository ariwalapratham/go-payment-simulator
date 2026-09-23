package middleware

import (
	"github.com/ariwalapratham/go-payment-simulator/internal/server"
)

// RateLimitMiddleware records rate-limit hits for observability.
// Enforcement (token bucket / Redis) can be layered on later; for now
// this is the New Relic custom-event hook the limiter will call.
type RateLimitMiddleware struct {
	server *server.Server
}

func NewRateLimitMiddleware(s *server.Server) *RateLimitMiddleware {
	return &RateLimitMiddleware{server: s}
}

// RecordRateLimitHit emits a New Relic custom event when a request is limited.
func (r *RateLimitMiddleware) RecordRateLimitHit(endpoint string) {
	if r.server.LoggerService == nil {
		return
	}
	app := r.server.LoggerService.GetApplication()
	if app == nil {
		return
	}
	app.RecordCustomEvent("RateLimitHit", map[string]interface{}{
		"endpoint": endpoint,
	})
}
