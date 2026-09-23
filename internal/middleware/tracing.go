package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/integrations/nrgin"
	"github.com/newrelic/go-agent/v3/integrations/nrpkgerrors"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/ariwalapratham/go-payment-simulator/internal/server"
)

// TracingMiddleware wraps New Relic Gin instrumentation.
type TracingMiddleware struct {
	nrApp *newrelic.Application
}

func NewTracingMiddleware(_ *server.Server, nrApp *newrelic.Application) *TracingMiddleware {
	return &TracingMiddleware{nrApp: nrApp}
}

// NewRelicMiddleware returns the New Relic middleware for Gin.
// When New Relic is not initialized it returns a no-op.
func (tm *TracingMiddleware) NewRelicMiddleware() gin.HandlerFunc {
	if tm.nrApp == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return nrgin.Middleware(tm.nrApp)
}

// EnhanceTracing adds request attributes to the New Relic transaction
// and records handler errors with the stack-aware wrapper.
func (tm *TracingMiddleware) EnhanceTracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		txn := newrelic.FromContext(c.Request.Context())
		if txn == nil {
			c.Next()
			return
		}

		txn.AddAttribute("http.real_ip", c.ClientIP())
		txn.AddAttribute("http.user_agent", c.Request.UserAgent())
		if requestID := GetRequestID(c); requestID != "" {
			txn.AddAttribute("request.id", requestID)
		}

		c.Next()

		if err := c.Errors.Last(); err != nil {
			txn.NoticeError(nrpkgerrors.Wrap(err.Err))
		}
		txn.AddAttribute("http.status_code", c.Writer.Status())
	}
}
