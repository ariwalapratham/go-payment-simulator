package loadtest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	defaultRefundsPer = 8
	minRefundPayments = 2
	minRefundsPer     = 2
)

// RefundConfig is a concurrent capture-then-refund burst against already-created payments.
type RefundConfig struct {
	Payments    int
	RefundsPer  int
	Concurrency int
	WaitTimeout time.Duration
}

// RefundCall is one HTTP refund attempt.
type RefundCall struct {
	PaymentID uuid.UUID
	SharedKey bool
	Status    int
	RefundID  uuid.UUID
}

// PaymentWaiter blocks until the given public ids are AUTHORIZED.
type PaymentWaiter interface {
	WaitAuthorized(ctx context.Context, ids []uuid.UUID) error
}

// RefundBurst creates payments, waits for AUTHORIZED, captures, then refunds.
// Half the payments reuse one idempotency key; the rest use a fresh key per call.
func RefundBurst(ctx context.Context, api *Client, wait PaymentWaiter, cfg RefundConfig) ([]RefundCall, error) {
	n, per, limit, paramErr := refundParams(cfg)
	if paramErr != nil {
		return nil, paramErr
	}
	if api == nil {
		return nil, errors.New("client is required")
	}
	if wait == nil {
		return nil, errors.New("payment waiter is required")
	}
	ids, createErr := createPayments(ctx, api, n)
	if createErr != nil {
		return nil, createErr
	}
	waitCtx := ctx
	if cfg.WaitTimeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, cfg.WaitTimeout)
		defer cancel()
	}
	if waitErr := wait.WaitAuthorized(waitCtx, ids); waitErr != nil {
		return nil, fmt.Errorf("wait authorized: %w", waitErr)
	}
	if capErr := capturePayments(ctx, api, ids); capErr != nil {
		return nil, capErr
	}
	calls, refundErr := refundPayments(ctx, api, ids, per, limit)
	if refundErr != nil {
		return calls, refundErr
	}
	if checkErr := ValidateRefundCalls(calls); checkErr != nil {
		return calls, checkErr
	}
	return calls, nil
}

func refundParams(cfg RefundConfig) (int, int, int, error) {
	n := cfg.Payments
	if n < minRefundPayments {
		return 0, 0, 0, fmt.Errorf("payments must be >= %d so both shared and unique keys are covered", minRefundPayments)
	}
	per := cfg.RefundsPer
	if per < minRefundsPer {
		per = defaultRefundsPer
	}
	limit := cfg.Concurrency
	if limit < 1 {
		limit = n * per
	}
	return n, per, limit, nil
}

func createPayments(ctx context.Context, api *Client, n int) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, n)
	for range n {
		id, code, err := api.CreatePayment(ctx, uuid.NewString())
		if err != nil {
			return nil, fmt.Errorf("create payment: %w", err)
		}
		if code != http.StatusCreated {
			return nil, fmt.Errorf("create payment status %d", code)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func capturePayments(ctx context.Context, api *Client, ids []uuid.UUID) error {
	for _, id := range ids {
		code, err := api.Capture(ctx, id)
		if err != nil {
			return err
		}
		if code != http.StatusOK {
			return fmt.Errorf("capture %s status %d", id, code)
		}
	}
	return nil
}

func refundPayments(ctx context.Context, api *Client, ids []uuid.UUID, per, limit int) ([]RefundCall, error) {
	sharedCutoff := len(ids) / 2 //nolint:mnd // half share a key, half use unique keys
	calls := make([]RefundCall, len(ids)*per)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	var mu sync.Mutex
	for i, id := range ids {
		shared := i < sharedCutoff
		sharedKey := uuid.NewString()
		for r := range per {
			idx := i*per + r
			paymentID := id
			key := uuid.NewString()
			if shared {
				key = sharedKey
			}
			g.Go(func() error {
				refundID, code, err := api.Refund(gctx, paymentID, key)
				if err != nil {
					return fmt.Errorf("refund %s: %w", paymentID, err)
				}
				mu.Lock()
				calls[idx] = RefundCall{
					PaymentID: paymentID,
					SharedKey: shared,
					Status:    code,
					RefundID:  refundID,
				}
				mu.Unlock()
				return nil
			})
		}
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return calls, nil
}

// ValidateRefundCalls checks shared-key calls share one refund id and unique-key
// calls produce one 201 and the rest 422.
func ValidateRefundCalls(calls []RefundCall) error {
	type agg struct {
		shared    bool
		statuses  []int
		refundIDs []uuid.UUID
	}
	byPay := make(map[uuid.UUID]*agg)
	for _, c := range calls {
		a, ok := byPay[c.PaymentID]
		if !ok {
			a = &agg{shared: c.SharedKey}
			byPay[c.PaymentID] = a
		}
		a.statuses = append(a.statuses, c.Status)
		if c.RefundID != uuid.Nil {
			a.refundIDs = append(a.refundIDs, c.RefundID)
		}
	}
	for id, a := range byPay {
		if a.shared {
			if err := checkShared(id, a.statuses, a.refundIDs); err != nil {
				return err
			}
			continue
		}
		if err := checkUnique(id, a.statuses); err != nil {
			return err
		}
	}
	return nil
}

func checkShared(paymentID uuid.UUID, statuses []int, ids []uuid.UUID) error {
	for _, s := range statuses {
		if s != http.StatusOK && s != http.StatusCreated {
			return fmt.Errorf("payment %s shared-key refund status %d", paymentID, s)
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("payment %s shared-key refunds returned no id", paymentID)
	}
	first := ids[0]
	for _, id := range ids[1:] {
		if id != first {
			return fmt.Errorf("payment %s shared-key refunds returned mixed ids", paymentID)
		}
	}
	return nil
}

func checkUnique(paymentID uuid.UUID, statuses []int) error {
	created, conflict := 0, 0
	for _, s := range statuses {
		switch s {
		case http.StatusCreated:
			created++
		case http.StatusUnprocessableEntity:
			conflict++
		default:
			return fmt.Errorf("payment %s unique-key refund status %d", paymentID, s)
		}
	}
	if created != 1 {
		return fmt.Errorf("payment %s unique-key refunds: want 1 created, got %d", paymentID, created)
	}
	if conflict != len(statuses)-1 {
		return fmt.Errorf("payment %s unique-key refunds: want %d conflicts, got %d", paymentID, len(statuses)-1, conflict)
	}
	return nil
}
