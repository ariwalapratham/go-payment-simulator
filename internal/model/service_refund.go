package model

import (
	"time"

	"github.com/google/uuid"
)

type CreateRefundRequest struct {
	Amount int64 `json:"amount"`
}

type RefundResponse struct {
	ID        string    `json:"id"`
	PaymentID string    `json:"payment_id"`
	Amount    int64     `json:"amount"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func NewRefundResponse(
	id, paymentID uuid.UUID,
	amount int64,
	status RefundStatus,
	createdAt time.Time,
) RefundResponse {
	return RefundResponse{
		ID:        id.String(),
		PaymentID: paymentID.String(),
		Amount:    amount,
		Status:    status.String(),
		CreatedAt: createdAt,
	}
}
