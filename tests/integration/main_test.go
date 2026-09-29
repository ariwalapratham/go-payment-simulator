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
		fmt.Fprintf(os.Stderr, "integration tests skipped: load config: %v\n", err)
		return 0
	}

	ctx := context.Background()
	if err := database.Migrate(ctx, &log, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "integration tests skipped: %v\n", err)
		return m.Run()
	}

	pool, err := pgxpool.New(ctx, database.DSN(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests skipped: %v\n", err)
		return m.Run()
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "integration tests skipped: %v\n", err)
		return m.Run()
	}

	testCfg = cfg
	testPool = pool
	return m.Run()
}
