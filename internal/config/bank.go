package config

import (
	"fmt"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
)

const (
	defaultBankCallTimeoutSec = 30
	defaultBankSuccessPct     = 90
	defaultBankDeclinedPct    = 5
	defaultBankTimeoutPct     = 3
	defaultBankFailurePct     = 2
)

// BankConfig is the fake/real acquirer settings. CallTimeout is seconds for the worker's Authorize context.
type BankConfig struct {
	Provider    string               `koanf:"provider"`
	CallTimeout int                  `koanf:"call_timeout"`
	Simulator   bank.SimulatorConfig `koanf:"simulator"`
}

// DefaultBankConfig is default simulator weights and a 30s authorize timeout.
func DefaultBankConfig() BankConfig {
	return BankConfig{
		Provider:    bank.ProviderSimulator,
		CallTimeout: defaultBankCallTimeoutSec,
		Simulator: bank.SimulatorConfig{
			SuccessPct:  defaultBankSuccessPct,
			DeclinedPct: defaultBankDeclinedPct,
			TimeoutPct:  defaultBankTimeoutPct,
			FailurePct:  defaultBankFailurePct,
		},
	}
}

func (c *BankConfig) applyDefaults() {
	d := DefaultBankConfig()
	if c.Provider == "" {
		c.Provider = d.Provider
	}
	if c.CallTimeout <= 0 {
		c.CallTimeout = d.CallTimeout
	}
	if c.Simulator.SuccessPct == 0 && c.Simulator.DeclinedPct == 0 &&
		c.Simulator.TimeoutPct == 0 && c.Simulator.FailurePct == 0 {
		c.Simulator.SuccessPct = d.Simulator.SuccessPct
		c.Simulator.DeclinedPct = d.Simulator.DeclinedPct
		c.Simulator.TimeoutPct = d.Simulator.TimeoutPct
		c.Simulator.FailurePct = d.Simulator.FailurePct
	}
}

// Validate checks provider, timeout, and simulator weights after applyDefaults.
func (c BankConfig) Validate() error {
	if c.Provider != bank.ProviderSimulator {
		return fmt.Errorf("%w: %s", bank.ErrUnknownProvider, c.Provider)
	}
	if c.CallTimeout < 1 {
		return fmt.Errorf("bank call_timeout must be >= 1 second")
	}
	return c.Simulator.Validate()
}
