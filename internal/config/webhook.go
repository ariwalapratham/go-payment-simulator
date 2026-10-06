package config

import "fmt"

const (
	defaultWebhookPoolSize       = 4
	defaultWebhookPollIntervalMS = 200
	defaultWebhookLeaseSeconds   = 45
	defaultWebhookCallTimeoutSec = 30
	defaultWebhookMaxAttempts    = 5
	defaultWebhookBaseDelayMS    = 1000
	defaultWebhookMaxDelayMS     = 16000
	defaultWebhookJitter         = 0.1
)

// WebhookConfig drives the outbound delivery worker pool and retry policy.
type WebhookConfig struct {
	PoolSize       int      `koanf:"pool_size"`
	PollIntervalMS int      `koanf:"poll_interval_ms"`
	LeaseSeconds   int      `koanf:"lease_seconds"`
	CallTimeoutSec int      `koanf:"call_timeout_sec"`
	MaxAttempts    int      `koanf:"max_attempts"`
	BaseDelayMS    int      `koanf:"base_delay_ms"`
	MaxDelayMS     int      `koanf:"max_delay_ms"`
	Jitter         *float64 `koanf:"jitter"`
}

// DefaultWebhookConfig is a small in-process pool with exponential backoff (1s–16s).
func DefaultWebhookConfig() WebhookConfig {
	j := defaultWebhookJitter
	return WebhookConfig{
		PoolSize:       defaultWebhookPoolSize,
		PollIntervalMS: defaultWebhookPollIntervalMS,
		LeaseSeconds:   defaultWebhookLeaseSeconds,
		CallTimeoutSec: defaultWebhookCallTimeoutSec,
		MaxAttempts:    defaultWebhookMaxAttempts,
		BaseDelayMS:    defaultWebhookBaseDelayMS,
		MaxDelayMS:     defaultWebhookMaxDelayMS,
		Jitter:         &j,
	}
}

func (c *WebhookConfig) applyDefaults() {
	d := DefaultWebhookConfig()
	if c.PoolSize <= 0 {
		c.PoolSize = d.PoolSize
	}
	if c.PollIntervalMS <= 0 {
		c.PollIntervalMS = d.PollIntervalMS
	}
	if c.LeaseSeconds <= 0 {
		c.LeaseSeconds = d.LeaseSeconds
	}
	if c.CallTimeoutSec <= 0 {
		c.CallTimeoutSec = d.CallTimeoutSec
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
		j := defaultWebhookJitter
		c.Jitter = &j
	}
}

// RetryJitter is 0 when unset.
func (c WebhookConfig) RetryJitter() float64 {
	if c.Jitter == nil {
		return 0
	}
	return *c.Jitter
}

func (c WebhookConfig) Validate() error {
	if c.PoolSize < 1 {
		return fmt.Errorf("webhook pool_size must be >= 1")
	}
	if c.PollIntervalMS < 1 {
		return fmt.Errorf("webhook poll_interval_ms must be >= 1")
	}
	if c.LeaseSeconds < 1 {
		return fmt.Errorf("webhook lease_seconds must be >= 1")
	}
	if c.CallTimeoutSec < 1 {
		return fmt.Errorf("webhook call_timeout_sec must be >= 1")
	}
	if c.LeaseSeconds < c.CallTimeoutSec {
		return fmt.Errorf("webhook lease_seconds (%d) must be >= call_timeout_sec (%d)", c.LeaseSeconds, c.CallTimeoutSec)
	}
	if c.MaxAttempts < 1 {
		return fmt.Errorf("webhook max_attempts must be >= 1")
	}
	if c.BaseDelayMS < 1 {
		return fmt.Errorf("webhook base_delay_ms must be >= 1")
	}
	if c.MaxDelayMS < c.BaseDelayMS {
		return fmt.Errorf("webhook max_delay_ms must be >= base_delay_ms")
	}
	j := c.RetryJitter()
	if j < 0 || j > 1 {
		return fmt.Errorf("webhook jitter must be in [0, 1]")
	}
	return nil
}
