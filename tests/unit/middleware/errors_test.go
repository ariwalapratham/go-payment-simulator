package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestErrorHandlerEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ErrorHandler())
	r.GET("/fail", func(c *gin.Context) {
		middleware.AbortWithError(c, errs.NewConflictError("invalid_state_transition", "cannot capture"))
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusConflict)
	}

	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	errObj := body["error"]
	if errObj["code"] != "invalid_state_transition" {
		t.Fatalf("code: %q", errObj["code"])
	}
	if errObj["request_id"] == "" {
		t.Fatal("missing request_id")
	}
}
