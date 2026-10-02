package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/observability"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/google/uuid"
)

type fakeBank struct {
	mu       sync.Mutex
	outcomes []bank.Outcome
	i        int
	delay    time.Duration
	started  chan struct{}
}

func (f *fakeBank) Authorize(ctx context.Context, _ bank.AuthorizeRequest) (bank.AuthorizeResult, error) {
	if f.started != nil {
		select {
		case <-f.started:
		default:
			close(f.started)
		}
	}
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return bank.AuthorizeResult{}, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.i >= len(f.outcomes) {
		return bank.AuthorizeResult{}, errors.New("no outcomes left")
	}
	o := f.outcomes[f.i]
	f.i++
	return bank.AuthorizeResult{Outcome: o, Message: o.String()}, nil
}

type fakeStore struct {
	mu      sync.Mutex
	job     *repository.PaymentRecord
	applied []repository.ApplyAuthorizeInput
}

func (f *fakeStore) ClaimDuePayment(context.Context, time.Duration) (*repository.PaymentRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.job == nil || f.job.Payment.Status != model.PaymentStatusPending {
		return nil, repository.ErrNoJob
	}
	cp := *f.job
	return &cp, nil
}

func (f *fakeStore) ApplyAuthorizeDecision(_ context.Context, in repository.ApplyAuthorizeInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, in)
	if f.job != nil {
		f.job.Payment.Status = in.Status
		f.job.Payment.AttemptCount = in.AttemptCount
	}
	return nil
}

func pendingJob() *repository.PaymentRecord {
	return &repository.PaymentRecord{
		MerchantPublicID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Payment: model.DBPayment{
			ID:       1,
			PublicID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Amount:   5000,
			Currency: "USD",
			Status:   model.PaymentStatusPending,
		},
	}
}

type recMetrics struct {
	mu       sync.Mutex
	attempts int
	retries  int
	lost     int
	outcome  string
	toStatus string
}

func (m *recMetrics) ObserveAuthorizeAttempt(bankOutcome, toStatus string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts++
	m.outcome = bankOutcome
	m.toStatus = toStatus
}

func (m *recMetrics) ObserveAuthorizeRetry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retries++
}

func (m *recMetrics) ObserveClaimLost() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lost++
}

func testProcessor(t *testing.T, store *fakeStore, gw bank.Gateway) *service.PaymentProcessor {
	t.Helper()
	return testProcessorMetrics(t, store, gw, nil)
}

func testProcessorMetrics(
	t *testing.T, store *fakeStore, gw bank.Gateway, metrics observability.AuthorizeMetrics,
) *service.PaymentProcessor {
	t.Helper()
	p, err := service.NewPaymentProcessor(
		store, gw, testRetry(5), 50*time.Millisecond, time.Second, nil, metrics,
	)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProcessorAuthorizeSuccess(t *testing.T) {
	t.Parallel()

	store := &fakeStore{job: pendingJob()}
	p := testProcessor(t, store, &fakeBank{outcomes: []bank.Outcome{bank.OutcomeSuccess}})
	if err := p.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.applied) != 1 || store.applied[0].Status != model.PaymentStatusAuthorized {
		t.Fatalf("applied %+v", store.applied)
	}
	if store.applied[0].AttemptCount != 1 {
		t.Fatalf("attempts %d", store.applied[0].AttemptCount)
	}
}

func TestProcessorDecline(t *testing.T) {
	t.Parallel()

	store := &fakeStore{job: pendingJob()}
	p := testProcessor(t, store, &fakeBank{outcomes: []bank.Outcome{bank.OutcomeDeclined}})
	if err := p.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.applied[0].Status != model.PaymentStatusFailed {
		t.Fatalf("status %s", store.applied[0].Status)
	}
}

func TestProcessorRetryThenSuccess(t *testing.T) {
	t.Parallel()

	store := &fakeStore{job: pendingJob()}
	gw := &fakeBank{outcomes: []bank.Outcome{bank.OutcomeTimeout, bank.OutcomeSuccess}}
	p := testProcessor(t, store, gw)
	if err := p.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.applied[0].Status != model.PaymentStatusPending {
		t.Fatalf("first %s", store.applied[0].Status)
	}
	if err := p.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.applied[1].Status != model.PaymentStatusAuthorized || store.applied[1].AttemptCount != 2 {
		t.Fatalf("second %+v", store.applied[1])
	}
}

func TestProcessorNoJob(t *testing.T) {
	t.Parallel()

	p := testProcessor(t, &fakeStore{}, &fakeBank{})
	err := p.ProcessNext(context.Background())
	if !errors.Is(err, service.ErrNoJob) {
		t.Fatalf("got %v", err)
	}
}

func TestProcessorShutdownSkipsClaim(t *testing.T) {
	t.Parallel()

	store := &fakeStore{job: pendingJob()}
	p := testProcessor(t, store, &fakeBank{
		outcomes: []bank.Outcome{bank.OutcomeSuccess},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.ProcessNext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if len(store.applied) != 0 {
		t.Fatalf("applied on shutdown: %+v", store.applied)
	}
}

func TestProcessorCancelDuringAuthorizeReleasesLease(t *testing.T) {
	t.Parallel()

	store := &fakeStore{job: pendingJob()}
	gw := &fakeBank{
		outcomes: []bank.Outcome{bank.OutcomeSuccess},
		delay:    time.Second,
		started:  make(chan struct{}),
	}
	p, err := service.NewPaymentProcessor(store, gw, testRetry(5), 2*time.Second, 3*time.Second, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.ProcessNext(ctx) }()
	select {
	case <-gw.started:
	case <-time.After(time.Second):
		t.Fatal("authorize did not start")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(store.applied) != 1 {
		t.Fatalf("applied %+v", store.applied)
	}
	got := store.applied[0]
	if got.Status != model.PaymentStatusPending || got.AttemptCount != 0 {
		t.Fatalf("lease release %+v", got)
	}
}

func TestProcessorMetricsOnSuccessAndRetry(t *testing.T) {
	t.Parallel()

	store := &fakeStore{job: pendingJob()}
	m := &recMetrics{}
	gw := &fakeBank{outcomes: []bank.Outcome{bank.OutcomeTimeout, bank.OutcomeSuccess}}
	p := testProcessorMetrics(t, store, gw, m)
	if err := p.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.attempts != 1 || m.retries != 1 || m.toStatus != string(model.PaymentStatusPending) {
		t.Fatalf("retry metrics %+v", m)
	}
	if err := p.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.attempts != 2 || m.retries != 1 || m.outcome != string(bank.OutcomeSuccess) {
		t.Fatalf("success metrics %+v", m)
	}
}
