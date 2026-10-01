package service

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
)

const jitterSpan = 2

// ErrUnknownOutcome means the acquirer returned a value the policy does not handle.
var ErrUnknownOutcome = errors.New("unknown bank outcome")

// RetryConfig is authorize retry/backoff. Jitter is a fraction in [0, 1]; 0 disables it.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      float64
}

// Decision is the payment row update after one bank attempt.
type Decision struct {
	NextStatus    model.PaymentStatus
	NextAttemptAt time.Time
	AttemptCount  int
	LastError     *string
}

// Validate checks retry bounds.
func (c RetryConfig) Validate() error {
	if c.MaxAttempts < 1 {
		return fmt.Errorf("retry max_attempts must be >= 1")
	}
	if c.BaseDelay <= 0 {
		return fmt.Errorf("retry base_delay must be > 0")
	}
	if c.MaxDelay < c.BaseDelay {
		return fmt.Errorf("retry max_delay must be >= base_delay")
	}
	if c.Jitter < 0 || c.Jitter > 1 {
		return fmt.Errorf("retry jitter must be in [0, 1]")
	}
	return nil
}

// DecideAuthorize maps a bank outcome onto the next payment state.
// attemptsBefore is payments.attempt_count before this Authorize call (0 for a new row).
func DecideAuthorize(
	outcome bank.Outcome,
	message string,
	attemptsBefore int,
	now time.Time,
	cfg RetryConfig,
) (Decision, error) {
	if attemptsBefore < 0 {
		return Decision{}, fmt.Errorf("attempts before must be >= 0")
	}
	if !outcome.Valid() {
		return Decision{}, fmt.Errorf("%w: %s", ErrUnknownOutcome, outcome)
	}

	nextAttempt := attemptsBefore + 1
	switch outcome {
	case bank.OutcomeSuccess:
		return Decision{
			NextStatus:    model.PaymentStatusAuthorized,
			NextAttemptAt: now,
			AttemptCount:  nextAttempt,
		}, nil
	case bank.OutcomeDeclined:
		return Decision{
			NextStatus:    model.PaymentStatusFailed,
			NextAttemptAt: now,
			AttemptCount:  nextAttempt,
			LastError:     lastErrorPtr(message, outcome),
		}, nil
	case bank.OutcomeTimeout, bank.OutcomeTemporaryFailure:
		d := Decision{
			NextStatus:   model.PaymentStatusFailed,
			AttemptCount: nextAttempt,
			LastError:    lastErrorPtr(message, outcome),
		}
		if nextAttempt < cfg.MaxAttempts {
			d.NextStatus = model.PaymentStatusPending
			d.NextAttemptAt = now.Add(backoff(attemptsBefore, cfg))
			return d, nil
		}
		d.NextAttemptAt = now
		return d, nil
	default:
		return Decision{}, fmt.Errorf("%w: %s", ErrUnknownOutcome, outcome)
	}
}

func lastErrorPtr(message string, outcome bank.Outcome) *string {
	if message == "" {
		message = outcome.String()
	}
	return &message
}

func backoff(attemptsBefore int, cfg RetryConfig) time.Duration {
	delay := cfg.BaseDelay
	for range attemptsBefore {
		if delay >= cfg.MaxDelay {
			out := jitterDelay(cfg.MaxDelay, cfg.Jitter)
			if out > cfg.MaxDelay {
				return cfg.MaxDelay
			}
			return out
		}
		delay *= 2
	}
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	out := jitterDelay(delay, cfg.Jitter)
	if out > cfg.MaxDelay {
		return cfg.MaxDelay
	}
	return out
}

func jitterDelay(delay time.Duration, jitter float64) time.Duration {
	if jitter <= 0 || delay <= 0 {
		return delay
	}
	u := (rand.Float64()*jitterSpan - 1) * jitter
	out := time.Duration(float64(delay) * (1 + u))
	if out < 0 {
		return 0
	}
	return out
}
