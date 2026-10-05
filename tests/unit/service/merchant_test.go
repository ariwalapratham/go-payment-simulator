package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/google/uuid"
)

type fakeMerchants struct {
	byID   map[uuid.UUID]*model.DBMerchant
	byHash map[string]*model.DBMerchant
}

func newFakeMerchants() *fakeMerchants {
	return &fakeMerchants{
		byID:   make(map[uuid.UUID]*model.DBMerchant),
		byHash: make(map[string]*model.DBMerchant),
	}
}

func (f *fakeMerchants) Create(
	_ context.Context,
	name, apiKeyHash string,
	webhookURL *string,
	webhookSecret string,
) (*model.DBMerchant, error) {
	if _, ok := f.byHash[apiKeyHash]; ok {
		return nil, repository.ErrDuplicateAPIKeyHash
	}
	m := &model.DBMerchant{
		ID:            int64(len(f.byID) + 1),
		PublicID:      uuid.New(),
		Name:          name,
		APIKeyHash:    apiKeyHash,
		WebhookURL:    webhookURL,
		WebhookSecret: webhookSecret,
		CreatedAt:     time.Now().UTC(),
	}
	f.byID[m.PublicID] = m
	f.byHash[apiKeyHash] = m
	return m, nil
}

func (f *fakeMerchants) GetByPublicID(_ context.Context, publicID uuid.UUID) (*model.DBMerchant, error) {
	m, ok := f.byID[publicID]
	if !ok {
		return nil, repository.ErrMerchantNotFound
	}
	return m, nil
}

func (f *fakeMerchants) GetByAPIKeyHash(_ context.Context, apiKeyHash string) (*model.DBMerchant, error) {
	m, ok := f.byHash[apiKeyHash]
	if !ok {
		return nil, repository.ErrMerchantNotFound
	}
	return m, nil
}

func (f *fakeMerchants) List(context.Context) ([]model.DBMerchant, error) {
	out := make([]model.DBMerchant, 0, len(f.byID))
	for _, m := range f.byID {
		out = append(out, *m)
	}
	return out, nil
}

func (f *fakeMerchants) Update(
	_ context.Context,
	publicID uuid.UUID,
	name *string,
	webhookURLSet bool,
	webhookURL *string,
) (*model.DBMerchant, error) {
	m, ok := f.byID[publicID]
	if !ok {
		return nil, repository.ErrMerchantNotFound
	}
	if name != nil {
		m.Name = *name
	}
	if webhookURLSet {
		m.WebhookURL = webhookURL
	}
	return m, nil
}

func (f *fakeMerchants) RotateAPIKeyHash(
	_ context.Context,
	publicID uuid.UUID,
	oldHash, newHash string,
) (*model.DBMerchant, error) {
	m, ok := f.byID[publicID]
	if !ok || m.APIKeyHash != oldHash {
		return nil, repository.ErrMerchantNotFound
	}
	if _, exists := f.byHash[newHash]; exists {
		return nil, repository.ErrDuplicateAPIKeyHash
	}
	delete(f.byHash, m.APIKeyHash)
	m.APIKeyHash = newHash
	f.byHash[newHash] = m
	return m, nil
}

func TestCreateMerchantReturnsSecretsOnceShape(t *testing.T) {
	t.Parallel()

	svc := service.NewMerchantService(newFakeMerchants())
	url := "https://acme.example.com/hooks"
	got, err := svc.Create(context.Background(), "Acme", &url)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Acme" {
		t.Fatalf("name %q", got.Name)
	}
	if got.WebhookURL == nil || *got.WebhookURL != url {
		t.Fatalf("webhook_url %+v", got.WebhookURL)
	}
	if !strings.HasPrefix(got.APIKey, "sk_test_") {
		t.Fatalf("api_key prefix %q", got.APIKey)
	}
	if len(got.WebhookSecret) != 64 {
		t.Fatalf("webhook_secret len %d", len(got.WebhookSecret))
	}
}

func TestCreateMerchantOptionalWebhookURL(t *testing.T) {
	t.Parallel()

	svc := service.NewMerchantService(newFakeMerchants())
	got, err := svc.Create(context.Background(), "Acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.WebhookURL != nil {
		t.Fatalf("wanted nil webhook_url, got %v", got.WebhookURL)
	}
}

