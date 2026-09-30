package bank

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Simulator is a Gateway that draws SUCCESS / DECLINED / TIMEOUT / TEMPORARY_FAILURE.
type Simulator struct {
	cfg  SimulatorConfig
	seq  []Outcome
	mu   sync.Mutex
	rng  *rand.Rand
	seqI int
}

// NewSimulator builds a weighted simulator. Safe for concurrent Authorize calls.
func NewSimulator(cfg SimulatorConfig) (*Simulator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Simulator{cfg: cfg, rng: newRNG(cfg.Seed)}, nil
}

// NewDeterministicSimulator returns seq in order, wrapping. At least one valid outcome is required.
func NewDeterministicSimulator(outcomes ...Outcome) (*Simulator, error) {
	if len(outcomes) == 0 {
		return nil, fmt.Errorf("deterministic simulator needs at least one outcome")
	}
	for _, o := range outcomes {
		if !o.Valid() {
			return nil, fmt.Errorf("invalid bank outcome: %q", o)
		}
	}
	return &Simulator{
		seq: slices.Clone(outcomes),
		rng: newRNG(nil),
	}, nil
}

// Authorize implements Gateway. req is used for a stable provider reference on success.
func (s *Simulator) Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error) {
	if err := ctx.Err(); err != nil {
		return AuthorizeResult{}, err
	}
	delay, outcome := s.next()
	if err := wait(ctx, delay); err != nil {
		return AuthorizeResult{}, err
	}
	return resultFor(outcome, req.PaymentPublicID), nil
}

func (s *Simulator) next() (time.Duration, Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.delayLocked(), s.pickLocked()
}

func (s *Simulator) pickLocked() Outcome {
	if n := len(s.seq); n > 0 {
		o := s.seq[s.seqI%n]
		s.seqI++
		return o
	}
	n := s.rng.IntN(percentTotal)
	n -= s.cfg.SuccessPct
	if n < 0 {
		return OutcomeSuccess
	}
	n -= s.cfg.DeclinedPct
	if n < 0 {
		return OutcomeDeclined
	}
	n -= s.cfg.TimeoutPct
	if n < 0 {
		return OutcomeTimeout
	}
	return OutcomeTemporaryFailure
}

func (s *Simulator) delayLocked() time.Duration {
	minD := time.Duration(s.cfg.MinDelayMS) * time.Millisecond
	maxD := time.Duration(s.cfg.MaxDelayMS) * time.Millisecond
	if maxD <= minD {
		return minD
	}
	span := int64(maxD - minD)
	return minD + time.Duration(s.rng.Int64N(span+1))
}

func resultFor(outcome Outcome, paymentID uuid.UUID) AuthorizeResult {
	res := AuthorizeResult{Outcome: outcome}
	switch outcome {
	case OutcomeSuccess:
		if paymentID != uuid.Nil {
			res.ProviderReference = paymentID.String()
		} else {
			res.ProviderReference = uuid.NewString()
		}
	case OutcomeDeclined:
		res.Message = "card_declined"
	case OutcomeTimeout:
		res.Message = "acquirer_timeout"
	case OutcomeTemporaryFailure:
		res.Message = "temporary_failure"
	}
	return res
}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func newRNG(seed *int64) *rand.Rand {
	var s uint64
	if seed != nil {
		s = uint64(*seed) //nolint:gosec // G115: RNG seed, not a size conversion
	} else {
		s = uint64(time.Now().UnixNano()) //nolint:gosec // G115: RNG seed from clock
	}
	return rand.New(rand.NewPCG(s, s^1))
}
