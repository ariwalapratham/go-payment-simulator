//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/gin-gonic/gin"
)

func TestListPayments_Empty(t *testing.T) {
	resetDB(t)
	r := testRouter(t)

	rec := listPayments(t, r, seedAPIKey(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	data, next := decodePaymentList(t, rec)
	if len(data) != 0 {
		t.Fatalf("data: %+v", data)
	}
	if next != nil {
		t.Fatalf("next_cursor: %v", *next)
	}
}

func TestListPayments_ScopedAndFiltered(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	ownerKey := seedAPIKey()
	_, otherKey := createMerchant(t, "other-merchant")

	pendingID := createPayment(t, r)
	capturedID := createPayment(t, r)
	forcePaymentStatus(t, capturedID, model.PaymentStatusAuthorized)
	if rec := postPaymentAction(t, r, ownerKey, capturedID, "capture"); rec.Code != http.StatusOK {
		t.Fatalf("capture: %d %s", rec.Code, rec.Body.String())
	}
	other := postPayment(t, r, otherKey, randomKey(), paymentJSON(1000, "USD"))
	if other.Code != http.StatusCreated {
		t.Fatalf("other create: %d %s", other.Code, other.Body.String())
	}

	all := listPayments(t, r, ownerKey, "")
	if all.Code != http.StatusOK {
		t.Fatalf("list: %d %s", all.Code, all.Body.String())
	}
	allData, _ := decodePaymentList(t, all)
	ids := listIDs(allData)
	if !containsID(ids, pendingID) || !containsID(ids, capturedID) {
		t.Fatalf("owner ids: %v", ids)
	}
	otherID, _ := decodePayment(t, other)["id"].(string)
	if containsID(ids, otherID) {
		t.Fatalf("leaked other payment: %v", ids)
	}

	filtered := listPayments(t, r, ownerKey, "?status=CAPTURED")
	if filtered.Code != http.StatusOK {
		t.Fatalf("filter: %d %s", filtered.Code, filtered.Body.String())
	}
	got, next := decodePaymentList(t, filtered)
	if len(got) != 1 || got[0]["id"] != capturedID {
		t.Fatalf("filtered: %+v", got)
	}
	if next != nil {
		t.Fatalf("next_cursor: %v", *next)
	}
}

func TestListPayments_CursorPages(t *testing.T) {
	resetDB(t)
	r := testRouter(t)
	key := seedAPIKey()

	var created []string
	for i := 0; i < 5; i++ {
		created = append(created, createPayment(t, r))
	}

	seen := make(map[string]struct{})
	cursor := ""
	pages := 0
	for {
		q := "?limit=2"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		rec := listPayments(t, r, key, q)
		if rec.Code != http.StatusOK {
			t.Fatalf("page: %d %s", rec.Code, rec.Body.String())
		}
		data, next := decodePaymentList(t, rec)
		pages++
		if pages > 3 {
			t.Fatal("too many pages")
		}
		for _, row := range data {
			id, _ := row["id"].(string)
			if _, dup := seen[id]; dup {
				t.Fatalf("overlap id %s", id)
			}
			seen[id] = struct{}{}
		}
		if next == nil {
			if pages != 3 || len(data) != 1 {
				t.Fatalf("last page pages=%d n=%d", pages, len(data))
			}
			break
		}
		if len(data) != 2 {
			t.Fatalf("page size %d", len(data))
		}
		cursor = *next
	}
	if len(seen) != 5 {
		t.Fatalf("seen %d want 5 created=%v", len(seen), created)
	}
}

func listPayments(t *testing.T, r *gin.Engine, apiKey, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/payments"+rawQuery, nil)
	req.Header.Set(middleware.HeaderAPIKey, apiKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodePaymentList(t *testing.T, rec *httptest.ResponseRecorder) (data []map[string]any, next *string) {
	t.Helper()
	var body struct {
		Data       []map[string]any `json:"data"`
		NextCursor *string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if body.Data == nil {
		t.Fatal("data is null")
	}
	return body.Data, body.NextCursor
}

func listIDs(data []map[string]any) []string {
	ids := make([]string, 0, len(data))
	for _, row := range data {
		id, _ := row["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
