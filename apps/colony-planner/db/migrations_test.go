package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/BBruington/colony-planner/db"
	"github.com/BBruington/colony-planner/db/dbtest"
)

func countRows(t *testing.T, database *dbtest.Database, query string, arguments ...any) int64 {
	t.Helper()
	var count int64
	if err := database.Pool.QueryRow(context.Background(), query, arguments...).Scan(&count); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return count
}

// applicationTables lists the schema's tables other than goose's own.
func applicationTables(t *testing.T, database *dbtest.Database) []string {
	t.Helper()
	rows, err := database.Pool.Query(context.Background(), `SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' AND table_name <> 'goose_db_version'
		ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return tables
}

// A new database has no campaign until the site owner approves one: the migrations insert no rows,
// and core creates a campaign's settings, colony, resources and roles at approval. The Down section
// undoes everything, so a full down and up again works.
func TestAnEmptyDatabaseMigratesCleanly(t *testing.T) {
	testContext := context.Background()
	database := dbtest.New(t)
	tables := applicationTables(t, database)
	if len(tables) == 0 {
		t.Fatal("the migrations created no tables")
	}
	for _, table := range tables {
		if count := countRows(t, database, `SELECT count(*) FROM `+table); count != 0 {
			t.Errorf("%s has %d rows on a new database", table, count)
		}
	}

	connection := stdlib.OpenDBFromPool(database.Pool)
	defer func() { _ = connection.Close() }()
	provider, err := db.MigrationProvider(connection, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(testContext, 0); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if remaining := applicationTables(t, database); len(remaining) != 0 {
		t.Fatalf("tables left after migrating down: %v", remaining)
	}
	if types := countRows(t, database, `SELECT count(*) FROM pg_type
		WHERE typnamespace = current_schema()::regnamespace AND typtype = 'e'`); types != 0 {
		t.Fatalf("%d enum types left after migrating down", types)
	}
	if _, err := provider.Up(testContext); err != nil {
		t.Fatalf("migrate up again: %v", err)
	}
	if again := applicationTables(t, database); len(again) != len(tables) {
		t.Fatalf("tables after migrating up again = %v, want %v", again, tables)
	}
}

func requireConstraintViolation(t *testing.T, err error, constraintName string) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.ConstraintName != constraintName {
		t.Fatalf("got %v; want a violation of %s", err, constraintName)
	}
}

// The schema itself keeps campaigns apart and holds the membership rules (CLAUDE.md Invariant 11,
// docs/design/multi-campaign.md §2), whatever core does.
func TestTheSchemaKeepsCampaignsApart(t *testing.T) {
	testContext := context.Background()
	database := dbtest.New(t)
	execute := func(statement string, arguments ...any) error {
		_, err := database.Pool.Exec(testContext, statement, arguments...)
		return err
	}
	insertAccount := func(discordUserID string) uuid.UUID {
		t.Helper()
		var accountID uuid.UUID
		if err := database.Pool.QueryRow(testContext, `INSERT INTO account (discord_user_id, discord_username, display_name)
			VALUES ($1, $1, $1) RETURNING id`, discordUserID).Scan(&accountID); err != nil {
			t.Fatalf("insert account: %v", err)
		}
		return accountID
	}
	insertCampaign := func(slug string) uuid.UUID {
		t.Helper()
		var campaignID uuid.UUID
		if err := database.Pool.QueryRow(testContext, `INSERT INTO campaign (slug, name, invite_code)
			VALUES ($1, $1, repeat('a', 22) || $1) RETURNING id`, slug).Scan(&campaignID); err != nil {
			t.Fatalf("insert campaign: %v", err)
		}
		for _, statement := range []string{
			`INSERT INTO settings (campaign_id) VALUES ($1)`,
			`INSERT INTO colony (campaign_id) VALUES ($1)`,
			`INSERT INTO resource (campaign_id, key, label, icon) VALUES ($1, 'food', 'Food', 'Wheat')`,
			`INSERT INTO role (campaign_id, key, label, icon) VALUES ($1, 'FARMER', 'Farmer', 'Wheat')`,
		} {
			if err := execute(statement, campaignID); err != nil {
				t.Fatalf("%s: %v", statement, err)
			}
		}
		return campaignID
	}
	insertMember := func(campaignID uuid.UUID, accountID uuid.UUID, role string, status string) uuid.UUID {
		t.Helper()
		var memberID uuid.UUID
		if err := database.Pool.QueryRow(testContext, `INSERT INTO campaign_member (campaign_id, account_id, role, status)
			VALUES ($1, $2, $3::campaign_role, $4::member_status) RETURNING id`, campaignID, accountID, role, status).Scan(&memberID); err != nil {
			t.Fatalf("insert member: %v", err)
		}
		return memberID
	}

	dmAccount := insertAccount("100000000000000001")
	playerAccount := insertAccount("100000000000000002")
	firstCampaign := insertCampaign("first-campaign")
	secondCampaign := insertCampaign("second-campaign")
	firstDm := insertMember(firstCampaign, dmAccount, "DM", "ACTIVE")
	firstPlayer := insertMember(firstCampaign, playerAccount, "PLAYER", "ACTIVE")
	insertMember(secondCampaign, playerAccount, "DM", "ACTIVE")

	// Keys are per campaign: both campaigns have their own food.
	if count := countRows(t, database, `SELECT count(*) FROM resource WHERE key = 'food'`); count != 2 {
		t.Fatalf("food rows = %d, want one per campaign", count)
	}
	requireConstraintViolation(t,
		execute(`INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, created_by)
			VALUES ($1, 0, 'CUSTOM', 'Cross', 'PUBLIC', $2)`, secondCampaign, firstDm),
		"timeline_event_created_by_fkey")
	requireConstraintViolation(t, execute(`UPDATE campaign_member SET role = 'DM' WHERE id = $1`, firstPlayer), "campaign_member_one_dm")
	requireConstraintViolation(t, execute(`UPDATE campaign_member SET status = 'REMOVED' WHERE id = $1`, firstDm), "campaign_member_dm_active")
	requireConstraintViolation(t, execute(`UPDATE campaign_member SET left_on_own = true WHERE id = $1`, firstPlayer), "campaign_member_left_is_removed")
	requireConstraintViolation(t, execute(`INSERT INTO campaign (slug, name, invite_code) VALUES ('No Capitals', 'x', repeat('b', 22))`), "campaign_slug_check")
	const requestStatement = `INSERT INTO campaign_request (account_id, campaign_name, slug) VALUES ($1, $2, $2)`
	if err := execute(requestStatement, dmAccount, "sky-isles"); err != nil {
		t.Fatalf("insert campaign request: %v", err)
	}
	requireConstraintViolation(t, execute(requestStatement, dmAccount, "sky-isles-two"), "campaign_request_one_open")
	requireConstraintViolation(t, execute(requestStatement, playerAccount, "sky-isles"), "campaign_request_open_slug")

	// Removing a member clears only the author column of their entries, not the entry's campaign.
	if err := execute(`INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, created_by)
		VALUES ($1, 0, 'CUSTOM', 'Founded', 'PUBLIC', $2)`, firstCampaign, firstPlayer); err != nil {
		t.Fatalf("insert timeline entry: %v", err)
	}
	if err := execute(`DELETE FROM campaign_member WHERE id = $1`, firstPlayer); err != nil {
		t.Fatalf("delete member: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM timeline_event WHERE campaign_id = $1 AND created_by IS NULL`, firstCampaign); count != 1 {
		t.Fatalf("the entry of a deleted member = %d rows, want 1 with no author", count)
	}

	// Deleting a campaign takes every campaign-owned row with it and leaves the other campaign alone.
	if err := execute(`DELETE FROM campaign WHERE id = $1`, firstCampaign); err != nil {
		t.Fatalf("delete campaign: %v", err)
	}
	for _, table := range []string{"settings", "colony", "resource", "role", "timeline_event", "campaign_member"} {
		if left := countRows(t, database, `SELECT count(*) FROM `+table+` WHERE campaign_id = $1`, firstCampaign); left != 0 {
			t.Errorf("%s kept %d rows of the deleted campaign", table, left)
		}
		if kept := countRows(t, database, `SELECT count(*) FROM `+table+` WHERE campaign_id = $1`, secondCampaign); kept == 0 && table != "timeline_event" {
			t.Errorf("%s lost the other campaign's rows", table)
		}
	}
}
