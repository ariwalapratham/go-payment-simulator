package config_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/config"
)

func TestDefaultWorkerConfig(t *testing.T) {
	t.Parallel()

	c := config.DefaultWorkerConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.PoolSize < 1 || c.MaxAttempts < 1 {
		t.Fatalf("%+v", c)
	}
	if c.RetryJitter() < 0 || c.RetryJitter() > 1 {
		t.Fatalf("jitter %v", c.RetryJitter())
	}
}

func TestWorkerConfigRejectsBadJitter(t *testing.T) {
	t.Parallel()

	c := config.DefaultWorkerConfig()
	j := 1.5
	c.Jitter = &j
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkerConfigRejectsLeaseShorterThanBankTimeout(t *testing.T) {
	t.Parallel()

	c := config.DefaultWorkerConfig()
	c.LeaseSeconds = 1
	if err := c.ValidateLease(30); err == nil {
		t.Fatal("expected error")
	}
}
