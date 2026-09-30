package bank

import "fmt"

const percentTotal = 100

// SimulatorConfig is weighted fake-acquirer outcomes. Percents must sum to 100.
type SimulatorConfig struct {
	SuccessPct  int    `koanf:"success_pct"`
	DeclinedPct int    `koanf:"declined_pct"`
	TimeoutPct  int    `koanf:"timeout_pct"`
	FailurePct  int    `koanf:"failure_pct"`
	MinDelayMS  int    `koanf:"min_delay_ms"`
	MaxDelayMS  int    `koanf:"max_delay_ms"`
	Seed        *int64 `koanf:"seed"`
}

// Validate checks weights and delay bounds. Zero config is invalid; apply app defaults first.
func (c SimulatorConfig) Validate() error {
	if c.SuccessPct < 0 || c.DeclinedPct < 0 || c.TimeoutPct < 0 || c.FailurePct < 0 {
		return fmt.Errorf("simulator outcome percents must be >= 0")
	}
	sum := c.SuccessPct + c.DeclinedPct + c.TimeoutPct + c.FailurePct
	if sum != percentTotal {
		return fmt.Errorf("simulator outcome percents must sum to %d, got %d", percentTotal, sum)
	}
	if c.MinDelayMS < 0 || c.MaxDelayMS < 0 {
		return fmt.Errorf("simulator delay_ms must be >= 0")
	}
	if c.MaxDelayMS < c.MinDelayMS {
		return fmt.Errorf("simulator max_delay_ms %d < min_delay_ms %d", c.MaxDelayMS, c.MinDelayMS)
	}
	return nil
}
