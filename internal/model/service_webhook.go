package model

// WebhookEventData is the "data" object
type WebhookEventData struct {
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
}
