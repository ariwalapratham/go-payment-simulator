package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
)

func testRetry(maxAttempts int) service.RetryConfig {
	return service.RetryConfig{
		MaxAttempts: maxAttempts,
		BaseDelay:   time.Second,
		MaxDelay:    4 * time.Second,
		Jitter:      0,
	}
}

func TestDecideAuthorizeSuccess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d, err := service.DecideAuthorize(bank.OutcomeSuccess, "", 0, now, testRetry(5))
	if err != nil {
		t.Fatal(err)
	}
	if d.NextStatus != model.PaymentStatusAuthorized || d.AttemptCount != 1 || d.LastError != nil {
		t.Fatalf("%+v", d)
	}
}

func TestDecideAuthorizeDeclinedIsTerminal(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d, err := service.DecideAuthorize(bank.OutcomeDeclined, "card_declined", 0, now, testRetry(5))
	if err != nil {
		t.Fatal(err)
	}
	if d.NextStatus != model.PaymentStatusFailed || d.AttemptCount != 1 {
		t.Fatalf("%+v", d)
	}
	if d.LastError == nil || *d.LastError != "card_declined" {
		t.Fatalf("last_error %+v", d.LastError)
	}
}

func TestDecideAuthorizeTimeoutRetries(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d, err := service.DecideAuthorize(bank.OutcomeTimeout, "", 0, now, testRetry(5))
	if err != nil {
		t.Fatal(err)
	}
	if d.NextStatus != model.PaymentStatusPending || d.AttemptCount != 1 {
		t.Fatalf("%+v", d)
	}
	if !d.NextAttemptAt.Equal(now.Add(time.Second)) {
		t.Fatalf("next_attempt_at %s", d.NextAttemptAt)
	}
}

func TestDecideAuthorizeTimeoutExhausted(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d, err := service.DecideAuthorize(bank.OutcomeTimeout, "acquirer_timeout", 4, now, testRetry(5))
	if err != nil {
		t.Fatal(err)
	}
	if d.NextStatus != model.PaymentStatusFailed || d.AttemptCount != 5 {
		t.Fatalf("%+v", d)
	}
}

func TestDecideAuthorizeBackoffCaps(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d, err := service.DecideAuthorize(bank.OutcomeTemporaryFailure, "", 3, now, testRetry(10))
	if err != nil {
		t.Fatal(err)
	}
	if d.NextStatus != model.PaymentStatusPending {
		t.Fatalf("%+v", d)
	}
	if !d.NextAttemptAt.Equal(now.Add(4 * time.Second)) {
		t.Fatalf("capped delay: %s", d.NextAttemptAt.Sub(now))
	}
}

func TestDecideAuthorizeUnknownOutcome(t *testing.T) {
	t.Parallel()

	_, err := service.DecideAuthorize(bank.Outcome("nope"), "", 0, time.Now(), testRetry(5))
	if !errors.Is(err, service.ErrUnknownOutcome) {
		t.Fatalf("got %v", err)
	}
}

func TestRetryConfigValidate(t *testing.T) {
	t.Parallel()

	cfg := testRetry(5)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.MaxAttempts = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}
