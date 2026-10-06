package config

import "fmt"

const (
	defaultWebhookPoolSize       = 4
	defaultWebhookPollIntervalMS = 200
	defaultWebhookLeaseSeconds   = 45
	defaultWebhookCallTimeoutSec = 30
)

// WebhookConfig drives the outbound delivery worker pool.
type WebhookConfig struct {
	PoolSize       int `koanf:"pool_size"`
	PollIntervalMS int `koanf:"poll_interval_ms"`
	LeaseSeconds   int `koanf:"lease_seconds"`
	CallTimeoutSec int `koanf:"call_timeout_sec"`
}

// DefaultWebhookConfig matches payment worker defaults for local dev.
func DefaultWebhookConfig() WebhookConfig {
	return WebhookConfig{
		PoolSize:       defaultWebhookPoolSize,
		PollIntervalMS: defaultWebhookPollIntervalMS,
		LeaseSeconds:   defaultWebhookLeaseSeconds,
		CallTimeoutSec: defaultWebhookCallTimeoutSec,
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
	return nil
}
