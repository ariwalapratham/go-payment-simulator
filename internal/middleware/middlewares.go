package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/ariwalapratham/go-payment-simulator/internal/server"
)

// Middlewares groups the gin middleware factories the router composes.
type Middlewares struct {
	RequestID gin.HandlerFunc
	Logger    gin.HandlerFunc
	Tracing   *TracingMiddleware
	RateLimit *RateLimitMiddleware
}

// NewMiddlewares builds the shared middleware set from the server.
// Request logging uses s.Logger; New Relic is optional (nil-safe).
func NewMiddlewares(s *server.Server) *Middlewares {
	var nrApp *newrelic.Application
	if s.LoggerService != nil {
		nrApp = s.LoggerService.GetApplication()
	}

	return &Middlewares{
		RequestID: RequestID(),
		Logger:    Logger(s.Logger),
		Tracing:   NewTracingMiddleware(s, nrApp),
		RateLimit: NewRateLimitMiddleware(s),
	}
}
