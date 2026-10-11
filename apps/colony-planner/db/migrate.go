package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// MigrationProvider runs the embedded goose migrations against database. With sessionLock it takes
// Postgres's session lock first, so two processes starting at once can't both migrate. Tests skip the
// lock: each migrates its own schema, and one global lock would only serialize them.
func MigrationProvider(database *sql.DB, sessionLock bool) (*goose.Provider, error) {
	migrationFiles, err := fs.Sub(Migrations, "migrations")
	if err != nil {
		return nil, err
	}
	var providerOptions []goose.ProviderOption
	if sessionLock {
		locker, err := lock.NewPostgresSessionLocker()
		if err != nil {
			return nil, err
		}
		providerOptions = append(providerOptions, goose.WithSessionLocker(locker))
	}
	return goose.NewProvider(goose.DialectPostgres, database, migrationFiles, providerOptions...)
}

// MigrateUp applies every pending migration through the pool, under the session lock.
func MigrateUp(requestContext context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	database := stdlib.OpenDBFromPool(pool)
	defer func() { _ = database.Close() }()
	provider, err := MigrationProvider(database, true)
	if err != nil {
		return err
	}
	results, err := provider.Up(requestContext)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	for _, result := range results {
		logger.Info("migration applied", slog.String("file", result.Source.Path), slog.Duration("duration", result.Duration))
	}
	return nil
}
