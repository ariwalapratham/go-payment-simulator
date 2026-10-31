package loadtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

const refundScenario = "refund"

// RunConfig is one CLI scenario: paced create, storm, mix, or refund burst.
type RunConfig struct {
	Scenario     string
	BaseURLs     []string
	APIKey       string
	DatabaseURL  string
	Rate         int
	Duration     time.Duration
	Timeout      time.Duration
	DrainTimeout time.Duration
	Mode         KeyMode
	StormKeys    int
	Mix          bool
	MaxAttempts  int
	Env          EnvLabels
	Refund       RefundConfig
}

// Run executes the scenario, prints a report to w, and returns a non-nil error
// when HTTP, drain, or SQL checks fail.
func Run(ctx context.Context, w io.Writer, cfg RunConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	store, storeErr := NewStore(ctx, cfg.DatabaseURL)
	if storeErr != nil {
		return storeErr
	}
	defer store.Close()

	rep, since, attackEnd, runErr := executeScenario(ctx, store, cfg)
	if runErr != nil {
		_ = rep.Write(w)
		return runErr
	}
	if drainErr := drainAndCheck(ctx, w, store, &rep, since, attackEnd, cfg); drainErr != nil {
		return drainErr
	}
	if writeErr := rep.Write(w); writeErr != nil {
		return writeErr
	}
	if len(rep.Checks) > 0 {
		return fmt.Errorf("%d check(s) failed", len(rep.Checks))
	}
	return nil
}

func executeScenario(ctx context.Context, store *Store, cfg RunConfig) (Report, time.Time, time.Time, error) {
	rep := Report{
		Scenario: cfg.Scenario,
		Machine:  ReadMachine(),
		Env:      cfg.labeledEnv(),
	}
	since := time.Now().UTC()
	if cfg.Scenario == refundScenario {
		calls, err := runRefund(ctx, store, cfg)
		if err != nil {
			return rep, since, since, err
		}
		rep.RefundCalls = len(calls)
		return rep, since, time.Now().UTC(), nil
	}
	stats, err := runAttack(ctx, cfg)
	if err != nil {
		return rep, since, since, err
	}
	rep.Attack = &stats
	if httpErr := CheckAttackHTTP(stats, cfg.Mode); httpErr != nil {
		return rep, stats.StartedAt.UTC(), stats.EndedAt.UTC(), httpErr
	}
	since = stats.StartedAt.UTC()
	pending, err := store.pendingCount(ctx, since, true)
	if err != nil {
		return rep, since, stats.EndedAt.UTC(), err
	}
	rep.PendingAtEnd = pending
	return rep, since, stats.EndedAt.UTC(), nil
}

func drainAndCheck(
	ctx context.Context,
	w io.Writer,
	store *Store,
	rep *Report,
	since, attackEnd time.Time,
	cfg RunConfig,
) error {
	drainCtx, cancel := context.WithTimeout(ctx, cfg.drainTimeoutFor(rep.PendingAtEnd))
	defer cancel()
	if err := store.WaitDrain(drainCtx, since, cfg.Mix); err != nil {
		left := -1
		if n, nErr := store.pendingCount(ctx, since, !cfg.Mix); nErr == nil {
			left = n
			_, _ = fmt.Fprintf(w,
				"drain timed out with %d PENDING remaining (queue still draining; not necessarily a product bug)\n",
				left,
			)
		}
		counts, countErr := store.StatusCounts(ctx, since)
		if countErr == nil {
			rep.StatusCounts = counts
		}
		_ = rep.Write(w)
		if left >= 0 {
			return fmt.Errorf("drain timed out with %d PENDING remaining: %w", left, err)
		}
		return err
	}
	rep.Drain = time.Since(attackEnd)
	n, err := store.CompletionsAfter(ctx, since, attackEnd)
	if err != nil {
		return err
	}
	rep.WorkerCompletions = n
	if rep.Drain > 0 {
		rep.WorkerThroughput = float64(n) / rep.Drain.Seconds()
	}
	return fillSQL(ctx, store, rep, since, cfg)
}

