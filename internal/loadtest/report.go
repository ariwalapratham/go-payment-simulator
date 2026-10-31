package loadtest

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EnvLabels are the running API's pool and bank settings, supplied by the operator.
type EnvLabels struct {
	DBPoolPerProcess     int
	WorkerPoolPerProcess int
	BankDelayMS          int
	BankMix              string
	APIProcesses         int
}

// Report is one load-test run: HTTP, worker drain, SQL latency, and invariant checks.
type Report struct {
	Scenario          string
	Machine           Machine
	Env               EnvLabels
	Attack            *AttackStats
	RefundCalls       int
	PendingAtEnd      int
	Drain             time.Duration
	WorkerCompletions int
	WorkerThroughput  float64
	AuthLatency       LatencyStats
	StatusCounts      map[string]int
	Checks            []CheckFailure
}

// Write prints a single-run text report.
func (r Report) Write(w io.Writer) error {
	if err := writeHeader(w, r); err != nil {
		return err
	}
	if r.Attack != nil {
		if err := writeAttack(w, *r.Attack); err != nil {
			return err
		}
	}
	if r.RefundCalls > 0 {
		if _, err := fmt.Fprintf(w, "\nrefund_calls=%d\n", r.RefundCalls); err != nil {
			return err
		}
	}
	if err := writeWorker(w, r); err != nil {
		return err
	}
	if err := writeLatency(w, r.AuthLatency); err != nil {
		return err
	}
	if err := writeCounts(w, r.StatusCounts); err != nil {
		return err
	}
	return writeChecks(w, r.Checks)
}

func writeHeader(w io.Writer, r Report) error {
	procs := r.Env.APIProcesses
	if procs < 1 {
		procs = 1
	}
	poolNote := strconv.Itoa(r.Env.DBPoolPerProcess)
	workerNote := strconv.Itoa(r.Env.WorkerPoolPerProcess)
	if procs > 1 {
		poolNote = fmt.Sprintf("%d×%d", r.Env.DBPoolPerProcess, procs)
		workerNote = fmt.Sprintf("%d×%d", r.Env.WorkerPoolPerProcess, procs)
	}
	if _, err := fmt.Fprintf(w, "=== loadtest %s ===\n", r.Scenario); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "machine: %s\n", r.Machine); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w,
		"api_processes=%d db_pool=%s worker_pool=%s bank_delay_ms=%d bank_mix=%s\n",
		procs, poolNote, workerNote, r.Env.BankDelayMS, r.Env.BankMix,
	)
	return err
}

func writeAttack(w io.Writer, a AttackStats) error {
	if _, err := fmt.Fprintf(w, "\ncreate_api:\n  requests=%d rate=%.1f/s success=%.3f\n",
		a.Requests, a.Rate, a.Success); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  status: %s\n", formatCounts(a.StatusCodes)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "  latency: p50=%s p95=%s p99=%s\n",
		a.P50.Round(time.Millisecond), a.P95.Round(time.Millisecond), a.P99.Round(time.Millisecond))
	return err
}

func writeWorker(w io.Writer, r Report) error {
	if _, err := fmt.Fprintf(w, "\nworker:\n  pending_at_attack_end=%d drain=%s completions=%d",
		r.PendingAtEnd, r.Drain.Round(time.Millisecond), r.WorkerCompletions); err != nil {
		return err
	}
	if r.Drain > 0 && r.WorkerCompletions > 0 {
		if _, err := fmt.Fprintf(w, " rate=%.1f/s", r.WorkerThroughput); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeLatency(w io.Writer, lat LatencyStats) error {
	if _, err := fmt.Fprintf(w, "\nauthorize_latency (AUTHORIZED updated_at-created_at):\n  n=%d", lat.N); err != nil {
		return err
	}
	if lat.N == 0 {
		_, err := fmt.Fprintln(w)
		return err
	}
	_, err := fmt.Fprintf(w, " p50=%s p95=%s p99=%s\n",
		lat.P50.Round(time.Millisecond), lat.P95.Round(time.Millisecond), lat.P99.Round(time.Millisecond))
	return err
}

func writeCounts(w io.Writer, counts map[string]int) error {
	if len(counts) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "payments by status: %s\n", formatCounts(counts))
	return err
}

func writeChecks(w io.Writer, checks []CheckFailure) error {
	if len(checks) == 0 {
		_, err := fmt.Fprintln(w, "checks: ok")
		return err
	}
	if _, err := fmt.Fprintf(w, "checks: FAIL (%d)\n", len(checks)); err != nil {
		return err
	}
	for _, c := range checks {
		if _, err := fmt.Fprintf(w, "  - %s: %s\n", c.Name, c.Detail); err != nil {
			return err
		}
	}
	return nil
}

func formatCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}
