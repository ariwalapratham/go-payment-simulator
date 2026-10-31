// Command loadtest paces payment creates and refund bursts against a running API.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/loadtest"
)

const (
	defaultBaseURLs = "http://localhost:8080"
	//nolint:gosec // G101: local default DSN, not a production secret
	defaultDatabaseURL  = "postgres://postgres:postgres@localhost:5434/payments?sslmode=disable"
	defaultRate         = 15
	defaultMixRate      = 5
	defaultStormKeys    = 20
	defaultDBPool       = 25
	defaultWorkerPool   = 4
	defaultBankDelayMS  = 200
	defaultPayments     = 10
	defaultRefundsPer   = 8
	defaultMaxAttempts  = 5
	defaultHTTPTimeout  = 15 * time.Second
	defaultDrainTimeout = 10 * time.Minute
	exitUsage           = 2
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 1 {
		usage()
		return exitUsage
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch args[0] {
	case "create":
		err = runCreate(ctx, args[1:])
	case "storm":
		err = runStorm(ctx, args[1:])
	case "mix":
		err = runMix(ctx, args[1:])
	case "refund":
		err = runRefund(ctx, args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		usage()
		return exitUsage
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "loadtest: %v\n", err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: loadtest <create|storm|mix|refund> [flags]

The API must already be running. Pass API_KEY for a merchant without webhook_url.
Create/storm/refund assume a pinned bank (100%% success, 200ms delay) unless you
override --bank-mix / --bank-delay-ms to match the process you started.
Mix assumes the default simulator weights 90/5/3/2.

`)
}

type commonFlags struct {
	apiKey       string
	baseURLs     string
	databaseURL  string
	timeout      time.Duration
	drainTimeout time.Duration
	dbPool       int
	workerPool   int
	bankDelayMS  int
	bankMix      string
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func registerCommon(fs *flag.FlagSet, c *commonFlags, mix bool) {
	fs.StringVar(&c.apiKey, "api-key", os.Getenv("API_KEY"), "merchant API key (or API_KEY)")
	fs.StringVar(&c.baseURLs, "base-urls", envOr("BASE_URLS", defaultBaseURLs), "comma-separated API origins")
	fs.StringVar(&c.databaseURL, "database-url", envOr("DATABASE_URL", defaultDatabaseURL), "Postgres URL")
	fs.DurationVar(&c.timeout, "timeout", defaultHTTPTimeout, "HTTP client timeout")
	fs.DurationVar(&c.drainTimeout, "drain-timeout", defaultDrainTimeout, "max wait for PENDING drain")
	fs.IntVar(&c.dbPool, "db-pool", defaultDBPool, "PAYMENTS_DATABASE_MAX_OPEN_CONNS per API process (label)")
	fs.IntVar(&c.workerPool, "worker-pool", defaultWorkerPool, "PAYMENTS_WORKER_POOL_SIZE per API process (label)")
	delay := defaultBankDelayMS
	mixLabel := "pinned 100/0/0/0"
	if mix {
		delay = 0
		mixLabel = "default 90/5/3/2"
	}
	fs.IntVar(&c.bankDelayMS, "bank-delay-ms", delay, "simulator min/max delay (label)")
	fs.StringVar(&c.bankMix, "bank-mix", mixLabel, "simulator outcome mix (label)")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runCreate(ctx context.Context, args []string) error {
	fs := newFlagSet("create")
	var c commonFlags
	registerCommon(fs, &c, false)
	rate := fs.Int("rate", defaultRate, "creates per second")
	duration := fs.Duration("duration", time.Minute, "attack duration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return loadtest.Run(ctx, os.Stdout, pacedConfig("create", c, *rate, *duration, loadtest.KeyUnique, false, 0))
}

func runStorm(ctx context.Context, args []string) error {
	fs := newFlagSet("storm")
	var c commonFlags
	registerCommon(fs, &c, false)
	rate := fs.Int("rate", defaultRate, "creates per second")
	duration := fs.Duration("duration", time.Minute, "attack duration")
	keys := fs.Int("storm-keys", defaultStormKeys, "shared idempotency keys")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return loadtest.Run(ctx, os.Stdout, pacedConfig("storm", c, *rate, *duration, loadtest.KeyStorm, false, *keys))
}

func runMix(ctx context.Context, args []string) error {
	fs := newFlagSet("mix")
	var c commonFlags
	registerCommon(fs, &c, true)
	rate := fs.Int("rate", defaultMixRate, "creates per second")
	duration := fs.Duration("duration", time.Minute, "attack duration")
	maxAttempts := fs.Int("max-attempts", defaultMaxAttempts, "PAYMENTS_WORKER_MAX_ATTEMPTS")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := pacedConfig("mix", c, *rate, *duration, loadtest.KeyUnique, true, 0)
	cfg.MaxAttempts = *maxAttempts
	return loadtest.Run(ctx, os.Stdout, cfg)
}

func runRefund(ctx context.Context, args []string) error {
	fs := newFlagSet("refund")
	var c commonFlags
	registerCommon(fs, &c, false)
	payments := fs.Int("payments", defaultPayments, "payments to capture then refund")
	per := fs.Int("refunds-per", defaultRefundsPer, "concurrent refunds per payment")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := pacedConfig("refund", c, 0, 0, loadtest.KeyUnique, false, 0)
	cfg.Refund = loadtest.RefundConfig{Payments: *payments, RefundsPer: *per}
	return loadtest.Run(ctx, os.Stdout, cfg)
}

func pacedConfig(
	scenario string,
	c commonFlags,
	rate int,
	duration time.Duration,
	mode loadtest.KeyMode,
	mix bool,
	stormKeys int,
) loadtest.RunConfig {
	return loadtest.RunConfig{
		Scenario:     scenario,
		BaseURLs:     loadtest.ParseBaseURLs(c.baseURLs),
		APIKey:       c.apiKey,
		DatabaseURL:  c.databaseURL,
		Rate:         rate,
		Duration:     duration,
		Timeout:      c.timeout,
		DrainTimeout: c.drainTimeout,
		Mode:         mode,
		StormKeys:    stormKeys,
		Mix:          mix,
		MaxAttempts:  defaultMaxAttempts,
		Env: loadtest.EnvLabels{
			DBPoolPerProcess:     c.dbPool,
			WorkerPoolPerProcess: c.workerPool,
			BankDelayMS:          c.bankDelayMS,
			BankMix:              c.bankMix,
		},
	}
}
