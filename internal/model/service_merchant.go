package model

import "time"

// CreateMerchantRequest is POST /v1/admin/merchants. webhook_url is optional.
type CreateMerchantRequest struct {
	Name       string  `json:"name"`
	WebhookURL *string `json:"webhook_url"`
}

// UpdateMerchantRequest is PATCH /v1/admin/merchants/:id. All fields optional; at least one required.
type UpdateMerchantRequest struct {
	Name       *string `json:"name"`
	WebhookURL *string `json:"webhook_url"`
}

// UpdateMeRequest is PATCH /v1/merchant/me. Only webhook_url is applied.
type UpdateMeRequest struct {
	WebhookURL *string `json:"webhook_url"`
}

// MerchantResponse is a merchant without secrets (list, get, me, admin patch).
type MerchantResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	WebhookURL *string   `json:"webhook_url"`
	CreatedAt  time.Time `json:"created_at"`
}

// MerchantCreatedResponse includes plaintext api_key and webhook_secret once at create.
type MerchantCreatedResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	WebhookURL    *string   `json:"webhook_url"`
	APIKey        string    `json:"api_key"`
	WebhookSecret string    `json:"webhook_secret"`
	CreatedAt     time.Time `json:"created_at"`
}

// MerchantListResponse is GET /v1/admin/merchants.
type MerchantListResponse struct {
	Data []MerchantResponse `json:"data"`
}

// RotateMerchantKeyResponse includes the new plaintext api_key once.
type RotateMerchantKeyResponse struct {
	ID     string `json:"id"`
	APIKey string `json:"api_key"`
}

// NewMerchantResponse maps a merchant to the public JSON shape (no secrets).
func NewMerchantResponse(id, name string, webhookURL *string, createdAt time.Time) MerchantResponse {
	return MerchantResponse{
		ID:         id,
		Name:       name,
		WebhookURL: webhookURL,
		CreatedAt:  createdAt,
	}
}
