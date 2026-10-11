package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	goosedatabase "github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"
)

// ErrUnknownMigrationHistory is CheckMigrationHistory's refusal.
var ErrUnknownMigrationHistory = errors.New("the database records migrations this build doesn't have")

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

// CheckMigrationHistory refuses a database whose goose_db_version records a version that is not among
// the embedded migrations, newer ones included. Goose picks what to apply by version number alone, so on
// such a database (one migrated before M1.5's migrations were squashed into 0001, say) a later migration
// that reuses a recorded version would be skipped without a word. A database without the table is new.
func CheckMigrationHistory(checkContext context.Context, database *sql.DB) error {
	provider, err := MigrationProvider(database, false)
	if err != nil {
		return err
	}
	embeddedVersions := map[int64]bool{}
	var newestEmbeddedVersion int64
	for _, source := range provider.ListSources() {
		embeddedVersions[source.Version] = true
		newestEmbeddedVersion = max(newestEmbeddedVersion, source.Version)
	}

	versionStore, err := goosedatabase.NewStore(goosedatabase.DialectPostgres, goose.DefaultTablename)
	if err != nil {
		return err
	}
	tableChecker, supported := versionStore.(goosedatabase.StoreExtender)
	if !supported {
		return errors.New("check the migration history: goose can't tell whether its table exists")
	}
	tableExists, err := tableChecker.TableExists(checkContext, database)
	if err != nil {
		return fmt.Errorf("check the migration history: %w", err)
	}
	if !tableExists {
		return nil
	}
	recordedMigrations, err := versionStore.ListMigrations(checkContext, database)
	if err != nil {
		return fmt.Errorf("check the migration history: %w", err)
	}
	var unknownVersions []int64
	for _, recorded := range recordedMigrations {
		// Version 0 is the row goose writes when it creates its table.
		if recorded.Version != 0 && !embeddedVersions[recorded.Version] {
			unknownVersions = append(unknownVersions, recorded.Version)
		}
	}
	if len(unknownVersions) == 0 {
		return nil
	}
	slices.Sort(unknownVersions)
	unknownVersions = slices.Compact(unknownVersions)
	versionNames := make([]string, 0, len(unknownVersions))
	for _, version := range unknownVersions {
		versionNames = append(versionNames, strconv.FormatInt(version, 10))
	}
	return fmt.Errorf("%w (%s; the newest here is %d). Migrating it could skip a later migration, so nothing ran: "+
		"run `mise run fresh` to rebuild the local database (it erases the local data)",
		ErrUnknownMigrationHistory, strings.Join(versionNames, ", "), newestEmbeddedVersion)
}

// MigrateUp applies every pending migration through the pool, under the session lock, after
// CheckMigrationHistory.
func MigrateUp(requestContext context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	database := stdlib.OpenDBFromPool(pool)
	defer func() { _ = database.Close() }()
	if err := CheckMigrationHistory(requestContext, database); err != nil {
		return err
	}
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
