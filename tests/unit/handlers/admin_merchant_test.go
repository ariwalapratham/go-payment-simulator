package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
)

func TestAdminMerchantsRequiresKey(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/merchants", strings.NewReader(`{"name":"Acme"}`))
	req.Header.Set("Content-Type", "application/json")
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminMerchantsWrongKey(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/merchants", nil)
	req.Header.Set(middleware.HeaderAdminKey, "wrong")
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminCreateInvalidJSON(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/merchants", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.HeaderAdminKey, "test-admin-key")
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestMerchantMeRequiresAPIKey(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/merchant/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminGetInvalidMerchantID(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/merchants/not-a-uuid", nil)
	req.Header.Set(middleware.HeaderAdminKey, "test-admin-key")
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusBadRequest)
	}
}
