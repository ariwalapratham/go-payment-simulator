//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestAdminCreateMerchantSecretsOnce(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	rec := postAdminMerchant(t, r, `{"name":"Acme Corp","webhook_url":"https://acme.example.com/hooks"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := decodeObj(t, rec)
	id, _ := created["id"].(string)
	apiKey, _ := created["api_key"].(string)
	secret, _ := created["webhook_secret"].(string)
	if id == "" || !strings.HasPrefix(apiKey, "sk_test_") || secret == "" {
		t.Fatalf("create body %+v", created)
	}

	got := adminJSON(t, r, http.MethodGet, "/v1/admin/merchants/"+id, "")
	if got.Code != http.StatusOK {
		t.Fatalf("get: %d %s", got.Code, got.Body.String())
	}
	assertNoSecrets(t, decodeObj(t, got))

	list := adminJSON(t, r, http.MethodGet, "/v1/admin/merchants", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	var wrap map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	data, _ := wrap["data"].([]any)
	if len(data) < 1 {
		t.Fatalf("list data %+v", wrap)
	}
	row, _ := data[0].(map[string]any)
	assertNoSecrets(t, row)
}

func TestAdminPatchAndRotateKey(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	created := decodeObj(t, postAdminMerchant(t, r, `{"name":"Acme"}`))
	id, _ := created["id"].(string)
	oldKey, _ := created["api_key"].(string)

	patch := adminJSON(t, r, http.MethodPatch, "/v1/admin/merchants/"+id,
		`{"name":"Acme Corporation","webhook_url":"https://acme.example.com/v2"}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", patch.Code, patch.Body.String())
	}
	body := decodeObj(t, patch)
	assertNoSecrets(t, body)
	if body["name"] != "Acme Corporation" {
		t.Fatalf("name %+v", body)
	}

	rotated := adminJSON(t, r, http.MethodPost, "/v1/admin/merchants/"+id+"/rotate-key", "")
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rotated.Code, rotated.Body.String())
	}
	rot := decodeObj(t, rotated)
	newKey, _ := rot["api_key"].(string)
	if newKey == "" || newKey == oldKey {
		t.Fatalf("rotate body %+v", rot)
	}

	oldMe := merchantMe(t, r, http.MethodGet, oldKey, "")
	if oldMe.Code != http.StatusUnauthorized {
		t.Fatalf("old key: %d", oldMe.Code)
	}
	me := merchantMe(t, r, http.MethodGet, newKey, "")
	if me.Code != http.StatusOK {
		t.Fatalf("new key: %d %s", me.Code, me.Body.String())
	}
	assertNoSecrets(t, decodeObj(t, me))
}

func TestMerchantMePatchWebhookURL(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	created := decodeObj(t, postAdminMerchant(t, r, `{"name":"Acme"}`))
	apiKey, _ := created["api_key"].(string)

	rec := merchantMe(t, r, http.MethodPatch, apiKey, `{"webhook_url":"https://acme.example.com/me","name":"ignored"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch me: %d %s", rec.Code, rec.Body.String())
	}
	body := decodeObj(t, rec)
	assertNoSecrets(t, body)
	if body["webhook_url"] != "https://acme.example.com/me" {
		t.Fatalf("url %+v", body)
	}
	if body["name"] != "Acme" {
		t.Fatalf("name should stay Acme: %+v", body)
	}

	missing := merchantMe(t, r, http.MethodPatch, apiKey, `{"name":"nope"}`)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing webhook_url: %d %s", missing.Code, missing.Body.String())
	}
}

func TestAdminCreateRejectsInvalidWebhookURL(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	rec := postAdminMerchant(t, r, `{"name":"Acme","webhook_url":"not-a-url"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	private := postAdminMerchant(t, r, `{"name":"Acme","webhook_url":"http://127.0.0.1/hooks"}`)
	if private.Code != http.StatusBadRequest {
		t.Fatalf("private url: got %d want %d body=%s", private.Code, http.StatusBadRequest, private.Body.String())
	}
}

func postAdminMerchant(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	return adminJSON(t, r, http.MethodPost, "/v1/admin/merchants", body)
}

func adminJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(middleware.HeaderAdminKey, testAdminAPIKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func merchantMe(t *testing.T, r *gin.Engine, method, apiKey, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, "/v1/merchant/me", nil)
	} else {
		req = httptest.NewRequest(method, "/v1/merchant/me", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(middleware.HeaderAPIKey, apiKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeObj(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	return body
}

func assertNoSecrets(t *testing.T, body map[string]any) {
	t.Helper()
	if _, ok := body["api_key"]; ok {
		t.Fatalf("api_key leaked: %+v", body)
	}
	if _, ok := body["webhook_secret"]; ok {
		t.Fatalf("webhook_secret leaked: %+v", body)
	}
}
