package bank_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/google/uuid"
)

const pctAll = 100

func TestParseOutcome(t *testing.T) {
	t.Parallel()

	o, err := bank.ParseOutcome("DECLINED")
	if err != nil || o != bank.OutcomeDeclined {
		t.Fatalf("parse DECLINED: %v %q", err, o)
	}
	if _, err := bank.ParseOutcome("bogus"); err == nil {
		t.Fatal("expected error for bogus outcome")
	}
}

func TestOutcomeRetryable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		o     bank.Outcome
		retry bool
		valid bool
	}{
		{bank.OutcomeSuccess, false, true},
		{bank.OutcomeDeclined, false, true},
		{bank.OutcomeTimeout, true, true},
		{bank.OutcomeTemporaryFailure, true, true},
		{bank.Outcome("nope"), false, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.o), func(t *testing.T) {
			t.Parallel()
			if tc.o.Retryable() != tc.retry {
				t.Fatalf("Retryable=%v want %v", tc.o.Retryable(), tc.retry)
			}
			if tc.o.Valid() != tc.valid {
				t.Fatalf("Valid=%v want %v", tc.o.Valid(), tc.valid)
			}
		})
	}
}

func TestNewGatewayUnknownProvider(t *testing.T) {
	t.Parallel()

	_, err := bank.NewGateway("http", bank.SimulatorConfig{})
	if !errors.Is(err, bank.ErrUnknownProvider) {
		t.Fatalf("got %v", err)
	}
}

func TestNewGatewaySimulator(t *testing.T) {
	t.Parallel()

	gw, err := bank.NewGateway(bank.ProviderSimulator, mustSimConfig(pctAll, 0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if gw == nil {
		t.Fatal("nil gateway")
	}
}

func TestNewGatewayEmptyProvider(t *testing.T) {
	t.Parallel()

	gw, err := bank.NewGateway("", mustSimConfig(pctAll, 0, 0, 0))
	if err != nil || gw == nil {
		t.Fatalf("default provider: gw=%v err=%v", gw, err)
	}
}

func TestSimulatorDeterministic(t *testing.T) {
	t.Parallel()

	sim, err := bank.NewDeterministicSimulator(
		bank.OutcomeTimeout,
		bank.OutcomeDeclined,
		bank.OutcomeSuccess,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	req := bank.AuthorizeRequest{PaymentPublicID: uuid.MustParse("22222222-2222-2222-2222-222222222222")}
	want := []bank.Outcome{bank.OutcomeTimeout, bank.OutcomeDeclined, bank.OutcomeSuccess, bank.OutcomeTimeout}
	for i, o := range want {
		res, err := sim.Authorize(ctx, req)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if res.Outcome != o {
			t.Fatalf("call %d: got %s want %s", i, res.Outcome, o)
		}
		if o == bank.OutcomeSuccess && res.ProviderReference != req.PaymentPublicID.String() {
			t.Fatalf("success ref %q", res.ProviderReference)
		}
		if o == bank.OutcomeDeclined && res.Message != "card_declined" {
			t.Fatalf("decline message %q", res.Message)
		}
	}
}

func TestSimulatorAlwaysSuccess(t *testing.T) {
	t.Parallel()

	sim, err := bank.NewSimulator(mustSimConfig(pctAll, 0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for range 20 {
		res, err := sim.Authorize(ctx, bank.AuthorizeRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Outcome != bank.OutcomeSuccess || res.ProviderReference == "" {
			t.Fatalf("got %+v", res)
		}
	}
}

func TestSimulatorAlwaysDeclined(t *testing.T) {
	t.Parallel()

	sim, err := bank.NewSimulator(mustSimConfig(0, pctAll, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	res, err := sim.Authorize(context.Background(), bank.AuthorizeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != bank.OutcomeDeclined {
		t.Fatalf("got %s", res.Outcome)
	}
}

func TestSimulatorCanceledContext(t *testing.T) {
	t.Parallel()

	sim, err := bank.NewDeterministicSimulator(bank.OutcomeSuccess)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = sim.Authorize(ctx, bank.AuthorizeRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestSimulatorHonorsTimeout(t *testing.T) {
	t.Parallel()

	cfg := mustSimConfig(pctAll, 0, 0, 0)
	const delayMS = 200
	const wait = 20 * time.Millisecond
	cfg.MinDelayMS = delayMS
	cfg.MaxDelayMS = delayMS
	sim, err := bank.NewSimulator(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, err = sim.Authorize(ctx, bank.AuthorizeRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestSimulatorConcurrentAuthorize(t *testing.T) {
	t.Parallel()

	sim, err := bank.NewSimulator(mustSimConfig(pctAll, 0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const n = 32
	errCh := make(chan error, n)
	for range n {
		go func() {
			_, aerr := sim.Authorize(ctx, bank.AuthorizeRequest{})
			errCh <- aerr
		}()
	}
	for range n {
		if aerr := <-errCh; aerr != nil {
			t.Fatal(aerr)
		}
	}
}

func TestSimulatorRejectsBadWeights(t *testing.T) {
	t.Parallel()

	_, err := bank.NewSimulator(bank.SimulatorConfig{SuccessPct: 50, DeclinedPct: 50, TimeoutPct: 1})
	if err == nil {
		t.Fatal("expected percent sum error")
	}
}

func TestNewDeterministicEmpty(t *testing.T) {
	t.Parallel()

	if _, err := bank.NewDeterministicSimulator(); err == nil {
		t.Fatal("expected error")
	}
}

func mustSimConfig(success, declined, timeoutPct, failure int) bank.SimulatorConfig {
	return bank.SimulatorConfig{
		SuccessPct:  success,
		DeclinedPct: declined,
		TimeoutPct:  timeoutPct,
		FailurePct:  failure,
	}
}
