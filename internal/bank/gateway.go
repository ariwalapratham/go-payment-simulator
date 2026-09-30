package bank

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Outcome is the acquirer's decision for one authorize attempt.
type Outcome string

const (
	OutcomeSuccess          Outcome = "SUCCESS"
	OutcomeDeclined         Outcome = "DECLINED"
	OutcomeTimeout          Outcome = "TIMEOUT"
	OutcomeTemporaryFailure Outcome = "TEMPORARY_FAILURE"
)

func (o Outcome) String() string { return string(o) }

// Valid reports whether o is a known acquirer outcome.
func (o Outcome) Valid() bool {
	switch o {
	case OutcomeSuccess, OutcomeDeclined, OutcomeTimeout, OutcomeTemporaryFailure:
		return true
	default:
		return false
	}
}

// ParseOutcome maps a stored or configured string to Outcome.
func ParseOutcome(v string) (Outcome, error) {
	o := Outcome(v)
	if !o.Valid() {
		return "", fmt.Errorf("invalid bank outcome: %q", v)
	}
	return o, nil
}

// Retryable is true for outcomes that may succeed on a later attempt.
// Declines are terminal; success is finished, not retried.
func (o Outcome) Retryable() bool {
	switch o {
	case OutcomeTimeout, OutcomeTemporaryFailure:
		return true
	case OutcomeSuccess, OutcomeDeclined:
		return false
	default:
		return false
	}
}

// AuthorizeRequest is the domain input for one authorize call.
type AuthorizeRequest struct {
	PaymentPublicID  uuid.UUID
	MerchantPublicID uuid.UUID
	Amount           int64
	Currency         string
}

// AuthorizeResult is the acquirer response. Outcome is always set when err is nil.
type AuthorizeResult struct {
	Outcome           Outcome
	ProviderReference string
	Message           string
}

// Gateway authorizes a payment with an external acquirer.
// A later HTTP adapter should implement this same method; add Capture when that API exists.
type Gateway interface {
	Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error)
}
