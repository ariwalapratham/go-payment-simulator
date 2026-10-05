package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestAdminAuth(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.ErrorHandler(), middleware.AdminAuth("secret"))
	r.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing key: got %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderAdminKey, "wrong")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderAdminKey, "secret")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid key: got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderAdminKey, "sec")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("shorter key: got %d", rec.Code)
	}
}

func TestAdminAuthEmptyExpectedRejects(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.ErrorHandler(), middleware.AdminAuth(""))
	r.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderAdminKey, "")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("empty expected: got %d", rec.Code)
	}
}
