package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/seed"
)

func seedDatabase(commandContext context.Context, arguments []string) error {
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	hexes := flags.Int("hexes", 0, "paint this many sample hexes instead of the default handful")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	loaded, err := loadConfig()
	if err != nil {
		return err
	}
	if err := seed.RequireLocalDevelopment(loaded.appEnvironment, loaded.databaseURL); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	logger := newLogger(loaded.appEnvironment)
	pool, err := openDatabase(commandContext, loaded.databaseURL, "colony seed")
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrateUp(commandContext, pool, logger); err != nil {
		return err
	}

	ownerDiscordID := loaded.siteOwnerDiscordID
	if ownerDiscordID == "" {
		ownerDiscordID = core.SeedDmDiscordID
	}
	mainlandArt, err := os.ReadFile(seed.MainlandArtPath)
	if err != nil {
		return fmt.Errorf("seed: run it from the repository root, where the Mainland art is: %w", err)
	}
	service := core.New(core.Options{Pool: pool, Logger: logger, SiteOwnerDiscordID: ownerDiscordID})
	if err := seed.Run(commandContext, seed.Options{Pool: pool, Core: service, OwnerDiscordID: ownerDiscordID, Hexes: *hexes, MainlandArt: mainlandArt}); err != nil {
		return err
	}
	fmt.Println("seed: the two sample campaigns are loaded.")
	return nil
}

func resetDatabase(commandContext context.Context) error {
	loaded, err := loadConfig()
	if err != nil {
		return err
	}
	if err := seed.RequireLocalDevelopment(loaded.appEnvironment, loaded.databaseURL); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	logger := newLogger(loaded.appEnvironment)
	pool, err := openDatabase(commandContext, loaded.databaseURL, "colony reset")
	if err != nil {
		return err
	}
	defer pool.Close()
	// Maintenance DDL rather than an application query, so it lives here and not in db/queries. It
	// runs before migrateUp's history check, so reset (and fresh) rebuild a database that check refuses.
	if _, err := pool.Exec(commandContext, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return fmt.Errorf("reset: drop the schema: %w", err)
	}
	if err := migrateUp(commandContext, pool, logger); err != nil {
		return err
	}
	// A running server (`mise run dev`) still holds connections with statements prepared against the
	// dropped tables, which Postgres refuses once each ("cached plan must not change result type").
	// Closing them makes it reconnect against the new schema; browsers signed in before the reset are
	// signed out on their next visit (core.ErrSessionEnded).
	var closedConnections int
	if err := pool.QueryRow(commandContext, `SELECT count(*) FILTER (WHERE pg_terminate_backend(pid))
		FROM pg_stat_activity WHERE datname = current_database() AND application_name = $1`,
		serverApplicationName).Scan(&closedConnections); err != nil {
		return fmt.Errorf("reset: close the running server's connections: %w", err)
	}
	fmt.Println("reset: the local database is empty and migrated.")
	if closedConnections > 0 {
		fmt.Println("reset: a running server was connected; it reconnects by itself, and everyone signs in again.")
	}
	return nil
}
