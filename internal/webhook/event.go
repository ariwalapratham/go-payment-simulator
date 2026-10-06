package webhook

import (
	"encoding/json"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/google/uuid"
)

// OutboundEvent is the JSON body POSTed to the merchant (api-db-new-changes §3.11).
type OutboundEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	CreatedAt time.Time              `json:"created_at"`
	Data      model.WebhookEventData `json:"data"`
}

// DataPayload builds the JSONB "data" stored on webhook_events.
func DataPayload(paymentPublicID uuid.UUID, status model.PaymentStatus, amount int64, currency string) (json.RawMessage, error) {
	d := model.WebhookEventData{
		PaymentID: paymentPublicID.String(),
		Status:    status.String(),
		Amount:    amount,
		Currency:  currency,
	}
	return json.Marshal(d)
}

// MarshalOutbound builds the signed POST body from a stored event row.
func MarshalOutbound(publicID uuid.UUID, eventType model.WebhookEventType, createdAt time.Time, payload json.RawMessage) ([]byte, error) {
	var data model.WebhookEventData
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	ev := OutboundEvent{
		ID:        publicID.String(),
		Type:      eventType.String(),
		CreatedAt: createdAt.UTC().Truncate(time.Second),
		Data:      data,
	}
	return json.Marshal(ev)
}
