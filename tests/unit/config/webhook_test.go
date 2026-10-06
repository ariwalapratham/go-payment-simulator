package config_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/config"
)

func TestDefaultWebhookConfig(t *testing.T) {
	t.Parallel()

	c := config.DefaultWebhookConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.MaxAttempts < 1 || c.BaseDelayMS < 1 {
		t.Fatalf("%+v", c)
	}
}
