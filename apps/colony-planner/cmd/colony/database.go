package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BBruington/colony-planner/db"
)

// serverApplicationName labels the server's connections, so `colony reset` can tell them apart.
const serverApplicationName = "colony serve"

// openDatabase builds the pool. It keeps no idle connections, so a quiet server lets a hosted
// database scale to zero (spec §8.3). applicationName shows in pg_stat_activity.
func openDatabase(commandContext context.Context, databaseURL string, applicationName string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL: %w", err)
	}
	if _, named := poolConfig.ConnConfig.RuntimeParams["application_name"]; !named {
		poolConfig.ConnConfig.RuntimeParams["application_name"] = applicationName
	}
	poolConfig.MaxConns = 5
	poolConfig.MinConns = 0
	poolConfig.MaxConnIdleTime = time.Minute
	pool, err := pgxpool.NewWithConfig(commandContext, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connect to the database: %w", err)
	}
	if err := pool.Ping(commandContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to the database (locally, is it running? try `mise run db:up`): %w", err)
	}
	return pool, nil
}

func migrateUp(commandContext context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	return db.MigrateUp(commandContext, pool, logger)
}