func (c RunConfig) validate() error {
	if c.APIKey == "" {
		return errors.New("api key is required")
	}
	if c.DatabaseURL == "" {
		return errors.New("database url is required")
	}
	if len(normalizeURLs(c.BaseURLs)) == 0 {
		return errors.New("at least one base url is required")
	}
	return nil
}

func (c RunConfig) labeledEnv() EnvLabels {
	e := c.Env
	e.APIProcesses = len(normalizeURLs(c.BaseURLs))
	if e.APIProcesses < 1 {
		e.APIProcesses = 1
	}
	if e.DBPoolPerProcess < 1 {
		e.DBPoolPerProcess = defaultDBPoolLabel
	}
	if e.WorkerPoolPerProcess < 1 {
		e.WorkerPoolPerProcess = defaultWorkerLabel
	}
	if e.BankMix == "" {
		e.BankMix = "pinned 100/0/0/0"
	}
	return e
}

func (c RunConfig) drainTimeout() time.Duration {
	if c.DrainTimeout > 0 {
		return c.DrainTimeout
	}
	return defaultDrainTimeout
}

// drainTimeoutFor extends the configured wait when remaining PENDING cannot
// drain in 10m (e.g. 200 RPS vs 4 workers at 200ms bank).
func (c RunConfig) drainTimeoutFor(pending int) time.Duration {
	base := c.drainTimeout()
	extra := c.estimateDrain(pending)
	if extra > base {
		return extra
	}
	return base
}

func (c RunConfig) estimateDrain(pending int) time.Duration {
	if pending < 1 {
		return 0
	}
	delayMS := c.Env.BankDelayMS
	if delayMS < 1 {
		return 0
	}
	workers := c.Env.WorkerPoolPerProcess
	if workers < 1 {
		workers = defaultWorkerLabel
	}
	procs := len(normalizeURLs(c.BaseURLs))
	if procs < 1 {
		procs = 1
	}
	delay := time.Duration(delayMS) * time.Millisecond
	authsPerSec := float64(workers*procs) / delay.Seconds()
	if authsPerSec < 1 {
		authsPerSec = 1
	}
	secs := float64(pending)/authsPerSec + drainSlack.Seconds()
	return time.Duration(secs * float64(time.Second))
}

func runAttack(ctx context.Context, cfg RunConfig) (AttackStats, error) {
	tr, err := PaymentTargeter(TargetConfig{
		BaseURLs:    cfg.BaseURLs,
		APIKey:      cfg.APIKey,
		Mode:        cfg.Mode,
		StormKeys:   cfg.StormKeys,
		StormPrefix: "storm-" + uuid.NewString(),
	})
	if err != nil {
		return AttackStats{}, err
	}
	return Attack(ctx, tr, AttackConfig{
		Rate:     cfg.Rate,
		Duration: cfg.Duration,
		Timeout:  cfg.Timeout,
		Name:     cfg.Scenario,
	})
}

func runRefund(ctx context.Context, store *Store, cfg RunConfig) ([]RefundCall, error) {
	urls := normalizeURLs(cfg.BaseURLs)
	api, err := NewClient(urls[0], cfg.APIKey, cfg.Timeout, minIdleConns)
	if err != nil {
		return nil, err
	}
	refundCfg := cfg.Refund
	refundCfg.WaitTimeout = cfg.drainTimeout()
	return RefundBurst(ctx, api, store, refundCfg)
}

func fillSQL(ctx context.Context, store *Store, rep *Report, since time.Time, cfg RunConfig) error {
	lat, err := store.AuthorizeLatency(ctx, since)
	if err != nil {
		return err
	}
	rep.AuthLatency = lat
	counts, err := store.StatusCounts(ctx, since)
	if err != nil {
		return err
	}
	rep.StatusCounts = counts
	checks, err := store.RunChecks(ctx, since, cfg.Mix, cfg.MaxAttempts)
	if err != nil {
		return err
	}
	rep.Checks = checks
	return nil
}
