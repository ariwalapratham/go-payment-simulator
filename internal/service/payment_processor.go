package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
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
	log         zerolog.Logger
	now         func() time.Time
}

// NewPaymentProcessor wires authorize processing. log may be nil.
func NewPaymentProcessor(
	store authorizeStore,
	gateway bank.Gateway,
	retry RetryConfig,
	callTimeout, lease time.Duration,
	log *zerolog.Logger,
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
		log:         zerolog.Nop(),
		now:         time.Now,
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
		return p.releaseClaim(job)
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
			return p.releaseClaim(job)
		}
		res.Outcome = bank.OutcomeTimeout
		res.Message = err.Error()
	}

	decision, err := DecideAuthorize(res.Outcome, res.Message, job.Payment.AttemptCount, p.now(), p.retry)
	if err != nil {
		return fmt.Errorf("decide authorize %s: %w", job.Payment.PublicID, err)
	}

	if err := p.persist(job, repository.ApplyAuthorizeInput{
		PaymentID:            job.Payment.ID,
		ExpectedAttemptCount: job.Payment.AttemptCount,
		Status:               decision.NextStatus,
		AttemptCount:         decision.AttemptCount,
		NextAttemptAt:        decision.NextAttemptAt,
		LastError:            decision.LastError,
	}); err != nil {
		return err
	}

	p.log.Info().
		Str("payment_id", job.Payment.PublicID.String()).
		Int("attempt", decision.AttemptCount).
		Str("outcome", res.Outcome.String()).
		Str("next_status", decision.NextStatus.String()).
		Time("next_attempt_at", decision.NextAttemptAt).
		Msg("authorize processed")
	return nil
}

func (p *PaymentProcessor) persist(job *repository.PaymentRecord, in repository.ApplyAuthorizeInput) error {
	ctx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()
	err := p.store.ApplyAuthorizeDecision(ctx, in)
	if errors.Is(err, repository.ErrLostLease) {
		p.log.Info().Str("payment_id", job.Payment.PublicID.String()).Msg("authorize claim lost")
		return nil
	}
	if err != nil {
		return fmt.Errorf("apply authorize %s: %w", job.Payment.PublicID, err)
	}
	return nil
}

func (p *PaymentProcessor) releaseClaim(job *repository.PaymentRecord) error {
	return p.persist(job, repository.ApplyAuthorizeInput{
		PaymentID:            job.Payment.ID,
		ExpectedAttemptCount: job.Payment.AttemptCount,
		Status:               model.PaymentStatusPending,
		AttemptCount:         job.Payment.AttemptCount,
		NextAttemptAt:        p.now(),
		LastError:            job.Payment.LastError,
	})
}
