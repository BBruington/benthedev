package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/BBruington/colony-planner/db"
)

func migrate(commandContext context.Context, arguments []string) error {
	if len(arguments) != 1 || (arguments[0] != "up" && arguments[0] != "status") {
		return fmt.Errorf("usage: colony migrate up|status")
	}
	loaded, err := loadConfig()
	if err != nil {
		return err
	}
	logger := newLogger(loaded.appEnvironment)
	pool, err := openDatabase(commandContext, loaded.databaseURL, "colony migrate")
	if err != nil {
		return err
	}
	defer pool.Close()

	if arguments[0] == "up" {
		return migrateUp(commandContext, pool, logger)
	}
	database := stdlib.OpenDBFromPool(pool)
	defer func() { _ = database.Close() }()
	provider, err := db.MigrationProvider(database, true)
	if err != nil {
		return err
	}
	statuses, err := provider.Status(commandContext)
	if err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	for _, status := range statuses {
		appliedAt := "-"
		if !status.AppliedAt.IsZero() {
			appliedAt = status.AppliedAt.UTC().Format("2006-01-02 15:04:05Z")
		}
		fmt.Printf("%-10s %-22s %s\n", status.State, appliedAt, filepath.Base(status.Source.Path))
	}
	return nil
}
