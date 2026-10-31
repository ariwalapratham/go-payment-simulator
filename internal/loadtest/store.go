package loadtest

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the Postgres side of drain waits and post-run invariant checks.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore opens a small pool against dsn (the API's database, typically localhost:5434).
func NewStore(ctx context.Context, dsn string) (*Store, error) {
	cfg, parseErr := pgxpool.ParseConfig(dsn)
	if parseErr != nil {
		return nil, fmt.Errorf("parse database url: %w", parseErr)
	}
	cfg.MaxConns = defaultStoreConns
	pool, connErr := pgxpool.NewWithConfig(ctx, cfg)
	if connErr != nil {
		return nil, fmt.Errorf("connect database: %w", connErr)
	}
	pingCtx, cancel := context.WithTimeout(ctx, storePingTimeout)
	defer cancel()
	if pingErr := pool.Ping(pingCtx); pingErr != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", pingErr)
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

// WaitDrain blocks until no cohort payment is PENDING and due (next_attempt_at <= now).
// When allPending is true, any remaining PENDING row fails the wait (mix run, after backoff).
func (s *Store) WaitDrain(ctx context.Context, since time.Time, allPending bool) error {
	return pollUntil(ctx, "drain wait", func(ctx context.Context) (bool, error) {
		n, err := s.pendingCount(ctx, since, !allPending)
		if err != nil {
			return false, err
		}
		return n == 0, nil
	})
}

// WaitAuthorized implements PaymentWaiter for a known set of public ids.
func (s *Store) WaitAuthorized(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return pollUntil(ctx, "wait authorized", func(ctx context.Context) (bool, error) {
		n, failed, err := s.authorizedCount(ctx, ids)
		if err != nil {
			return false, err
		}
		if failed > 0 {
			return false, fmt.Errorf("%d payment(s) FAILED (use a 100%% success bank mix)", failed)
		}
		return n == len(ids), nil
	})
}

func pollUntil(ctx context.Context, op string, ready func(context.Context) (bool, error)) error {
	for {
		ok, err := ready(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", op, ctx.Err())
		case <-time.After(defaultDrainPoll):
		}
	}
}

func (s *Store) pendingCount(ctx context.Context, since time.Time, dueOnly bool) (int, error) {
	q := `
SELECT count(*)
FROM payments
WHERE created_at >= $1 AND status = 'PENDING'`
	if dueOnly {
		q += ` AND next_attempt_at <= now()`
	}
	var n int
	if err := s.pool.QueryRow(ctx, q, since).Scan(&n); err != nil {
		return 0, fmt.Errorf("pending count: %w", err)
	}
	return n, nil
}

func (s *Store) authorizedCount(ctx context.Context, ids []uuid.UUID) (int, int, error) {
	const q = `
SELECT
  count(*) FILTER (WHERE status = 'AUTHORIZED'),
  count(*) FILTER (WHERE status = 'FAILED')
FROM payments
WHERE public_id = ANY($1)`
	var authorized, failed int
	if err := s.pool.QueryRow(ctx, q, ids).Scan(&authorized, &failed); err != nil {
		return 0, 0, fmt.Errorf("authorized count: %w", err)
	}
	return authorized, failed, nil
}

// CompletionsAfter is payments that reached AUTHORIZED or FAILED at or after t.
func (s *Store) CompletionsAfter(ctx context.Context, since, t time.Time) (int, error) {
	const q = `
SELECT count(*)
FROM payments
WHERE created_at >= $1
  AND updated_at >= $2
  AND status IN ('AUTHORIZED', 'FAILED')`
	var n int
	if err := s.pool.QueryRow(ctx, q, since, t).Scan(&n); err != nil {
		return 0, fmt.Errorf("completions after: %w", err)
	}
	return n, nil
}

// StatusCounts is payments in the run window grouped by status.
func (s *Store) StatusCounts(ctx context.Context, since time.Time) (map[string]int, error) {
	const q = `
SELECT status, count(*)
FROM payments
WHERE created_at >= $1
GROUP BY status`
	rows, err := s.pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("status counts: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var status string
		var n int
		if scanErr := rows.Scan(&status, &n); scanErr != nil {
			return nil, fmt.Errorf("scan status count: %w", scanErr)
		}
		out[status] = n
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("status counts rows: %w", rowsErr)
	}
	return out, nil
}
