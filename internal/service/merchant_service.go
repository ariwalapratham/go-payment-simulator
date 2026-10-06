package service

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/google/uuid"
)

const (
	maxMerchantNameBytes = 255
	maxWebhookURLBytes   = 2048
	secretInsertAttempts = 3
	apiKeyRotateAttempts = 3
)

var (
	// ErrInvalidMerchantName is a blank or whitespace-only merchant name.
	ErrInvalidMerchantName = errors.New("merchant name is required")
	// ErrMerchantNameTooLong is a name longer than maxMerchantNameBytes.
	ErrMerchantNameTooLong = errors.New("merchant name is too long")
	// ErrInvalidWebhookURL is a webhook_url that is not a public http(s) URL.
	ErrInvalidWebhookURL = errors.New("webhook_url must be a public http or https URL")
	// ErrWebhookURLTooLong is a webhook_url longer than maxWebhookURLBytes.
	ErrWebhookURLTooLong = errors.New("webhook_url is too long")
	// ErrNoMerchantUpdate is a PATCH with no recognized fields.
	ErrNoMerchantUpdate = errors.New("at least one field is required")
	// ErrMerchantKeyRotated means a concurrent rotate won the compare-and-set.
	ErrMerchantKeyRotated = errors.New("merchant api key was rotated concurrently")
)

// Merchant is the public merchant record. Secrets are never included.
type Merchant struct {
	PublicID   uuid.UUID
	Name       string
	WebhookURL *string
	CreatedAt  time.Time
}

// CreatedMerchant is returned once from Create; callers must persist APIKey and WebhookSecret.
type CreatedMerchant struct {
	Merchant
	APIKey        string
	WebhookSecret string
}

type merchantStore interface {
	Create(
		ctx context.Context,
		name, apiKeyHash string,
		webhookURL *string,
		webhookSecret string,
	) (*model.DBMerchant, error)
	GetByPublicID(ctx context.Context, publicID uuid.UUID) (*model.DBMerchant, error)
	GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.DBMerchant, error)
	List(ctx context.Context) ([]model.DBMerchant, error)
	Update(
		ctx context.Context,
		publicID uuid.UUID,
		name *string,
		webhookURLSet bool,
		webhookURL *string,
	) (*model.DBMerchant, error)
	RotateAPIKeyHash(ctx context.Context, publicID uuid.UUID, oldHash, newHash string) (*model.DBMerchant, error)
}

// MerchantService orchestrates merchant create/list/update/rotate; no HTTP types.
type MerchantService struct {
	store merchantStore
}

// NewMerchantService orchestrates merchant create/list/update/rotate; no HTTP types.
func NewMerchantService(store merchantStore) *MerchantService {
	return &MerchantService{store: store}
}

// Create inserts a merchant and returns plaintext api_key and webhook_secret once.
func (s *MerchantService) Create(ctx context.Context, name string, webhookURL *string) (CreatedMerchant, error) {
	name, err := normalizeMerchantName(name)
	if err != nil {
		return CreatedMerchant{}, err
	}
	hookURL, err := parseWebhookURL(webhookURL)
	if err != nil {
		return CreatedMerchant{}, err
	}

	urlPtr := optionalURL(hookURL)
	var lastErr error
	for range secretInsertAttempts {
		apiKey, apiKeyHash, webhookSecret, genErr := newMerchantSecrets()
		if genErr != nil {
			return CreatedMerchant{}, genErr
		}
		row, createErr := s.store.Create(ctx, name, apiKeyHash, urlPtr, webhookSecret)
		if errors.Is(createErr, repository.ErrDuplicateAPIKeyHash) {
			lastErr = createErr
			continue
		}
		if createErr != nil {
			return CreatedMerchant{}, mapMerchantErr(createErr)
		}
		return CreatedMerchant{
			Merchant:      toMerchant(row),
			APIKey:        apiKey,
			WebhookSecret: webhookSecret,
		}, nil
	}
	return CreatedMerchant{}, mapMerchantErr(lastErr)
}

// Get loads a merchant by public id. Secrets are never included.
func (s *MerchantService) Get(ctx context.Context, publicID uuid.UUID) (Merchant, error) {
	row, err := s.store.GetByPublicID(ctx, publicID)
	if err != nil {
		return Merchant{}, mapMerchantErr(err)
	}
	return toMerchant(row), nil
}

// List returns merchants without secrets (capped in the repository).
func (s *MerchantService) List(ctx context.Context) ([]Merchant, error) {
	rows, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Merchant, 0, len(rows))
	for i := range rows {
		out = append(out, toMerchant(&rows[i]))
	}
	return out, nil
}

