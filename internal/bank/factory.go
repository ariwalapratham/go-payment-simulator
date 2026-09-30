package bank

import (
	"errors"
	"fmt"
)

// ProviderSimulator is the in-process fake acquirer.
const ProviderSimulator = "simulator"

// ErrUnknownProvider is returned when the bank provider has no adapter.
var ErrUnknownProvider = errors.New("unknown bank provider")

// NewGateway builds the adapter for provider. Empty provider means simulator.
func NewGateway(provider string, sim SimulatorConfig) (Gateway, error) {
	if provider == "" {
		provider = ProviderSimulator
	}
	switch provider {
	case ProviderSimulator:
		return NewSimulator(sim)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, provider)
	}
}
