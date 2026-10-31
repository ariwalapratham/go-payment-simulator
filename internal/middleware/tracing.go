package middleware

import (
	"github.com/gin-gonic/gin"
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

// NewRelicMiddleware starts a transaction and puts it on the *request* context
// so pgx/nrpgx5 can create datastore segments. nrgin only stores the txn on
// gin.Context, which handlers never pass to the pool.
func (tm *TracingMiddleware) NewRelicMiddleware() gin.HandlerFunc {
	if tm.nrApp == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		txn := tm.nrApp.StartTransaction(c.Request.Method + " " + path)
		defer txn.End()
		txn.SetWebRequestHTTP(c.Request)
		c.Request = newrelic.RequestWithTransactionContext(c.Request, txn)
		c.Next()
	}
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
