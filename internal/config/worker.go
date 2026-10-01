package config

import "fmt"

const (
	defaultWorkerPoolSize       = 4
	defaultWorkerPollIntervalMS = 200
	defaultWorkerLeaseSeconds   = 45
	defaultLeaseBufferSec       = 15
	defaultWorkerMaxAttempts    = 5
	defaultWorkerBaseDelayMS    = 1000
	defaultWorkerMaxDelayMS     = 30000
	defaultWorkerJitter         = 0.1
)

// WorkerConfig is the authorize poller and retry policy. Durations are milliseconds except lease_seconds.
type WorkerConfig struct {
	PoolSize       int      `koanf:"pool_size"`
	PollIntervalMS int      `koanf:"poll_interval_ms"`
	LeaseSeconds   int      `koanf:"lease_seconds"`
	MaxAttempts    int      `koanf:"max_attempts"`
	BaseDelayMS    int      `koanf:"base_delay_ms"`
	MaxDelayMS     int      `koanf:"max_delay_ms"`
	Jitter         *float64 `koanf:"jitter"`
}

// DefaultWorkerConfig is a small in-process pool with exponential backoff.
func DefaultWorkerConfig() WorkerConfig {
	j := defaultWorkerJitter
	return WorkerConfig{
		PoolSize:       defaultWorkerPoolSize,
		PollIntervalMS: defaultWorkerPollIntervalMS,
		LeaseSeconds:   defaultWorkerLeaseSeconds,
		MaxAttempts:    defaultWorkerMaxAttempts,
		BaseDelayMS:    defaultWorkerBaseDelayMS,
		MaxDelayMS:     defaultWorkerMaxDelayMS,
		Jitter:         &j,
	}
}

func (c *WorkerConfig) applyDefaults(bankCallTimeoutSec int) {
	d := DefaultWorkerConfig()
	if c.PoolSize <= 0 {
		c.PoolSize = d.PoolSize
	}
	if c.PollIntervalMS <= 0 {
		c.PollIntervalMS = d.PollIntervalMS
	}
	if c.LeaseSeconds <= 0 {
		c.LeaseSeconds = bankCallTimeoutSec + defaultLeaseBufferSec
		if c.LeaseSeconds < 1 {
			c.LeaseSeconds = d.LeaseSeconds
		}
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = d.MaxAttempts
	}
	if c.BaseDelayMS <= 0 {
		c.BaseDelayMS = d.BaseDelayMS
	}
	if c.MaxDelayMS <= 0 {
		c.MaxDelayMS = d.MaxDelayMS
	}
	if c.Jitter == nil {
		j := defaultWorkerJitter
		c.Jitter = &j
	}
}

// RetryJitter is 0 when unset.
func (c WorkerConfig) RetryJitter() float64 {
	if c.Jitter == nil {
		return 0
	}
	return *c.Jitter
}

// Validate checks worker and retry bounds after applyDefaults.
func (c WorkerConfig) Validate() error {
	if c.PoolSize < 1 {
		return fmt.Errorf("worker pool_size must be >= 1")
	}
	if c.PollIntervalMS < 1 {
		return fmt.Errorf("worker poll_interval_ms must be >= 1")
	}
	if c.LeaseSeconds < 1 {
		return fmt.Errorf("worker lease_seconds must be >= 1")
	}
	if c.MaxAttempts < 1 {
		return fmt.Errorf("worker max_attempts must be >= 1")
	}
	if c.BaseDelayMS < 1 {
		return fmt.Errorf("worker base_delay_ms must be >= 1")
	}
	if c.MaxDelayMS < c.BaseDelayMS {
		return fmt.Errorf("worker max_delay_ms must be >= base_delay_ms")
	}
	j := c.RetryJitter()
	if j < 0 || j > 1 {
		return fmt.Errorf("worker jitter must be in [0, 1]")
	}
	return nil
}

// ValidateLease covers the bank call so two workers cannot authorize the same row.
func (c WorkerConfig) ValidateLease(bankCallTimeoutSec int) error {
	if c.LeaseSeconds < bankCallTimeoutSec {
		return fmt.Errorf("worker lease_seconds (%d) must be >= bank call_timeout (%d)", c.LeaseSeconds, bankCallTimeoutSec)
	}
	return nil
}
