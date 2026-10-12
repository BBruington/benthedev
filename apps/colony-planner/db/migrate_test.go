package db_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/BBruington/colony-planner/db"
	"github.com/BBruington/colony-planner/db/dbtest"
)

func checkMigrationHistory(t *testing.T, database *dbtest.Database) error {
	t.Helper()
	connection := stdlib.OpenDBFromPool(database.Pool)
	defer func() { _ = connection.Close() }()
	return db.CheckMigrationHistory(context.Background(), connection)
}

func newestEmbeddedVersion(t *testing.T, database *dbtest.Database) int64 {
	t.Helper()
	connection := stdlib.OpenDBFromPool(database.Pool)
	defer func() { _ = connection.Close() }()
	provider, err := db.MigrationProvider(connection, false)
	if err != nil {
		t.Fatal(err)
	}
	sources := provider.ListSources()
	return sources[len(sources)-1].Version
}

// The author's database was migrated before M1.5's migrations were squashed into 0001, so it records
// versions 2 and 3, which no longer exist. Goose would treat a later 0002 as applied and skip it, so
// serve, migrate up and seed (each through db.MigrateUp) refuse a database that records a version this
// build doesn't have, and say how to rebuild it. reset never meets the refusal: it drops the schema
// before it migrates, so `mise run reset` and `fresh` rebuild such a database.
func TestUnknownMigrationHistoryIsRefused(t *testing.T) {
	testCases := map[string]func(newest int64) []int64{
		"the next versions":   func(newest int64) []int64 { return []int64{newest + 2, newest + 1} },
		"a far newer version": func(int64) []int64 { return []int64{20991231000000} },
	}
	for name, recordedVersions := range testCases {
		t.Run(name, func(t *testing.T) {
			testContext := context.Background()
			database := dbtest.New(t)
			if err := checkMigrationHistory(t, database); err != nil {
				t.Fatalf("a freshly migrated database was refused: %v", err)
			}
			newest := newestEmbeddedVersion(t, database)
			unknownVersions := recordedVersions(newest)
			slices.Sort(unknownVersions)
			versionNames := make([]string, 0, len(unknownVersions))
			for _, version := range unknownVersions {
				versionNames = append(versionNames, strconv.FormatInt(version, 10))
			}
			wantMentions := []string{
				"(" + strings.Join(versionNames, ", ") + ";",
				"the newest here is " + strconv.FormatInt(newest, 10),
				"mise run fresh",
			}
			for _, version := range unknownVersions {
				if _, err := database.Pool.Exec(testContext,
					`INSERT INTO goose_db_version (version_id, is_applied) VALUES ($1, true)`, version); err != nil {
					t.Fatalf("record version %d: %v", version, err)
				}
			}
			recordedBefore := countRows(t, database, `SELECT count(*) FROM goose_db_version`)

			err := checkMigrationHistory(t, database)
			if !errors.Is(err, db.ErrUnknownMigrationHistory) {
				t.Fatalf("CheckMigrationHistory = %v; want ErrUnknownMigrationHistory", err)
			}
			for _, instruction := range wantMentions {
				if !strings.Contains(err.Error(), instruction) {
					t.Errorf("the refusal doesn't mention %q: %v", instruction, err)
				}
			}
			if err := db.MigrateUp(testContext, database.Pool, slog.New(slog.DiscardHandler)); !errors.Is(err, db.ErrUnknownMigrationHistory) {
				t.Fatalf("MigrateUp = %v; want ErrUnknownMigrationHistory", err)
			}
			if recordedAfter := countRows(t, database, `SELECT count(*) FROM goose_db_version`); recordedAfter != recordedBefore {
				t.Fatalf("the refused MigrateUp changed the history: %d rows, then %d", recordedBefore, recordedAfter)
			}

			// What `colony reset` does: drop the schema, then MigrateUp.
			var schemaName string
			if err := database.Pool.QueryRow(testContext, `SELECT current_schema()`).Scan(&schemaName); err != nil {
				t.Fatal(err)
			}
			quotedSchema := pgx.Identifier{schemaName}.Sanitize()
			if _, err := database.Pool.Exec(testContext, "DROP SCHEMA "+quotedSchema+" CASCADE; CREATE SCHEMA "+quotedSchema); err != nil {
				t.Fatalf("drop the schema: %v", err)
			}
			// Statements the pool prepared against the dropped tables go with their connections.
			database.Pool.Reset()
			if err := checkMigrationHistory(t, database); err != nil {
				t.Fatalf("a database without goose's table was refused: %v", err)
			}
			if err := db.MigrateUp(testContext, database.Pool, slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("MigrateUp after the reset: %v", err)
			}
			if err := checkMigrationHistory(t, database); err != nil {
				t.Fatalf("the rebuilt database was refused: %v", err)
			}
		})
	}
}
