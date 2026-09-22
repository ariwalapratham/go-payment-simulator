package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migpostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"

	"github.com/ariwalapratham/go-payment-simulator/internal/config"
	"github.com/ariwalapratham/go-payment-simulator/migrations"
)

// Migrate applies every pending SQL migration from the embedded migrations
// directory using golang-migrate. It succeeds without changes when the
// schema is already up to date.
func Migrate(ctx context.Context, logger *zerolog.Logger, cfg *config.Config) error {
	db, err := sql.Open("pgx", dsnFromConfig(cfg))
	if err != nil {
		return fmt.Errorf("open migration database connection: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping migration database: %w", err)
	}

	driver, err := migpostgres.WithInstance(db, &migpostgres.Config{})
	if err != nil {
		return fmt.Errorf("constructing postgres migration driver: %w", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("constructing database migrator: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	from := appliedVersion(m)

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info().Msgf("database schema up to date, version %d", from)
			return nil
		}
		return fmt.Errorf("applying database migrations: %w", err)
	}

	logger.Info().Msgf("migrated database schema, from %d to %d", from, appliedVersion(m))
	return nil
}

// appliedVersion returns the current schema version, or 0 when no migration
// has been applied yet.
func appliedVersion(m *migrate.Migrate) uint {
	version, _, err := m.Version()
	if err != nil {
		return 0
	}
	return version
}
