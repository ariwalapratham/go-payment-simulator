package handlers_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
)

func TestListPaymentsInvalidQuery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, query, code, field string
	}{
		{"bad status", "?status=UNKNOWN", "INVALID_STATUS", "status"},
		{"limit text", "?limit=abc", "INVALID_LIMIT", "limit"},
		{"limit negative", "?limit=-1", "INVALID_LIMIT", "limit"},
		{"cursor not base64", "?cursor=not-a-cursor", "INVALID_CURSOR", "cursor"},
		{"cursor not an id", "?cursor=" + base64.StdEncoding.EncodeToString([]byte("nope")), "INVALID_CURSOR", "cursor"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/payments"+tc.query, nil)
			req.Header.Set(middleware.HeaderAPIKey, unitAPIKey)
			testRouter().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
			}
			assertErrorField(t, rec, tc.code, tc.field)
		})
	}
}

func TestListPaymentsValidQueryReachesHandler(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/payments?limit=0", nil)
	req.Header.Set(middleware.HeaderAPIKey, unitAPIKey)
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
}
