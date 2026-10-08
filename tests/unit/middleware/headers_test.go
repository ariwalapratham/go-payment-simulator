package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestRequireIdempotencyKey(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.ErrorHandler(), middleware.RequireIdempotencyKey())
	r.POST("/x", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing key: got %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(middleware.HeaderIdempotencyKey, "k1")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid key: got %d", rec.Code)
	}
}