func TestCreateMerchantRejectsBlankNameAndBadURL(t *testing.T) {
	t.Parallel()

	svc := service.NewMerchantService(newFakeMerchants())
	_, err := svc.Create(context.Background(), "  ", nil)
	if !errors.Is(err, service.ErrInvalidMerchantName) {
		t.Fatalf("blank name: %v", err)
	}
	bad := "ftp://nope"
	_, err = svc.Create(context.Background(), "Acme", &bad)
	if !errors.Is(err, service.ErrInvalidWebhookURL) {
		t.Fatalf("bad url: %v", err)
	}
}

func TestCreateMerchantRejectsPrivateWebhookHosts(t *testing.T) {
	t.Parallel()

	svc := service.NewMerchantService(newFakeMerchants())
	for _, raw := range []string{
		"http://127.0.0.1/hooks",
		"http://localhost/hooks",
		"http://192.168.0.1/hooks",
		"http://10.0.0.1/hooks",
		"http://169.254.169.254/latest",
		"http://[::1]/hooks",
	} {
		u := raw
		_, err := svc.Create(context.Background(), "Acme", &u)
		if !errors.Is(err, service.ErrInvalidWebhookURL) {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

func TestCreateMerchantRejectsOverlongNameAndURL(t *testing.T) {
	t.Parallel()

	svc := service.NewMerchantService(newFakeMerchants())
	_, err := svc.Create(context.Background(), strings.Repeat("a", 256), nil)
	if !errors.Is(err, service.ErrMerchantNameTooLong) {
		t.Fatalf("long name: %v", err)
	}
	longURL := "https://acme.example.com/" + strings.Repeat("x", 2048)
	_, err = svc.Create(context.Background(), "Acme", &longURL)
	if !errors.Is(err, service.ErrWebhookURLTooLong) {
		t.Fatalf("long url: %v", err)
	}
}

func TestPublicIDByAPIKeyAndRotate(t *testing.T) {
	t.Parallel()

	store := newFakeMerchants()
	svc := service.NewMerchantService(store)
	created, err := svc.Create(context.Background(), "Acme", nil)
	if err != nil {
		t.Fatal(err)
	}

	id, err := svc.PublicIDByAPIKey(context.Background(), created.APIKey)
	if err != nil {
		t.Fatal(err)
	}
	if id != created.PublicID {
		t.Fatalf("id %s want %s", id, created.PublicID)
	}

	_, newKey, err := svc.RotateAPIKey(context.Background(), created.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	_, lookupErr := svc.PublicIDByAPIKey(context.Background(), created.APIKey)
	if !errors.Is(lookupErr, service.ErrMerchantNotFound) {
		t.Fatalf("old key: %v", lookupErr)
	}
	id, err = svc.PublicIDByAPIKey(context.Background(), newKey)
	if err != nil {
		t.Fatal(err)
	}
	if id != created.PublicID {
		t.Fatalf("rotated id %s", id)
	}
}

func TestUpdateMerchantAndSelfServiceURL(t *testing.T) {
	t.Parallel()

	svc := service.NewMerchantService(newFakeMerchants())
	created, err := svc.Create(context.Background(), "Acme", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Update(context.Background(), created.PublicID, nil, nil, false)
	if !errors.Is(err, service.ErrNoMerchantUpdate) {
		t.Fatalf("empty patch: %v", err)
	}

	name := "Acme Corp"
	got, err := svc.Update(context.Background(), created.PublicID, &name, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name {
		t.Fatalf("name %q", got.Name)
	}

	next := "https://acme.example.com/v2"
	got, err = svc.UpdateWebhookURL(context.Background(), created.PublicID, &next)
	if err != nil {
		t.Fatal(err)
	}
	if got.WebhookURL == nil || *got.WebhookURL != next {
		t.Fatalf("url %+v", got.WebhookURL)
	}

	empty := ""
	got, err = svc.UpdateWebhookURL(context.Background(), created.PublicID, &empty)
	if err != nil {
		t.Fatal(err)
	}
	if got.WebhookURL != nil {
		t.Fatalf("cleared url %+v", got.WebhookURL)
	}
}

func TestGetMerchantNotFound(t *testing.T) {
	t.Parallel()

	svc := service.NewMerchantService(newFakeMerchants())
	_, err := svc.Get(context.Background(), uuid.New())
	if !errors.Is(err, service.ErrMerchantNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestHashAPIKeyStable(t *testing.T) {
	t.Parallel()

	a := service.HashAPIKey("sk_test_abc")
	b := service.HashAPIKey("sk_test_abc")
	if a != b || a == "" {
		t.Fatalf("hash %q %q", a, b)
	}
	if a == service.HashAPIKey("sk_test_other") {
		t.Fatal("different keys hashed equal")
	}
}
