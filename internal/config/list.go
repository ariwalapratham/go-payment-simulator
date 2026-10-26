package config

import "fmt"

const (
	defaultListLimit = 20
	defaultListMax   = 100
)

// ListConfig is GET /v1/payments page size (FR28).
type ListConfig struct {
	DefaultLimit int `koanf:"default_limit"`
	MaxLimit     int `koanf:"max_limit"`
}

// DefaultListConfig is 20 per page, max 100.
func DefaultListConfig() ListConfig {
	return ListConfig{DefaultLimit: defaultListLimit, MaxLimit: defaultListMax}
}

func (c *ListConfig) applyDefaults() {
	d := DefaultListConfig()
	if c.DefaultLimit <= 0 {
		c.DefaultLimit = d.DefaultLimit
	}
	if c.MaxLimit <= 0 {
		c.MaxLimit = d.MaxLimit
	}
}

func (c ListConfig) Validate() error {
	if c.DefaultLimit < 1 {
		return fmt.Errorf("list default_limit must be >= 1")
	}
	if c.MaxLimit < 1 {
		return fmt.Errorf("list max_limit must be >= 1")
	}
	if c.DefaultLimit > c.MaxLimit {
		return fmt.Errorf("list default_limit must be <= max_limit")
	}
	return nil
}
