package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// DBMerchant represents the merchants table in the database.
type DBMerchant struct {
	ID         int64     `db:"id"`
	PublicID   uuid.UUID `db:"public_id"`
	Name       string    `db:"name"`
	APIKeyHash string    `db:"api_key_hash"`
	CreatedAt  time.Time `db:"created_at"`
}

// DBPayment represents the payments table in the database.
type DBPayment struct {
	ID            int64         `db:"id"`
	PublicID      uuid.UUID     `db:"public_id"`
	MerchantID    int64         `db:"merchant_id"`
	Amount        int64         `db:"amount"`
	Currency      string        `db:"currency"`
	Status        PaymentStatus `db:"status"`
	AttemptCount  int           `db:"attempt_count"`
	NextAttemptAt time.Time     `db:"next_attempt_at"`
	LastError     *string       `db:"last_error"`
	CreatedAt     time.Time     `db:"created_at"`
	UpdatedAt     time.Time     `db:"updated_at"`
}

// DBRefund represents the refunds table in the database.
type DBRefund struct {
	ID        int64        `db:"id"`
	PublicID  uuid.UUID    `db:"public_id"`
	PaymentID int64        `db:"payment_id"`
	Amount    int64        `db:"amount"`
	Status    RefundStatus `db:"status"`
	CreatedAt time.Time    `db:"created_at"`
}

// DBIdempotencyKey represents the idempotency_keys table in the database.
type DBIdempotencyKey struct {
	ID             int64     `db:"id"`
	MerchantID     int64     `db:"merchant_id"`
	IdempotencyKey string    `db:"idempotency_key"`
	PaymentID      *int64    `db:"payment_id"`
	RequestHash    string    `db:"request_hash"`
	CreatedAt      time.Time `db:"created_at"`
}

// DBWebhookEvent represents the webhook_events table in the database.
type DBWebhookEvent struct {
	ID        int64            `db:"id"`
	PublicID  uuid.UUID        `db:"public_id"`
	PaymentID int64            `db:"payment_id"`
	Type      WebhookEventType `db:"type"`
	Payload   json.RawMessage  `db:"payload"`
	CreatedAt time.Time        `db:"created_at"`
}

// DBWebhookDelivery represents the webhook_deliveries table in the database.
type DBWebhookDelivery struct {
	ID             int64                 `db:"id"`
	WebhookEventID int64                 `db:"webhook_event_id"`
	Status         WebhookDeliveryStatus `db:"status"`
	AttemptCount   int                   `db:"attempt_count"`
	NextAttemptAt  time.Time             `db:"next_attempt_at"`
	LastError      *string               `db:"last_error"`
	CreatedAt      time.Time             `db:"created_at"`
	UpdatedAt      time.Time             `db:"updated_at"`
}

// TableName returns the table name for the DBMerchant struct.
func (DBMerchant) TableName() string { return TableMerchants }

// TableName returns the table name for the DBPayment struct.
func (DBPayment) TableName() string { return TablePayments }

// TableName returns the table name for the DBRefund struct.
func (DBRefund) TableName() string { return TableRefunds }

// TableName returns the table name for the DBIdempotencyKey struct.
func (DBIdempotencyKey) TableName() string { return TableIdempotencyKeys }

// TableName returns the table name for the DBWebhookEvent struct.
func (DBWebhookEvent) TableName() string { return TableWebhookEvents }

// TableName returns the table name for the DBWebhookDelivery struct.
func (DBWebhookDelivery) TableName() string { return TableWebhookDeliveries }
