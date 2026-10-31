package loadtest

import (
	"context"
	"fmt"
	"time"
)

// CheckFailure is one invariant that did not hold.
type CheckFailure struct {
	Name   string
	Detail string
}

// LatencyStats is authorize latency from updated_at - created_at on AUTHORIZED rows.
// Capture and refund also bump updated_at, so only rows still AUTHORIZED are included.
type LatencyStats struct {
	N   int
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
}

// RunChecks is duplicates, refund overshoots, stuck PENDING, and mix terminal-state rules.
func (s *Store) RunChecks(ctx context.Context, since time.Time, mix bool, maxAttempts int) ([]CheckFailure, error) {
	if maxAttempts < 1 {
		maxAttempts = defaultMaxAttempts
	}
	var out []CheckFailure
	dups, err := s.duplicateKeys(ctx, since)
	if err != nil {
		return nil, err
	}
	out = append(out, dups...)
	overs, err := s.refundOvershoots(ctx, since)
	if err != nil {
		return nil, err
	}
	out = append(out, overs...)
	stuck, err := s.stuckPending(ctx, since)
	if err != nil {
		return nil, err
	}
	if stuck > 0 {
		out = append(out, CheckFailure{
			Name:   "stuck_pending",
			Detail: fmt.Sprintf("%d PENDING row(s) with next_attempt_at more than 60s in the past", stuck),
		})
	}
	if mix {
		mixFails, mixErr := s.mixChecks(ctx, since, maxAttempts)
		if mixErr != nil {
			return nil, mixErr
		}
		out = append(out, mixFails...)
	}
	return out, nil
}

func (s *Store) duplicateKeys(ctx context.Context, since time.Time) ([]CheckFailure, error) {
	const q = `
SELECT merchant_id, idempotency_key, count(*) AS claims, count(DISTINCT payment_id) AS payments
FROM idempotency_keys
WHERE scope = 'payment.create'
  AND created_at >= $1
GROUP BY merchant_id, idempotency_key
HAVING count(*) > 1 OR count(DISTINCT payment_id) > 1`
	rows, err := s.pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("duplicate keys: %w", err)
	}
	defer rows.Close()
	var out []CheckFailure
	for rows.Next() {
		var merchantID int64
		var key string
		var claims, payments int
		if scanErr := rows.Scan(&merchantID, &key, &claims, &payments); scanErr != nil {
			return nil, fmt.Errorf("scan duplicate key: %w", scanErr)
		}
		out = append(out, CheckFailure{
			Name: "duplicate_key",
			Detail: fmt.Sprintf(
				"merchant_id=%d key=%s claims=%d payments=%d",
				merchantID, key, claims, payments,
			),
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("duplicate keys rows: %w", rowsErr)
	}
	return out, nil
}

func (s *Store) refundOvershoots(ctx context.Context, since time.Time) ([]CheckFailure, error) {
	const q = `
SELECT p.public_id, p.amount, COALESCE(SUM(r.amount), 0) AS refunded
FROM payments p
JOIN refunds r ON r.payment_id = p.id AND r.status = 'SUCCEEDED'
WHERE p.created_at >= $1
GROUP BY p.id
HAVING COALESCE(SUM(r.amount), 0) > p.amount`
	rows, err := s.pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("refund overshoots: %w", err)
	}
	defer rows.Close()
	var out []CheckFailure
	for rows.Next() {
		var publicID string
		var amount, refunded int64
		if scanErr := rows.Scan(&publicID, &amount, &refunded); scanErr != nil {
			return nil, fmt.Errorf("scan refund overshoot: %w", scanErr)
		}
		out = append(out, CheckFailure{
			Name:   "refund_overshoot",
			Detail: fmt.Sprintf("payment=%s amount=%d refunded=%d", publicID, amount, refunded),
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("refund overshoots rows: %w", rowsErr)
	}
	return out, nil
}

func (s *Store) stuckPending(ctx context.Context, since time.Time) (int, error) {
	const q = `
SELECT count(*)
FROM payments
WHERE status = 'PENDING'
  AND created_at >= $1
  AND next_attempt_at < now() - make_interval(secs => $2)`
	var n int
	secs := int(stuckPendingGrace.Seconds())
	if err := s.pool.QueryRow(ctx, q, since, secs).Scan(&n); err != nil {
		return 0, fmt.Errorf("stuck pending: %w", err)
	}
	return n, nil
}

func (s *Store) mixChecks(ctx context.Context, since time.Time, maxAttempts int) ([]CheckFailure, error) {
	var pending, over int
	if err := s.pool.QueryRow(ctx, `
SELECT count(*)
FROM payments
WHERE created_at >= $1 AND status = 'PENDING'`, since).Scan(&pending); err != nil {
		return nil, fmt.Errorf("mix pending: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `
SELECT count(*)
FROM payments
WHERE created_at >= $1 AND attempt_count > $2`, since, maxAttempts).Scan(&over); err != nil {
		return nil, fmt.Errorf("mix attempt_count: %w", err)
	}
	var out []CheckFailure
	if pending > 0 {
		out = append(out, CheckFailure{
			Name:   "mix_pending",
			Detail: fmt.Sprintf("%d payment(s) still PENDING after drain", pending),
		})
	}
	if over > 0 {
		out = append(out, CheckFailure{
			Name:   "mix_attempts",
			Detail: fmt.Sprintf("%d payment(s) with attempt_count > %d", over, maxAttempts),
		})
	}
	return out, nil
}

// AuthorizeLatency is p50/p95/p99 of updated_at - created_at for AUTHORIZED rows.
func (s *Store) AuthorizeLatency(ctx context.Context, since time.Time) (LatencyStats, error) {
	const q = `
SELECT
  count(*)::int,
  COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY extract(epoch FROM (updated_at - created_at))), 0),
  COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY extract(epoch FROM (updated_at - created_at))), 0),
  COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY extract(epoch FROM (updated_at - created_at))), 0)
FROM payments
WHERE status = 'AUTHORIZED' AND created_at >= $1`
	var n int
	var p50, p95, p99 float64
	if err := s.pool.QueryRow(ctx, q, since).Scan(&n, &p50, &p95, &p99); err != nil {
		return LatencyStats{}, fmt.Errorf("authorize latency: %w", err)
	}
	return LatencyStats{
		N:   n,
		P50: time.Duration(p50 * float64(time.Second)),
		P95: time.Duration(p95 * float64(time.Second)),
		P99: time.Duration(p99 * float64(time.Second)),
	}, nil
}
