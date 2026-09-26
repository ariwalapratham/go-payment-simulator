package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/ariwalapratham/go-payment-simulator/internal/server"
)

type Middlewares struct {
	RequestID gin.HandlerFunc
	Tracing   *TracingMiddleware
	Recovery  gin.HandlerFunc
	Logger    gin.HandlerFunc
	Errors    gin.HandlerFunc
	RateLimit *RateLimitMiddleware
}

func NewMiddlewares(s *server.Server) *Middlewares {
	var nrApp *newrelic.Application
	if s.LoggerService != nil {
		nrApp = s.LoggerService.GetApplication()
	}

	return &Middlewares{
		RequestID: RequestID(),
		Tracing:   NewTracingMiddleware(s, nrApp),
		Recovery:  Recovery(s.Logger),
		Logger:    Logger(s.Logger),
		Errors:    ErrorHandler(),
		RateLimit: NewRateLimitMiddleware(s),
	}
}
