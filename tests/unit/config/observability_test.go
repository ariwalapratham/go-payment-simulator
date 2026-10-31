package config_test

import (
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/config"
)

func TestEnvKeyTransformObservability(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"PAYMENTS_OBSERVABILITY_SERVICE_NAME":             "observability.service_name",
		"PAYMENTS_OBSERVABILITY_NEW_RELIC_LICENSE_KEY":    "observability.new_relic.license_key",
		"PAYMENTS_OBSERVABILITY_LOGGING_LEVEL":            "observability.logging.level",
		"PAYMENTS_OBSERVABILITY_HEALTH_CHECKS_INTERVAL":   "observability.health_checks.interval",
	}
	for env, want := range cases {
		if got := config.EnvKeyTransform(env); got != want {
			t.Errorf("%s: got %q want %q", env, got, want)
		}
	}
}

func TestApplyObservabilityDefaultsLicenseKeyOnly(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Primary: config.Primary{Env: "development"},
		Observability: &config.ObservabilityConfig{
			NewRelic: config.NewRelicConfig{LicenseKey: "test-key"},
		},
	}
	cfg.ApplyObservabilityDefaults()

	if cfg.Observability.ServiceName != "payment-simulator" {
		t.Fatalf("service_name %q", cfg.Observability.ServiceName)
	}
	if cfg.Observability.Logging.Level != "info" {
		t.Fatalf("logging level %q", cfg.Observability.Logging.Level)
	}
	if cfg.Observability.NewRelic.LicenseKey != "test-key" {
		t.Fatalf("license key %q", cfg.Observability.NewRelic.LicenseKey)
	}
}
