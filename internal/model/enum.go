package model

import (
	"errors"
	"fmt"
)

var ErrInvalidTransition = errors.New("invalid state transition")

type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "PENDING"
	PaymentStatusAuthorized PaymentStatus = "AUTHORIZED"
	PaymentStatusCaptured   PaymentStatus = "CAPTURED"
	PaymentStatusFailed     PaymentStatus = "FAILED"
	PaymentStatusCancelled  PaymentStatus = "CANCELLED"
	PaymentStatusRefunded   PaymentStatus = "REFUNDED"
)

func (s PaymentStatus) String() string { return string(s) }

func (s PaymentStatus) Valid() bool {
	switch s {
	case PaymentStatusPending, PaymentStatusAuthorized, PaymentStatusCaptured,
		PaymentStatusFailed, PaymentStatusCancelled, PaymentStatusRefunded:
		return true
	default:
		return false
	}
}

func ParsePaymentStatus(v string) (PaymentStatus, error) {
	s := PaymentStatus(v)
	if !s.Valid() {
		return "", fmt.Errorf("invalid payment status: %q", v)
	}
	return s, nil
}

// AllowedPaymentTransitions matches the API state diagram.
var AllowedPaymentTransitions = map[PaymentStatus][]PaymentStatus{
	PaymentStatusPending:    {PaymentStatusAuthorized, PaymentStatusFailed, PaymentStatusCancelled},
	PaymentStatusAuthorized: {PaymentStatusCaptured, PaymentStatusCancelled},
	PaymentStatusCaptured:   {PaymentStatusRefunded},
	PaymentStatusFailed:     {},
	PaymentStatusCancelled:  {},
	PaymentStatusRefunded:   {},
}

func (s PaymentStatus) CanTransitionTo(next PaymentStatus) bool {
	for _, allowed := range AllowedPaymentTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// Transition is the payment status machine; invalid edges return ErrInvalidTransition.
func Transition(current, next PaymentStatus) error {
	if !current.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current, next)
	}
	return nil
}

type RefundStatus string

const (
	RefundStatusPending   RefundStatus = "PENDING"
	RefundStatusSucceeded RefundStatus = "SUCCEEDED"
	RefundStatusFailed    RefundStatus = "FAILED"
)

func (s RefundStatus) String() string { return string(s) }

func (s RefundStatus) Valid() bool {
	switch s {
	case RefundStatusPending, RefundStatusSucceeded, RefundStatusFailed:
		return true
	default:
		return false
	}
}

func ParseRefundStatus(v string) (RefundStatus, error) {
	s := RefundStatus(v)
	if !s.Valid() {
		return "", fmt.Errorf("invalid refund status: %q", v)
	}
	return s, nil
}

// IdempotencyScope partitions (merchant, key) uniqueness per operation.
type IdempotencyScope string

const (
	IdempotencyScopePaymentCreate IdempotencyScope = "payment.create"
	IdempotencyScopePaymentRefund IdempotencyScope = "payment.refund"
)

func (s IdempotencyScope) String() string { return string(s) }

type WebhookEventType string

const (
	WebhookEventPaymentAuthorized WebhookEventType = "payment.authorized"
	WebhookEventPaymentCaptured   WebhookEventType = "payment.captured"
	WebhookEventPaymentFailed     WebhookEventType = "payment.failed"
	WebhookEventPaymentRefunded   WebhookEventType = "payment.refunded"
)

func (t WebhookEventType) String() string { return string(t) }

func (t WebhookEventType) Valid() bool {
	switch t {
	case WebhookEventPaymentAuthorized, WebhookEventPaymentCaptured,
		WebhookEventPaymentFailed, WebhookEventPaymentRefunded:
		return true
	default:
		return false
	}
}

func ParseWebhookEventType(v string) (WebhookEventType, error) {
	t := WebhookEventType(v)
	if !t.Valid() {
		return "", fmt.Errorf("invalid webhook event type: %q", v)
	}
	return t, nil
}

type WebhookDeliveryStatus string

const (
	WebhookDeliveryStatusPending   WebhookDeliveryStatus = "PENDING"
	WebhookDeliveryStatusDelivered WebhookDeliveryStatus = "DELIVERED"
	WebhookDeliveryStatusFailed    WebhookDeliveryStatus = "FAILED"
)

func (s WebhookDeliveryStatus) String() string { return string(s) }

func (s WebhookDeliveryStatus) Valid() bool {
	switch s {
	case WebhookDeliveryStatusPending, WebhookDeliveryStatusDelivered, WebhookDeliveryStatusFailed:
		return true
	default:
		return false
	}
}

func ParseWebhookDeliveryStatus(v string) (WebhookDeliveryStatus, error) {
	s := WebhookDeliveryStatus(v)
	if !s.Valid() {
		return "", fmt.Errorf("invalid webhook delivery status: %q", v)
	}
	return s, nil
}
