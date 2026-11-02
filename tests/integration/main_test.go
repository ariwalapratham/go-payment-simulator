//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ariwalapratham/go-payment-simulator/internal/config"
	"github.com/ariwalapratham/go-payment-simulator/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

//nolint:gochecknoglobals // populated in TestMain for the integration package
var (
	testPool *pgxpool.Pool
	testCfg  *config.Config
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	_ = godotenv.Load("../../.env", ".env")

	log := zerolog.Nop()
	cfg, err := config.LoadConfig()
	if err != nil {
		return skipOrFail(m, false, "load config: %v", err)
	}

	ctx := context.Background()
	if err := database.Migrate(ctx, &log, cfg); err != nil {
		return skipOrFail(m, true, "%v", err)
	}

	pool, err := pgxpool.New(ctx, database.DSN(cfg))
	if err != nil {
		return skipOrFail(m, true, "%v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return skipOrFail(m, true, "%v", err)
	}

	testCfg = cfg
	testPool = pool
	return m.Run()
}

// skipOrFail skips locally when Postgres is missing. On CI it fails so the
// badge cannot go green with every test skipped.
func skipOrFail(m *testing.M, runAnyway bool, format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "integration tests skipped: "+format+"\n", args...)
	if os.Getenv("CI") != "" {
		return 1
	}
	if runAnyway {
		return m.Run()
	}
	return 0
}
