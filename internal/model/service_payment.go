package model

import (
	"time"

	"github.com/google/uuid"
)

type CreatePaymentRequest struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type PaymentResponse struct {
	ID         string    `json:"id"`
	MerchantID string    `json:"merchant_id"`
	Amount     int64     `json:"amount"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func NewPaymentResponse(
	id, merchantID uuid.UUID,
	amount int64,
	currency string,
	status PaymentStatus,
	createdAt, updatedAt time.Time,
) PaymentResponse {
	return PaymentResponse{
		ID:         id.String(),
		MerchantID: merchantID.String(),
		Amount:     amount,
		Currency:   currency,
		Status:     status.String(),
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
}
