package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/newrelic"
)

func TestNewRelicMiddlewarePutsTxnOnRequestContext(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	app, err := newrelic.NewApplication(
		newrelic.ConfigAppName("test"),
		newrelic.ConfigLicense("0000000000000000000000000000000000000000"),
		newrelic.ConfigEnabled(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	tm := middleware.NewTracingMiddleware(nil, app)

	var gotTxn bool
	r := gin.New()
	r.Use(tm.NewRelicMiddleware())
	r.GET("/x", func(c *gin.Context) {
		gotTxn = newrelic.FromContext(c.Request.Context()) != nil
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if !gotTxn {
		t.Fatal("request context missing New Relic transaction; nrpgx5 cannot emit DB spans")
	}
}
