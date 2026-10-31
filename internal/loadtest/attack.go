package loadtest

import (
	"context"
	"errors"
	"fmt"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

// AttackConfig is an open-loop create attack. Requests are paced even when earlier
// responses are still outstanding.
type AttackConfig struct {
	Rate     int
	Duration time.Duration
	Timeout  time.Duration
	Name     string
}

// AttackStats is the HTTP side of a paced create run.
type AttackStats struct {
	StartedAt   time.Time
	EndedAt     time.Time
	Requests    uint64
	Rate        float64
	StatusCodes map[string]int
	P50         time.Duration
	P95         time.Duration
	P99         time.Duration
	Success     float64
}

// Attack runs Vegeta against tr until Duration elapses or ctx is cancelled.
func Attack(ctx context.Context, tr vegeta.Targeter, cfg AttackConfig) (AttackStats, error) {
	if tr == nil {
		return AttackStats{}, errors.New("targeter is required")
	}
	if cfg.Rate < 1 {
		return AttackStats{}, errors.New("rate must be >= 1")
	}
	if cfg.Duration <= 0 {
		return AttackStats{}, errors.New("duration must be > 0")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultClientTimeout
	}
	name := cfg.Name
	if name == "" {
		name = "create"
	}

	conns, workers, maxWorkers := attackPool(cfg.Rate)
	attacker := vegeta.NewAttacker(
		vegeta.Timeout(timeout),
		vegeta.Connections(conns),
		vegeta.Workers(workers),
		vegeta.MaxWorkers(maxWorkers),
		vegeta.KeepAlive(true),
	)

	started := time.Now()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			attacker.Stop()
		case <-done:
		}
	}()

	var metrics vegeta.Metrics
	for res := range attacker.Attack(tr, vegeta.Rate{Freq: cfg.Rate, Per: time.Second}, cfg.Duration, name) {
		metrics.Add(res)
	}
	close(done)
	metrics.Close()
	ended := time.Now()

	codes := make(map[string]int, len(metrics.StatusCodes))
	for k, v := range metrics.StatusCodes {
		codes[k] = v
	}
	stats := AttackStats{
		StartedAt:   started,
		EndedAt:     ended,
		Requests:    metrics.Requests,
		Rate:        metrics.Rate,
		StatusCodes: codes,
		P50:         metrics.Latencies.P50,
		P95:         metrics.Latencies.P95,
		P99:         metrics.Latencies.P99,
		Success:     metrics.Success,
	}
	if err := ctx.Err(); err != nil {
		return stats, fmt.Errorf("attack cancelled: %w", err)
	}
	return stats, nil
}

// CheckAttackHTTP fails unique-key creates with no 201s, or a 5xx share above 1%.
func CheckAttackHTTP(stats AttackStats, mode KeyMode) error {
	if stats.Requests == 0 {
		return errors.New("no HTTP requests completed")
	}
	five := countStatusPrefix(stats.StatusCodes, '5')
	if stats.Requests > 0 && float64(five)/float64(stats.Requests)*100 > float64(max5xxPercent) {
		return fmt.Errorf("HTTP 5xx share too high: %d/%d", five, stats.Requests)
	}
	created := stats.StatusCodes["201"]
	ok := stats.StatusCodes["200"]
	if created == 0 && ok == 0 {
		return fmt.Errorf("no HTTP 2xx from create (status %v)", stats.StatusCodes)
	}
	if mode == KeyUnique && created == 0 {
		return fmt.Errorf("unique-key create got 0 HTTP 201s (status %v)", stats.StatusCodes)
	}
	return nil
}

func countStatusPrefix(codes map[string]int, prefix byte) int {
	n := 0
	for k, v := range codes {
		if len(k) > 0 && k[0] == prefix {
			n += v
		}
	}
	return n
}

func attackPool(rate int) (int, uint64, uint64) {
	conns := max(rate*idleConnMultiplier, minIdleConns)
	workers := uint64(max(minWorkers, min(rate, maxInitialWorkers)))
	maxWorkers := uint64(max(minMaxWorkers, rate*maxWorkerMultiplier))
	return conns, workers, maxWorkers
}
