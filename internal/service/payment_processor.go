package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/observability"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/rs/zerolog"
)

const persistTimeout = 5 * time.Second

// ErrNoJob means the worker should idle until the next poll.
var ErrNoJob = repository.ErrNoJob

type authorizeStore interface {
	ClaimDuePayment(ctx context.Context, lease time.Duration) (*repository.PaymentRecord, error)
	ApplyAuthorizeDecision(ctx context.Context, in repository.ApplyAuthorizeInput) error
}

// PaymentProcessor claims a due PENDING payment, calls the bank, and applies DecideAuthorize.
type PaymentProcessor struct {
	store       authorizeStore
	gateway     bank.Gateway
	retry       RetryConfig
	callTimeout time.Duration
	lease       time.Duration
	metrics     observability.AuthorizeMetrics
	log         zerolog.Logger
	now         func() time.Time
}

// NewPaymentProcessor wires authorize processing. log and metrics may be nil.
func NewPaymentProcessor(
	store authorizeStore,
	gateway bank.Gateway,
	retry RetryConfig,
	callTimeout, lease time.Duration,
	log *zerolog.Logger,
	metrics observability.AuthorizeMetrics,
) (*PaymentProcessor, error) {
	if store == nil {
		return nil, fmt.Errorf("payment processor: store is required")
	}
	if gateway == nil {
		return nil, fmt.Errorf("payment processor: gateway is required")
	}
	if err := retry.Validate(); err != nil {
		return nil, fmt.Errorf("payment processor: %w", err)
	}
	if callTimeout <= 0 {
		return nil, fmt.Errorf("payment processor: call timeout must be > 0")
	}
	if lease < callTimeout {
		return nil, fmt.Errorf("payment processor: lease must be >= call timeout")
	}
	p := &PaymentProcessor{
		store:       store,
		gateway:     gateway,
		retry:       retry,
		callTimeout: callTimeout,
		lease:       lease,
		metrics:     metrics,
		log:         zerolog.Nop(),
		now:         time.Now,
	}
	if p.metrics == nil {
		p.metrics = observability.NopAuthorizeMetrics{}
	}
	if log != nil {
		p.log = *log
	}
	return p, nil
}

// ProcessNext claims one due payment and authorizes it. Returns ErrNoJob when the queue is empty.
func (p *PaymentProcessor) ProcessNext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	job, err := p.store.ClaimDuePayment(ctx, p.lease)
	if err != nil {
		if errors.Is(err, repository.ErrNoJob) {
			return ErrNoJob
		}
		return err
	}
	return p.process(ctx, job)
}

func (p *PaymentProcessor) process(ctx context.Context, job *repository.PaymentRecord) error {
	if job.Payment.Status != model.PaymentStatusPending {
		return nil
	}
	if ctx.Err() != nil {
		return p.releaseClaim(ctx, job)
	}

	callCtx, cancel := context.WithTimeout(ctx, p.callTimeout)
	defer cancel()

	res, err := p.gateway.Authorize(callCtx, bank.AuthorizeRequest{
		PaymentPublicID:  job.Payment.PublicID,
		MerchantPublicID: job.MerchantPublicID,
		Amount:           job.Payment.Amount,
		Currency:         job.Payment.Currency,
	})
	if err != nil {
		if ctx.Err() != nil {
			return p.releaseClaim(ctx, job)
		}
		res.Outcome = bank.OutcomeTimeout
		res.Message = err.Error()
	}

	decision, err := DecideAuthorize(res.Outcome, res.Message, job.Payment.AttemptCount, p.now(), p.retry)
	if err != nil {
		return fmt.Errorf("decide authorize %s: %w", job.Payment.PublicID, err)
	}

	applied, err := p.persist(ctx, job, repository.ApplyAuthorizeInput{
		PaymentID:            job.Payment.ID,
		ExpectedAttemptCount: job.Payment.AttemptCount,
		Status:               decision.NextStatus,
		AttemptCount:         decision.AttemptCount,
		NextAttemptAt:        decision.NextAttemptAt,
		LastError:            decision.LastError,
	})
	if err != nil || !applied {
		return err
	}

	p.metrics.ObserveAuthorizeAttempt(res.Outcome.String(), decision.NextStatus.String())
	if decision.NextStatus == model.PaymentStatusPending {
		p.metrics.ObserveAuthorizeRetry()
	}
	p.logAuthorize(ctx, job, res.Outcome, decision)
	return nil
}

func (p *PaymentProcessor) persist(ctx context.Context, job *repository.PaymentRecord, in repository.ApplyAuthorizeInput) (bool, error) {
	dbCtx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()
	err := p.store.ApplyAuthorizeDecision(dbCtx, in)
	if errors.Is(err, repository.ErrLostLease) {
		p.metrics.ObserveClaimLost()
		e := p.evt(ctx).
			Str("payment_id", job.Payment.PublicID.String()).
			Str("event", "payment.authorize_write_abandoned")
		var lost repository.LostLeaseError
		if errors.As(err, &lost) && lost.Status != "" {
			e = e.Str("status", lost.Status.String())
		}
		e.Msg("abandoned authorize write, no longer pending")
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("apply authorize %s: %w", job.Payment.PublicID, err)
	}
	return true, nil
}

func (p *PaymentProcessor) releaseClaim(ctx context.Context, job *repository.PaymentRecord) error {
	applied, err := p.persist(ctx, job, repository.ApplyAuthorizeInput{
		PaymentID:            job.Payment.ID,
		ExpectedAttemptCount: job.Payment.AttemptCount,
		Status:               model.PaymentStatusPending,
		AttemptCount:         job.Payment.AttemptCount,
		NextAttemptAt:        p.now(),
		LastError:            job.Payment.LastError,
	})
	if err != nil || !applied {
		return err
	}
	p.evt(ctx).
		Str("payment_id", job.Payment.PublicID.String()).
		Str("event", "payment.authorize_claim_released").
		Str("reason", "shutdown").
		Msg("authorize claim released")
	return nil
}

func (p *PaymentProcessor) logAuthorize(ctx context.Context, job *repository.PaymentRecord, outcome bank.Outcome, d Decision) {
	event, msg := authorizeEvent(d.NextStatus)
	e := p.evt(ctx).
		Str("payment_id", job.Payment.PublicID.String()).
		Str("merchant_id", job.MerchantPublicID.String()).
		Int("attempt", d.AttemptCount).
		Str("bank_outcome", outcome.String()).
		Str("from_status", model.PaymentStatusPending.String()).
		Str("to_status", d.NextStatus.String()).
		Str("event", event)
	if d.NextStatus == model.PaymentStatusPending {
		e = e.Time("next_attempt_at", d.NextAttemptAt)
	}
	if d.LastError != nil {
		e = e.Str("last_error", *d.LastError)
	}
	e.Msg(msg)
}

func (p *PaymentProcessor) evt(ctx context.Context) *zerolog.Event {
	e := p.log.Info().Str("component", "payment_processor")
	if id, ok := observability.WorkerIDFrom(ctx); ok {
		e = e.Int("worker_id", id)
	}
	return e
}

func authorizeEvent(to model.PaymentStatus) (event, msg string) {
	switch to {
	case model.PaymentStatusAuthorized:
		return "payment.authorized", "payment authorized"
	case model.PaymentStatusFailed:
		return "payment.authorize_failed", "payment authorize failed"
	case model.PaymentStatusPending:
		return "payment.authorize_retry_scheduled", "payment authorize retry scheduled"
	default:
		return "payment.authorize_processed", "authorize processed"
	}
}
