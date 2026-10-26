package config_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/config"
)

func TestDefaultListConfig(t *testing.T) {
	t.Parallel()

	c := config.DefaultListConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.DefaultLimit != 20 || c.MaxLimit != 100 {
		t.Fatalf("%+v", c)
	}
}

func TestListConfigRejectsDefaultAboveMax(t *testing.T) {
	t.Parallel()

	c := config.ListConfig{DefaultLimit: 50, MaxLimit: 10}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}