// Update patches name and/or webhook_url. webhookURLSet false leaves the URL unchanged.
func (s *MerchantService) Update(
	ctx context.Context,
	publicID uuid.UUID,
	name *string,
	webhookURL *string,
	webhookURLSet bool,
) (Merchant, error) {
	if name == nil && !webhookURLSet {
		return Merchant{}, ErrNoMerchantUpdate
	}

	var normalizedName *string
	if name != nil {
		n, err := normalizeMerchantName(*name)
		if err != nil {
			return Merchant{}, err
		}
		normalizedName = &n
	}

	var normalizedURL *string
	if webhookURLSet {
		u, err := parseWebhookURL(webhookURL)
		if err != nil {
			return Merchant{}, err
		}
		normalizedURL = optionalURL(u)
	}

	row, err := s.store.Update(ctx, publicID, normalizedName, webhookURLSet, normalizedURL)
	if err != nil {
		return Merchant{}, mapMerchantErr(err)
	}
	return toMerchant(row), nil
}

// UpdateWebhookURL is the self-service PATCH: only webhook_url, including clear-to-null.
func (s *MerchantService) UpdateWebhookURL(
	ctx context.Context,
	publicID uuid.UUID,
	webhookURL *string,
) (Merchant, error) {
	return s.Update(ctx, publicID, nil, webhookURL, true)
}

// RotateAPIKey replaces api_key_hash via compare-and-set on the current hash.
func (s *MerchantService) RotateAPIKey(ctx context.Context, publicID uuid.UUID) (uuid.UUID, string, error) {
	for range apiKeyRotateAttempts {
		current, err := s.store.GetByPublicID(ctx, publicID)
		if err != nil {
			return uuid.Nil, "", mapMerchantErr(err)
		}
		apiKey, apiKeyHash, err := newAPIKey()
		if err != nil {
			return uuid.Nil, "", err
		}
		row, err := s.store.RotateAPIKeyHash(ctx, publicID, current.APIKeyHash, apiKeyHash)
		if errors.Is(err, repository.ErrDuplicateAPIKeyHash) || errors.Is(err, repository.ErrMerchantNotFound) {
			continue
		}
		if err != nil {
			return uuid.Nil, "", mapMerchantErr(err)
		}
		return row.PublicID, apiKey, nil
	}
	if _, err := s.store.GetByPublicID(ctx, publicID); err != nil {
		return uuid.Nil, "", mapMerchantErr(err)
	}
	return uuid.Nil, "", ErrMerchantKeyRotated
}

// PublicIDByAPIKey hashes the plaintext key and resolves the merchant. Used by API-key middleware.
func (s *MerchantService) PublicIDByAPIKey(ctx context.Context, apiKey string) (uuid.UUID, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return uuid.Nil, ErrMerchantNotFound
	}
	row, err := s.store.GetByAPIKeyHash(ctx, HashAPIKey(apiKey))
	if err != nil {
		return uuid.Nil, mapMerchantErr(err)
	}
	return row.PublicID, nil
}

func toMerchant(m *model.DBMerchant) Merchant {
	return Merchant{
		PublicID:   m.PublicID,
		Name:       m.Name,
		WebhookURL: m.WebhookURL,
		CreatedAt:  m.CreatedAt,
	}
}

func normalizeMerchantName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrInvalidMerchantName
	}
	if len(name) > maxMerchantNameBytes {
		return "", ErrMerchantNameTooLong
	}
	return name, nil
}

func parseWebhookURL(raw *string) (string, error) {
	if raw == nil {
		return "", nil
	}
	u := strings.TrimSpace(*raw)
	if u == "" {
		return "", nil
	}
	if len(u) > maxWebhookURLBytes {
		return "", ErrWebhookURLTooLong
	}
	parsed, err := url.ParseRequestURI(u)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ErrInvalidWebhookURL
	}
	if parsed.User != nil {
		return "", ErrInvalidWebhookURL
	}
	if webhookHostForbidden(parsed.Hostname()) {
		return "", ErrInvalidWebhookURL
	}
	return u, nil
}

func webhookHostForbidden(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	switch h {
	case "localhost", "localhost.localdomain", "metadata.google.internal":
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func optionalURL(u string) *string {
	if u == "" {
		return nil
	}
	return &u
}

func mapMerchantErr(err error) error {
	if errors.Is(err, repository.ErrMerchantNotFound) {
		return ErrMerchantNotFound
	}
	return err
}
