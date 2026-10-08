package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRequireAPIKey(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	want := uuid.New()
	r := gin.New()
	r.Use(middleware.ErrorHandler(), middleware.RequireAPIKey(func(_ context.Context, key string) (uuid.UUID, error) {
		if key == "sk_test_ok" {
			return want, nil
		}
		return uuid.Nil, errors.New("nope")
	}))
	r.GET("/x", func(c *gin.Context) {
		if middleware.MerchantIDFrom(c) != want {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing key: got %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderAPIKey, "sk_test_bad")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown key: got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderAPIKey, "sk_test_ok")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid key: got %d", rec.Code)
	}
}

func TestRequireAPIKeyRejectsMerchantID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.ErrorHandler(), middleware.RequireAPIKey(func(_ context.Context, _ string) (uuid.UUID, error) {
		return uuid.New(), nil
	}))
	r.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderAPIKey, "sk_test_ok")
	req.Header.Set(middleware.HeaderMerchantID, uuid.NewString())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("merchant id header: got %d", rec.Code)
	}
}

func TestRequireAPIKeyNilLookup(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.ErrorHandler(), middleware.RequireAPIKey(nil))
	r.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderAPIKey, "sk_test_ok")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("nil lookup: got %d", rec.Code)
	}
}
