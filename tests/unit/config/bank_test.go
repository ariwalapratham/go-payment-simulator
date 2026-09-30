package config_test

import (
	"errors"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/config"
)

func TestDefaultBankConfig(t *testing.T) {
	t.Parallel()

	c := config.DefaultBankConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Provider != bank.ProviderSimulator {
		t.Fatalf("provider %q", c.Provider)
	}
	if c.CallTimeout < 1 {
		t.Fatalf("timeout %d", c.CallTimeout)
	}
	sum := c.Simulator.SuccessPct + c.Simulator.DeclinedPct +
		c.Simulator.TimeoutPct + c.Simulator.FailurePct
	if sum != 100 {
		t.Fatalf("weights %+v", c.Simulator)
	}
}

func TestBankConfigNewGateway(t *testing.T) {
	t.Parallel()

	c := config.DefaultBankConfig()
	gw, err := bank.NewGateway(c.Provider, c.Simulator)
	if err != nil {
		t.Fatal(err)
	}
	if gw == nil {
		t.Fatal("nil gateway")
	}
}

func TestBankConfigRejectsUnknownProvider(t *testing.T) {
	t.Parallel()

	c := config.DefaultBankConfig()
	c.Provider = "http"
	err := c.Validate()
	if !errors.Is(err, bank.ErrUnknownProvider) {
		t.Fatalf("got %v", err)
	}
}
