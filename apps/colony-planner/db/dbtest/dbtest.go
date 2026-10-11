// Package dbtest gives each test its own freshly migrated schema in the TEST_DATABASE_URL database
// (the compose `colony_test` database, or CI's service container). Tests never touch DATABASE_URL.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/BBruington/colony-planner/db"
)

// QueryCounter counts the queries a pool runs, so a test can prove that a path touches no database.
type QueryCounter struct {
	queries atomic.Int64
}

func (counter *QueryCounter) TraceQueryStart(queryContext context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	counter.queries.Add(1)
	return queryContext
}

func (counter *QueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (counter *QueryCounter) Count() int64 {
	return counter.queries.Load()
}

func (counter *QueryCounter) Reset() {
	counter.queries.Store(0)
}

type Database struct {
	Pool    *pgxpool.Pool
	Queries *QueryCounter
	// URL reaches the same schema, for code that opens its own pool (`colony seed`, for example).
	URL string
}

// New creates a schema named after a random suffix, migrates it, and drops it when the test ends.
func New(t testing.TB) *Database {
	t.Helper()
	return newDatabase(t, -1)
}

// NewAtVersion is New stopped after migration version, so a migration test can insert rows in the
// old shape and then apply the next migration with MigrateTo.
func NewAtVersion(t testing.TB, version int64) *Database {
	t.Helper()
	return newDatabase(t, version)
}

// MigrateTo applies the migrations up to and including version.
func (database *Database) MigrateTo(migrateContext context.Context, version int64) error {
	connection := stdlib.OpenDBFromPool(database.Pool)
	defer func() { _ = connection.Close() }()
	provider, err := db.MigrationProvider(connection, false)
	if err != nil {
		return err
	}
	_, err = provider.UpTo(migrateContext, version)
	return err
}

// Version is the newest applied migration.
func (database *Database) Version(versionContext context.Context) (int64, error) {
	connection := stdlib.OpenDBFromPool(database.Pool)
	defer func() { _ = connection.Close() }()
	provider, err := db.MigrationProvider(connection, false)
	if err != nil {
		return 0, err
	}
	return provider.GetDBVersion(versionContext)
}

func newDatabase(t testing.TB, version int64) *Database {
	t.Helper()
	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		t.Fatal("TEST_DATABASE_URL is not set; run the tests with `mise run test`, which loads .env")
	}
	if err := db.RequireLocalHost(baseURL); err != nil {
		t.Fatalf("TEST_DATABASE_URL: %v", err)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL is not a URL: %v", err)
	}

	setupContext, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	schemaName := "test_" + hex.EncodeToString(suffix)

	administration, err := pgx.Connect(setupContext, baseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL (is the database running? try `mise run db:up`): %v", err)
	}
	if _, err := administration.Exec(setupContext, "CREATE SCHEMA "+schemaName); err != nil {
		_ = administration.Close(setupContext)
		t.Fatalf("create test schema: %v", err)
	}
	_ = administration.Close(setupContext)

	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	schemaURL := parsed.String()

	counter := &QueryCounter{}
	poolConfig, err := pgxpool.ParseConfig(schemaURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	poolConfig.MaxConns = 6
	poolConfig.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(setupContext, poolConfig)
	if err != nil {
		t.Fatalf("open the test pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		dropContext, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()
		connection, err := pgx.Connect(dropContext, baseURL)
		if err != nil {
			t.Errorf("drop test schema %s: %v", schemaName, err)
			return
		}
		defer func() { _ = connection.Close(dropContext) }()
		if _, err := connection.Exec(dropContext, "DROP SCHEMA "+schemaName+" CASCADE"); err != nil {
			t.Errorf("drop test schema %s: %v", schemaName, err)
		}
	})

	database := stdlib.OpenDBFromPool(pool)
	provider, err := db.MigrationProvider(database, false)
	if err != nil {
		t.Fatalf("prepare migrations: %v", err)
	}
	if version < 0 {
		_, err = provider.Up(setupContext)
	} else {
		_, err = provider.UpTo(setupContext, version)
	}
	if err != nil {
		t.Fatalf("migrate the test schema: %v", err)
	}
	_ = database.Close()
	counter.Reset()
	return &Database{Pool: pool, Queries: counter, URL: schemaURL}
}
