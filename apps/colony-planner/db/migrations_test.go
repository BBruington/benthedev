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

func queryText(t *testing.T, database *dbtest.Database, query string, arguments ...any) string {
	t.Helper()
	var value string
	if err := database.Pool.QueryRow(context.Background(), query, arguments...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
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

	// People and places (M2): every link between them is composite, so it stays inside one campaign.
	insertPlace := func(campaignID uuid.UUID, name string) uuid.UUID {
		t.Helper()
		var placeID uuid.UUID
		if err := database.Pool.QueryRow(testContext, `INSERT INTO place (campaign_id, name) VALUES ($1, $2) RETURNING id`, campaignID, name).Scan(&placeID); err != nil {
			t.Fatalf("insert place: %v", err)
		}
		return placeID
	}
	insertNpc := func(campaignID uuid.UUID, name string) uuid.UUID {
		t.Helper()
		var npcID uuid.UUID
		if err := database.Pool.QueryRow(testContext, `INSERT INTO npc (campaign_id, name) VALUES ($1, $2) RETURNING id`, campaignID, name).Scan(&npcID); err != nil {
			t.Fatalf("insert npc: %v", err)
		}
		return npcID
	}
	firstPlace := insertPlace(firstCampaign, "Docks")
	secondPlace := insertPlace(secondCampaign, "Ridge")
	firstNpc := insertNpc(firstCampaign, "Brannoc")
	secondNpc := insertNpc(secondCampaign, "Dagny")
	requireConstraintViolation(t, execute(`UPDATE npc SET current_place_id = $1 WHERE id = $2`, secondPlace, firstNpc), "npc_current_place_fkey")
	requireConstraintViolation(t, execute(`UPDATE npc SET origin_place_id = $1 WHERE id = $2`, secondPlace, firstNpc), "npc_origin_place_fkey")
	requireConstraintViolation(t, execute(`INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, npc_id)
		VALUES ($1, 0, 'CUSTOM', 'Cross', 'PUBLIC', $2)`, firstCampaign, secondNpc), "timeline_event_npc_fkey")
	if err := execute(`INSERT INTO role (campaign_id, key, label, icon) VALUES ($1, 'SAILOR', 'Sailor', 'Anchor')`, secondCampaign); err != nil {
		t.Fatalf("insert role: %v", err)
	}
	requireConstraintViolation(t, execute(`UPDATE npc SET colony_role_key = 'SAILOR' WHERE id = $1`, firstNpc), "npc_colony_role_fkey")
	requireConstraintViolation(t, execute(`INSERT INTO npc (campaign_id, name) VALUES ($1, '')`, firstCampaign), "npc_name_check")
	// A role is archived only when its headcount is 0, named colonists included (spec §4.2).
	if err := execute(`UPDATE npc SET colony_role_key = 'FARMER', current_place_id = $1, origin_place_id = $1, known_name = 'The Farmer' WHERE id = $2`, firstPlace, firstNpc); err != nil {
		t.Fatalf("make a named colonist: %v", err)
	}
	if displayName := queryText(t, database, `SELECT display_name FROM npc WHERE id = $1`, firstNpc); displayName != "The Farmer" {
		t.Fatalf("display_name = %q, want the known name", displayName)
	}
	requireConstraintViolation(t, execute(`UPDATE role SET archived = true WHERE campaign_id = $1 AND key = 'FARMER'`, firstCampaign), "role_archived_no_named_colonists")
	if err := execute(`UPDATE role SET archived = true WHERE campaign_id = $1 AND key = 'SAILOR'`, secondCampaign); err != nil {
		t.Fatalf("archive an empty role: %v", err)
	}
	requireConstraintViolation(t, execute(`UPDATE npc SET colony_role_key = 'SAILOR' WHERE id = $1`, secondNpc), "npc_colony_role_not_archived")
	// Deleting a place clears only the NPC's place columns; deleting an NPC clears only the entry's link.
	if err := execute(`INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, npc_id)
		VALUES ($1, 0, 'COLONIST_JOINED', 'Joined', 'PUBLIC', $2)`, secondCampaign, secondNpc); err != nil {
		t.Fatalf("insert a linked entry: %v", err)
	}
	if err := execute(`DELETE FROM place WHERE id = $1`, firstPlace); err != nil {
		t.Fatalf("delete place: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM npc WHERE id = $1 AND campaign_id = $2 AND current_place_id IS NULL AND origin_place_id IS NULL`, firstNpc, firstCampaign); count != 1 {
		t.Fatalf("the NPC of a deleted place = %d rows, want 1 with no place", count)
	}
	if err := execute(`DELETE FROM npc WHERE id = $1`, secondNpc); err != nil {
		t.Fatalf("delete npc: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM timeline_event WHERE campaign_id = $1 AND kind = 'COLONIST_JOINED' AND npc_id IS NULL`, secondCampaign); count != 1 {
		t.Fatalf("the entry of a deleted NPC = %d rows, want 1 with no link", count)
	}
	insertNpc(secondCampaign, "Old Brisk")

	// Quests, patrons, actions and requests (M3): every link is composite, so it stays inside one campaign.
	insertRow := func(statement string, arguments ...any) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := database.Pool.QueryRow(testContext, statement, arguments...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
		return id
	}
	firstPatron := insertRow(`INSERT INTO patron (campaign_id, name) VALUES ($1, 'Lady Vex') RETURNING id`, firstCampaign)
	secondPatron := insertRow(`INSERT INTO patron (campaign_id, name) VALUES ($1, 'Ridge Council') RETURNING id`, secondCampaign)
	firstAction := insertRow(`INSERT INTO patron_action (campaign_id, patron_id, name, favor_cost) VALUES ($1, $2, 'Grain Barge', 3) RETURNING id`, firstCampaign, firstPatron)
	secondAction := insertRow(`INSERT INTO patron_action (campaign_id, patron_id, name) VALUES ($1, $2, 'Timber Levy') RETURNING id`, secondCampaign, secondPatron)
	firstQuest := insertRow(`INSERT INTO quest (campaign_id, title, patron_id, reward_favor) VALUES ($1, 'Clear the Wolf Den', $2, 2) RETURNING id`, firstCampaign, firstPatron)
	secondQuest := insertRow(`INSERT INTO quest (campaign_id, title) VALUES ($1, 'Hold the Ridge') RETURNING id`, secondCampaign)
	secondDm := insertRow(`SELECT id FROM campaign_member WHERE campaign_id = $1 AND role = 'DM'`, secondCampaign)
	firstRequest := insertRow(`INSERT INTO request (campaign_id, kind, title, patron_id, patron_name, patron_action_id, requested_by, requested_day, made_while_revealed)
		VALUES ($1, 'PATRON_ACTION', 'Grain Barge', $2, 'Lady Vex', $3, $4, 0, true) RETURNING id`, firstCampaign, firstPatron, firstAction, firstDm)
	secondRequest := insertRow(`INSERT INTO request (campaign_id, kind, title, patron_id, patron_name, patron_action_id, requested_by, requested_day, made_while_revealed)
		VALUES ($1, 'PATRON_ACTION', 'Timber Levy', $2, 'Ridge Council', $3, $4, 0, true) RETURNING id`, secondCampaign, secondPatron, secondAction, secondDm)

	requireConstraintViolation(t, execute(`INSERT INTO patron_action (campaign_id, patron_id, name) VALUES ($1, $2, 'Cross')`, firstCampaign, secondPatron), "patron_action_patron_fkey")
	requireConstraintViolation(t, execute(`UPDATE quest SET patron_id = $1 WHERE id = $2`, secondPatron, firstQuest), "quest_patron_fkey")
	requireConstraintViolation(t, execute(`UPDATE quest SET giver_npc_id = $1 WHERE id = $2`, secondNpc, firstQuest), "quest_giver_npc_fkey")
	requireConstraintViolation(t, execute(`UPDATE request SET patron_id = $1 WHERE id = $2`, secondPatron, firstRequest), "request_patron_fkey")
	requireConstraintViolation(t, execute(`UPDATE request SET patron_action_id = $1 WHERE id = $2`, secondAction, firstRequest), "request_patron_action_fkey")
	requireConstraintViolation(t, execute(`UPDATE request SET requested_by = $1 WHERE id = $2`, secondDm, firstRequest), "request_requested_by_fkey")
	requireConstraintViolation(t, execute(`UPDATE request SET on_behalf_of = $1 WHERE id = $2`, secondDm, firstRequest), "request_on_behalf_of_fkey")
	requireConstraintViolation(t, execute(`UPDATE request SET decided_by = $1 WHERE id = $2`, secondDm, firstRequest), "request_decided_by_fkey")
	for _, link := range []struct {
		column, constraint string
		otherCampaignRow   uuid.UUID
	}{
		{"quest_id", "timeline_event_quest_fkey", secondQuest},
		{"patron_id", "timeline_event_patron_fkey", secondPatron},
		{"request_id", "timeline_event_request_fkey", secondRequest},
	} {
		requireConstraintViolation(t, execute(`INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, `+link.column+`)
			VALUES ($1, 0, 'CUSTOM', 'Cross', 'PUBLIC', $2)`, firstCampaign, link.otherCampaignRow), link.constraint)
	}
	// Favor needs a patron giver, for a reward and for a failure cost alike.
	requireConstraintViolation(t, execute(`INSERT INTO quest (campaign_id, title, reward_favor) VALUES ($1, 'Unpaid', 1)`, firstCampaign), "quest_reward_favor_needs_patron")
	requireConstraintViolation(t, execute(`INSERT INTO quest (campaign_id, title, failure_favor) VALUES ($1, 'Unpaid', 1)`, firstCampaign), "quest_failure_favor_needs_patron")
	requireConstraintViolation(t, execute(`UPDATE patron_action SET favor_cost = -1 WHERE id = $1`, firstAction), "patron_action_favor_cost_check")
	// A quest's favor amounts are never negative, though the patron's own favor may go below 0.
	requireConstraintViolation(t, execute(`UPDATE quest SET reward_favor = -1 WHERE id = $1`, firstQuest), "quest_reward_favor_check")
	requireConstraintViolation(t, execute(`UPDATE quest SET failure_favor = -1 WHERE id = $1`, firstQuest), "quest_failure_favor_check")
	requireConstraintViolation(t, execute(`UPDATE request SET patron_name = NULL WHERE id = $1`, firstRequest), "request_patron_action_names_patron")
	// Patrons and actions are archived, never deleted: a stray delete fails loudly.
	requireConstraintViolation(t, execute(`DELETE FROM patron WHERE id = $1`, firstPatron), "patron_action_patron_fkey")
	requireConstraintViolation(t, execute(`DELETE FROM patron_action WHERE id = $1`, firstAction), "request_patron_action_fkey")
	// Deleting a quest or an NPC clears only the link column of the rows that name them.
	if err := execute(`UPDATE quest SET giver_npc_id = $1 WHERE id = $2`, firstNpc, firstQuest); err != nil {
		t.Fatalf("set a quest giver: %v", err)
	}
	if err := execute(`INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, quest_id, patron_id, request_id)
		VALUES ($1, 0, 'QUEST_POSTED', 'New quest: Hold the Ridge', 'PUBLIC', $2, $3, $4)`, secondCampaign, secondQuest, secondPatron, secondRequest); err != nil {
		t.Fatalf("insert a linked entry: %v", err)
	}
	if err := execute(`DELETE FROM quest WHERE id = $1`, secondQuest); err != nil {
		t.Fatalf("delete quest: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM timeline_event WHERE campaign_id = $1 AND kind = 'QUEST_POSTED'
		AND quest_id IS NULL AND patron_id = $2 AND request_id = $3`, secondCampaign, secondPatron, secondRequest); count != 1 {
		t.Fatalf("the entry of a deleted quest = %d rows, want 1 with only the quest link cleared", count)
	}
	if err := execute(`DELETE FROM npc WHERE id = $1`, firstNpc); err != nil {
		t.Fatalf("delete the quest giver: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM quest WHERE id = $1 AND campaign_id = $2 AND giver_npc_id IS NULL AND patron_id = $3`,
		firstQuest, firstCampaign, firstPatron); count != 1 {
		t.Fatalf("the quest of a deleted giver = %d rows, want 1 with only the giver cleared", count)
	}

	// World maps (M4): images, maps, pins and portraits stay inside one campaign.
	insertImage := func(campaignID uuid.UUID, uploadedBy any, digest byte) uuid.UUID {
		t.Helper()
		return insertRow(`INSERT INTO image (campaign_id, content_type, bytes, sha256, width, height, uploaded_by)
			VALUES ($1, 'image/png', '\x89504e47'::bytea, decode(repeat(lpad(to_hex($3::integer), 2, '0'), 32), 'hex'), 4, 3, $2) RETURNING id`,
			campaignID, uploadedBy, int(digest))
	}
	firstImage := insertImage(firstCampaign, firstDm, 1)
	secondImage := insertImage(secondCampaign, secondDm, 1)
	requireConstraintViolation(t, execute(`INSERT INTO image (campaign_id, content_type, bytes, sha256, width, height)
		SELECT campaign_id, content_type, bytes, sha256, width, height FROM image WHERE id = $1`, firstImage), "image_campaign_sha256_key")
	requireConstraintViolation(t, execute(`UPDATE image SET uploaded_by = $1 WHERE id = $2`, secondDm, firstImage), "image_uploaded_by_fkey")
	requireConstraintViolation(t, execute(`UPDATE image SET content_type = 'image/svg+xml' WHERE id = $1`, firstImage), "image_content_type_check")
	firstMap := insertRow(`INSERT INTO map (campaign_id, name, image_id, width, height) VALUES ($1, 'Mainland', $2, 4, 3) RETURNING id`, firstCampaign, firstImage)
	secondMap := insertRow(`INSERT INTO map (campaign_id, name, image_id, width, height, is_mainland) VALUES ($1, 'Ridge', $2, 4, 3, true) RETURNING id`, secondCampaign, secondImage)
	requireConstraintViolation(t, execute(`INSERT INTO map (campaign_id, name, image_id, width, height) VALUES ($1, 'Cross', $2, 4, 3)`, firstCampaign, secondImage), "map_image_fkey")
	requireConstraintViolation(t, execute(`INSERT INTO map (campaign_id, name, external_url, width, height) VALUES ($1, 'Bundled', '/eldari.webp', 4, 3)`, firstCampaign), "map_external_url_check")
	requireConstraintViolation(t, execute(`INSERT INTO map (campaign_id, name, width, height) VALUES ($1, 'No art', 4, 3)`, firstCampaign), "map_one_source")
	if err := execute(`INSERT INTO map (campaign_id, name, external_url, width, height, is_mainland) VALUES ($1, 'Elsewhere', 'https://example.com/a.png', 4, 3, true)`, firstCampaign); err != nil {
		t.Fatalf("insert an external Mainland: %v", err)
	}
	requireConstraintViolation(t, execute(`UPDATE map SET is_mainland = true WHERE id = $1`, firstMap), "map_one_mainland")
	requireConstraintViolation(t, execute(`DELETE FROM image WHERE id = $1`, firstImage), "map_image_fkey")
	secondPlaceID := insertPlace(secondCampaign, "Pinned")
	requireConstraintViolation(t, execute(`UPDATE place SET map_id = $1, pin_x = 0.5, pin_y = 0.5 WHERE id = $2`, firstMap, secondPlaceID), "place_map_fkey")
	requireConstraintViolation(t, execute(`UPDATE place SET map_id = $1, pin_x = 0.5 WHERE id = $2`, secondMap, secondPlaceID), "place_pin_both")
	requireConstraintViolation(t, execute(`UPDATE place SET map_id = $1, pin_x = 1.5, pin_y = 0.5 WHERE id = $2`, secondMap, secondPlaceID), "place_pin_x_check")
	if err := execute(`UPDATE place SET map_id = $1, pin_x = 0.25, pin_y = 0.75 WHERE id = $2`, secondMap, secondPlaceID); err != nil {
		t.Fatalf("pin a place: %v", err)
	}
	portraitNpc := insertNpc(secondCampaign, "Portrait Sitter")
	requireConstraintViolation(t, execute(`UPDATE npc SET portrait_image_id = $1 WHERE id = $2`, firstImage, portraitNpc), "npc_portrait_image_fkey")
	if err := execute(`UPDATE npc SET portrait_image_id = $1 WHERE id = $2`, secondImage, portraitNpc); err != nil {
		t.Fatalf("set a portrait: %v", err)
	}
	requireConstraintViolation(t, execute(`INSERT INTO hex (campaign_id, q, r, terrain) VALUES ($1, 100001, 0, 'FOREST')`, firstCampaign), "hex_q_check")
	requireConstraintViolation(t, execute(`INSERT INTO hex (campaign_id, q, r, terrain) VALUES ($1, 0, -100001, 'FOREST')`, firstCampaign), "hex_r_check")
	requireConstraintViolation(t, execute(`INSERT INTO hex (campaign_id, q, r, terrain, label) VALUES ($1, 0, 0, 'FOREST', repeat('x', 41))`, firstCampaign), "hex_label_check")
	for _, campaignID := range []uuid.UUID{firstCampaign, secondCampaign} {
		if err := execute(`INSERT INTO hex (campaign_id, q, r, terrain, label) VALUES ($1, 100000, -100000, 'WATER', 'Far shore')`, campaignID); err != nil {
			t.Fatalf("chart a hex: %v", err)
		}
	}
	// Deleting a map clears only the pinned place's map column; the place stays.
	if err := execute(`DELETE FROM map WHERE id = $1`, secondMap); err != nil {
		t.Fatalf("delete map: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM place WHERE id = $1 AND campaign_id = $2 AND map_id IS NULL`, secondPlaceID, secondCampaign); count != 1 {
		t.Fatalf("the place of a deleted map = %d rows, want 1 with no map", count)
	}

	// Sessions (M5): series, dates and their links stay inside one campaign.
	insertSeries := func(campaignID uuid.UUID, title string) uuid.UUID {
		t.Helper()
		return insertRow(`INSERT INTO session_series (campaign_id, title, weekdays, every_weeks, first_date, start_local, duration_minutes)
			VALUES ($1, $2, '{5}', 1, '2026-10-02', '20:00', 240) RETURNING id`, campaignID, title)
	}
	firstSeries := insertSeries(firstCampaign, "Friday game")
	secondSeries := insertSeries(secondCampaign, "Ridge nights")
	insertSeriesWith := func(columns string, values string) error {
		return execute(`INSERT INTO session_series (campaign_id, title, first_date, start_local, duration_minutes, `+columns+`)
			VALUES ($1, 'Bad', '2026-10-02', '20:00', 240, `+values+`)`, firstCampaign)
	}
	requireConstraintViolation(t, insertSeriesWith("weekdays, every_weeks", "'{5}', NULL"), "session_series_repeats_on_weekdays")
	requireConstraintViolation(t, insertSeriesWith("weekdays, every_weeks", "'{}', 1"), "session_series_repeats_on_weekdays")
	requireConstraintViolation(t, insertSeriesWith("weekdays, every_weeks", "'{7}', 1"), "session_series_weekdays_check")
	requireConstraintViolation(t, insertSeriesWith("weekdays, every_weeks", "'{5}', 3"), "session_series_every_weeks_check")
	requireConstraintViolation(t, insertSeriesWith("last_date", "'2026-12-01'"), "session_series_one_off_has_no_end")
	requireConstraintViolation(t, insertSeriesWith("weekdays, every_weeks, last_date", "'{5}', 2, '2026-10-01'"), "session_series_dates_in_order")
	requireConstraintViolation(t, execute(`INSERT INTO session_series (campaign_id, title, first_date, start_local, duration_minutes)
		VALUES ($1, 'Short', '2026-10-02', '20:00', 10)`, firstCampaign), "session_series_duration_minutes_check")
	if err := execute(`INSERT INTO session_series (campaign_id, title, first_date, start_local, duration_minutes)
		VALUES ($1, 'One-off', '2026-10-09', '19:30', 180)`, firstCampaign); err != nil {
		t.Fatalf("insert a one-off: %v", err)
	}
	insertSession := func(campaignID uuid.UUID, seriesID any, localDate string) uuid.UUID {
		t.Helper()
		return insertRow(`INSERT INTO session (campaign_id, series_id, local_date, starts_at, duration_minutes, title)
			VALUES ($1, $2, $3::date, '2026-10-03T00:00:00Z', 240, 'Friday game') RETURNING id`, campaignID, seriesID, localDate)
	}
	firstSession := insertSession(firstCampaign, firstSeries, "2026-10-02")
	secondSession := insertSession(secondCampaign, secondSeries, "2026-10-02")
	requireConstraintViolation(t, execute(`INSERT INTO session (campaign_id, series_id, local_date, starts_at, duration_minutes, title)
		VALUES ($1, $2, '2026-10-09', now(), 240, 'Cross')`, firstCampaign, secondSeries), "session_series_fkey")
	requireConstraintViolation(t, execute(`INSERT INTO session (campaign_id, series_id, local_date, starts_at, duration_minutes, title)
		VALUES ($1, $2, '2026-10-02', now(), 240, 'Twice')`, firstCampaign, firstSeries), "session_series_local_date_key")
	requireConstraintViolation(t, execute(`UPDATE session SET colony_day_start = 3 WHERE id = $1`, firstSession), "session_stamps_when_played")
	requireConstraintViolation(t, execute(`UPDATE session SET status = 'PLAYED', colony_day_start = 5, colony_day_end = 4 WHERE id = $1`, firstSession), "session_end_after_start")
	requireConstraintViolation(t, execute(`UPDATE session SET discord_event_id = 'not-a-snowflake' WHERE id = $1`, firstSession), "session_discord_event_id_check")
	if err := execute(`UPDATE session SET status = 'PLAYED', colony_day_start = 5, colony_day_end = 7 WHERE id = $1`, firstSession); err != nil {
		t.Fatalf("stamp a played session: %v", err)
	}
	metNpc := insertNpc(secondCampaign, "Met at the Ridge")
	requireConstraintViolation(t, execute(`UPDATE npc SET first_session_id = $1 WHERE id = $2`, firstSession, metNpc), "npc_first_session_fkey")
	requireConstraintViolation(t, execute(`INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, session_id)
		VALUES ($1, 5, 'SESSION_PLAYED', 'Session played', 'PUBLIC', $2)`, secondCampaign, firstSession), "timeline_event_session_fkey")
	requireConstraintViolation(t, execute(`UPDATE settings SET ics_token = (SELECT ics_token FROM settings WHERE campaign_id = $1)
		WHERE campaign_id = $2`, firstCampaign, secondCampaign), "settings_ics_token_key")
	requireConstraintViolation(t, execute(`UPDATE settings SET ics_token = 'short' WHERE campaign_id = $1`, firstCampaign), "settings_ics_token_check")
	if tokens := countRows(t, database, `SELECT count(DISTINCT ics_token) FROM settings WHERE ics_token ~ '^[0-9a-f]{64}$'`); tokens != 2 {
		t.Fatalf("distinct generated ICS tokens = %d, want 2", tokens)
	}
	if location := queryText(t, database, `SELECT discord_event_location FROM settings WHERE campaign_id = $1`, firstCampaign); location != "Online" {
		t.Fatalf("default event location = %q", location)
	}
	// Deleting a session clears only the NPC's and the entry's link; deleting a series keeps its dates.
	metSession := insertSession(secondCampaign, secondSeries, "2026-10-09")
	if err := execute(`UPDATE npc SET first_session_id = $1 WHERE id = $2`, metSession, metNpc); err != nil {
		t.Fatalf("set a first session: %v", err)
	}
	if err := execute(`INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, session_id)
		VALUES ($1, 5, 'SESSION_PLAYED', 'Session played', 'PUBLIC', $2)`, secondCampaign, metSession); err != nil {
		t.Fatalf("insert a session entry: %v", err)
	}
	if err := execute(`DELETE FROM session WHERE id = $1`, metSession); err != nil {
		t.Fatalf("delete a session: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM npc WHERE id = $1 AND campaign_id = $2 AND first_session_id IS NULL`, metNpc, secondCampaign); count != 1 {
		t.Fatalf("the NPC of a deleted session = %d rows, want 1 with no first session", count)
	}
	if count := countRows(t, database, `SELECT count(*) FROM timeline_event WHERE campaign_id = $1 AND kind = 'SESSION_PLAYED' AND session_id IS NULL`, secondCampaign); count != 1 {
		t.Fatalf("the entry of a deleted session = %d rows, want 1 with no session", count)
	}
	if err := execute(`DELETE FROM session_series WHERE id = $1`, firstSeries); err != nil {
		t.Fatalf("delete a series: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM session WHERE id = $1 AND campaign_id = $2 AND series_id IS NULL AND status = 'PLAYED'`, firstSession, firstCampaign); count != 1 {
		t.Fatalf("the played date of a deleted series = %d rows, want 1 with no series", count)
	}
	// Deleting a campaign takes every campaign-owned row with it and leaves the other campaign alone.
	if err := execute(`DELETE FROM campaign WHERE id = $1`, firstCampaign); err != nil {
		t.Fatalf("delete campaign: %v", err)
	}
	for _, table := range []string{"settings", "colony", "resource", "role", "timeline_event", "campaign_member", "npc", "place",
		"patron", "patron_action", "quest", "request", "image", "map", "hex", "session_series", "session"} {
		if left := countRows(t, database, `SELECT count(*) FROM `+table+` WHERE campaign_id = $1`, firstCampaign); left != 0 {
			t.Errorf("%s kept %d rows of the deleted campaign", table, left)
		}
		if kept := countRows(t, database, `SELECT count(*) FROM `+table+` WHERE campaign_id = $1`, secondCampaign); kept == 0 && table != "timeline_event" && table != "quest" && table != "map" {
			t.Errorf("%s lost the other campaign's rows", table)
		}
	}
	if kept := countRows(t, database, `SELECT count(*) FROM session WHERE id = $1 AND series_id = $2`, secondSession, secondSeries); kept != 1 {
		t.Errorf("the other campaign's dated session = %d rows, want 1", kept)
	}
}
