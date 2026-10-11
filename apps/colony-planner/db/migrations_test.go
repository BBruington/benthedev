package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/db/dbtest"
)

// Migration 0003 turns the author's single-campaign database into the first campaign
// (docs/design/multi-campaign.md §2.4). These tests migrate to 0002, insert rows in the old shape,
// then apply 0003. They set the environment goose substitutes, so they don't run in parallel, and
// they never depend on the developer's own .env values.

const (
	authorDiscordID = "123456789012345678"
	otherDiscordID  = "223456789012345678"
)

type legacyMember struct {
	discordUserID string
	status        string
	characterName *string
}

func insertLegacyMember(t *testing.T, database *dbtest.Database, member legacyMember) uuid.UUID {
	t.Helper()
	var memberID uuid.UUID
	err := database.Pool.QueryRow(context.Background(), `
		INSERT INTO member (discord_user_id, discord_username, display_name, status, character_name, approved_at)
		VALUES ($1, $1, $1, $2::member_status, $3, CASE WHEN $2 = 'PENDING' THEN NULL ELSE now() END)
		RETURNING id`, member.discordUserID, member.status, member.characterName).Scan(&memberID)
	if err != nil {
		t.Fatalf("insert legacy member: %v", err)
	}
	return memberID
}

func setMigrationEnvironment(t *testing.T, siteOwnerDiscordID string, legacyDmDiscordID string) {
	t.Helper()
	t.Setenv("SITE_OWNER_DISCORD_ID", siteOwnerDiscordID)
	t.Setenv("DM_DISCORD_ID", legacyDmDiscordID)
}

func countRows(t *testing.T, database *dbtest.Database, query string, arguments ...any) int64 {
	t.Helper()
	var count int64
	if err := database.Pool.QueryRow(context.Background(), query, arguments...).Scan(&count); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return count
}

func TestMigration0003TurnsTheAuthorsDataIntoTheFirstCampaign(t *testing.T) {
	setMigrationEnvironment(t, authorDiscordID, "")
	testContext := context.Background()
	database := dbtest.NewAtVersion(t, 2)
	thorin := "Thorin"
	author := insertLegacyMember(t, database, legacyMember{discordUserID: authorDiscordID, status: "PENDING"})
	ana := insertLegacyMember(t, database, legacyMember{discordUserID: "seed:ana", status: "ACTIVE", characterName: &thorin})
	emil := insertLegacyMember(t, database, legacyMember{discordUserID: "seed:emil", status: "PENDING"})
	gus := insertLegacyMember(t, database, legacyMember{discordUserID: "seed:gus", status: "REMOVED"})
	statements := []string{
		`UPDATE settings SET campaign_name = 'The Iron Coast of the Far Northern Reach - Year 2', timezone = 'Europe/Lisbon', discord_announce_channel_id = '323456789012345678', discord_reminder_channel_id = '423456789012345678', discord_dm_channel_id = '523456789012345678'`,
		`UPDATE colony SET name = 'Port Eldari', day = 22, version = 9`,
		`UPDATE resource SET amount = 63 WHERE key = 'food'`,
		`UPDATE role SET unnamed_count = 4 WHERE key = 'FARMER'`,
	}
	for _, statement := range statements {
		if _, err := database.Pool.Exec(testContext, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if _, err := database.Pool.Exec(testContext, `
		INSERT INTO timeline_event (day, kind, title, visibility, created_by) VALUES
			(3, 'CUSTOM', 'Founded', 'PUBLIC', $1),
			(5, 'CUSTOM', 'A note', 'DM', NULL)`, author); err != nil {
		t.Fatalf("insert history: %v", err)
	}
	if _, err := database.Pool.Exec(testContext, `
		INSERT INTO web_session (token_hash, member_id, expires_at) VALUES ($1, $2, $3)`,
		make([]byte, 32), ana, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	if err := database.MigrateTo(testContext, 3); err != nil {
		t.Fatalf("migrate to 0003: %v", err)
	}

	var campaignID uuid.UUID
	var slug, name, inviteCode, status string
	if err := database.Pool.QueryRow(testContext, `SELECT id, slug, name, invite_code, status FROM campaign`).
		Scan(&campaignID, &slug, &name, &inviteCode, &status); err != nil {
		t.Fatalf("read the one campaign: %v", err)
	}
	if name != "The Iron Coast of the Far Northern Reach - Year 2" || slug != "the-iron-coast-of-the-far-northern-reach" || status != "ACTIVE" {
		t.Fatalf("campaign = %q, %q, %q", name, slug, status)
	}
	if len(inviteCode) < 22 {
		t.Fatalf("invite code %q is too short", inviteCode)
	}
	for _, table := range []string{"settings", "colony", "resource", "role", "timeline_event", "campaign_member"} {
		if unattributed := countRows(t, database, `SELECT count(*) FROM `+table+` WHERE campaign_id IS DISTINCT FROM $1`, campaignID); unattributed != 0 {
			t.Errorf("%s has %d rows outside the first campaign", table, unattributed)
		}
	}
	if count := countRows(t, database, `SELECT count(*) FROM campaign`); count != 1 {
		t.Fatalf("campaigns = %d, want 1", count)
	}
	if count := countRows(t, database, `SELECT count(*) FROM resource`); count != 4 {
		t.Fatalf("resources = %d", count)
	}
	if day := countRows(t, database, `SELECT day FROM colony`); day != 22 {
		t.Fatalf("colony day = %d", day)
	}
	if food := countRows(t, database, `SELECT amount FROM resource WHERE key = 'food'`); food != 63 {
		t.Fatalf("food = %d", food)
	}
	if timezone := countRows(t, database, `SELECT count(*) FROM settings WHERE timezone = 'Europe/Lisbon'`); timezone != 1 {
		t.Fatalf("the timezone wasn't kept")
	}
	// The M1 channels belonged to the unlinked legacy server; kept, they would block every save of
	// Settings, because a channel must belong to the linked server.
	if kept := countRows(t, database, `SELECT count(*) FROM settings WHERE discord_announce_channel_id IS NOT NULL
		OR discord_reminder_channel_id IS NOT NULL OR discord_dm_channel_id IS NOT NULL`); kept != 0 {
		t.Fatalf("settings kept %d rows with legacy channels", kept)
	}

	type membership struct {
		role, status  string
		characterName *string
	}
	memberships := map[uuid.UUID]membership{}
	rows, err := database.Pool.Query(testContext, `SELECT id, account_id, role, status, character_name FROM campaign_member`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var memberID, accountID uuid.UUID
		var row membership
		if err := rows.Scan(&memberID, &accountID, &row.role, &row.status, &row.characterName); err != nil {
			t.Fatal(err)
		}
		if memberID != accountID {
			t.Errorf("membership %s should reuse its account's id %s", memberID, accountID)
		}
		memberships[accountID] = row
	}
	rows.Close()
	expected := map[uuid.UUID]membership{
		author: {role: "DM", status: "ACTIVE"},
		ana:    {role: "PLAYER", status: "ACTIVE", characterName: &thorin},
		emil:   {role: "PLAYER", status: "PENDING"},
		gus:    {role: "PLAYER", status: "REMOVED"},
	}
	for accountID, want := range expected {
		got, found := memberships[accountID]
		if !found || got.role != want.role || got.status != want.status || (want.characterName == nil) != (got.characterName == nil) {
			t.Errorf("membership of %s = %+v, want %+v", accountID, got, want)
		}
	}
	if count := countRows(t, database, `SELECT count(*) FROM timeline_event WHERE created_by = $1`, author); count != 1 {
		t.Fatalf("the founding entry lost its author: %d", count)
	}
	if count := countRows(t, database, `SELECT count(*) FROM web_session WHERE account_id = $1`, ana); count != 1 {
		t.Fatalf("Ana's session didn't survive: %d", count)
	}
	if count := countRows(t, database, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'account' AND column_name IN ('status', 'character_name', 'approved_at')`); count != 0 {
		t.Fatalf("account still has %d per-campaign columns", count)
	}
	if count := countRows(t, database, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'settings' AND column_name IN ('id', 'campaign_name')`); count != 0 {
		t.Fatalf("settings still has its singleton id or campaign_name")
	}

	// The composite foreign key keeps created_by inside the campaign.
	if _, err := database.Pool.Exec(testContext, `
		INSERT INTO campaign (slug, name, invite_code) VALUES ('other-campaign', 'Other', repeat('a', 32))`); err != nil {
		t.Fatalf("insert a second campaign: %v", err)
	}
	if _, err := database.Pool.Exec(testContext, `
		INSERT INTO timeline_event (campaign_id, day, kind, title, visibility, created_by)
		SELECT id, 0, 'CUSTOM', 'Cross', 'PUBLIC', $1 FROM campaign WHERE slug = 'other-campaign'`, author); err == nil {
		t.Fatal("a timeline entry in one campaign named a member of another")
	}
	if _, err := database.Pool.Exec(testContext, `UPDATE campaign_member SET role = 'DM' WHERE id = $1`, ana); err == nil {
		t.Fatal("a second DM was stored")
	}
}

func TestMigration0003SlugsAreValidWithoutATrailingDash(t *testing.T) {
	testCases := map[string]string{
		strings.Repeat("abc ", 15): strings.TrimSuffix(strings.Repeat("abc-", 10), "-"),
		"Ël":                       "colony",
		"   Sample Colony!!":       "sample-colony",
	}
	for campaignName, wantSlug := range testCases {
		t.Run(wantSlug, func(t *testing.T) {
			setMigrationEnvironment(t, "", "")
			database := dbtest.NewAtVersion(t, 2)
			insertLegacyMember(t, database, legacyMember{discordUserID: "seed:dm", status: "ACTIVE"})
			if _, err := database.Pool.Exec(context.Background(), `UPDATE settings SET campaign_name = $1`, campaignName); err != nil {
				t.Fatal(err)
			}
			if err := database.MigrateTo(context.Background(), 3); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			var slug string
			if err := database.Pool.QueryRow(context.Background(), `SELECT slug FROM campaign`).Scan(&slug); err != nil {
				t.Fatal(err)
			}
			if slug != wantSlug {
				t.Fatalf("slug of %q = %q, want %q", campaignName, slug, wantSlug)
			}
		})
	}
}

func TestMigration0003LeavesAnEmptyDatabaseWithoutACampaign(t *testing.T) {
	// A stale legacy variable can't break a fresh database.
	setMigrationEnvironment(t, "", authorDiscordID)
	database := dbtest.NewAtVersion(t, 2)
	if err := database.MigrateTo(context.Background(), 3); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, table := range []string{"campaign", "settings", "colony", "resource", "role", "campaign_member"} {
		if count := countRows(t, database, `SELECT count(*) FROM `+table); count != 0 {
			t.Errorf("%s has %d rows on an empty database", table, count)
		}
	}
}

func TestMigration0003RefusesAmbiguousOwnersAndChangesNothing(t *testing.T) {
	testCases := []struct {
		name               string
		siteOwnerDiscordID string
		legacyDmDiscordID  string
		accounts           []string
		wantMessage        string
	}{
		{
			name:              "a seed:dm and a real account with only the legacy variable",
			legacyDmDiscordID: authorDiscordID,
			accounts:          []string{"seed:dm", authorDiscordID},
			wantMessage:       "rename DM_DISCORD_ID to SITE_OWNER_DISCORD_ID",
		},
		{
			name:               "the two variables differ",
			siteOwnerDiscordID: authorDiscordID,
			legacyDmDiscordID:  otherDiscordID,
			accounts:           []string{authorDiscordID, otherDiscordID},
			wantMessage:        "differ",
		},
		{
			name:               "no account has the owner's Discord ID",
			siteOwnerDiscordID: otherDiscordID,
			accounts:           []string{authorDiscordID},
			wantMessage:        "no account has Discord ID " + otherDiscordID,
		},
		{
			name:        "no variable and no seed:dm account",
			accounts:    []string{authorDiscordID},
			wantMessage: "no account has Discord ID seed:dm",
		},
		{
			name:               "an owner value that isn't a Discord ID",
			siteOwnerDiscordID: "12345",
			accounts:           []string{authorDiscordID},
			wantMessage:        "must be a Discord user ID",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			setMigrationEnvironment(t, testCase.siteOwnerDiscordID, testCase.legacyDmDiscordID)
			testContext := context.Background()
			database := dbtest.NewAtVersion(t, 2)
			for _, discordUserID := range testCase.accounts {
				insertLegacyMember(t, database, legacyMember{discordUserID: discordUserID, status: "ACTIVE"})
			}
			err := database.MigrateTo(testContext, 3)
			if err == nil || !strings.Contains(err.Error(), testCase.wantMessage) {
				t.Fatalf("migrate = %v, want an error containing %q", err, testCase.wantMessage)
			}
			version, err := database.Version(testContext)
			if err != nil || version != 2 {
				t.Fatalf("version after the refusal = %d, %v; want 2", version, err)
			}
			if count := countRows(t, database, `SELECT count(*) FROM member`); count != int64(len(testCase.accounts)) {
				t.Fatalf("member rows = %d", count)
			}
			if count := countRows(t, database, `SELECT count(*) FROM information_schema.tables
				WHERE table_schema = current_schema() AND table_name IN ('campaign', 'account')`); count != 0 {
				t.Fatalf("the refused migration left %d new tables", count)
			}
		})
	}
}

func TestMigration0003ChoosesSeedDmWithNoVariables(t *testing.T) {
	setMigrationEnvironment(t, "", "")
	database := dbtest.NewAtVersion(t, 2)
	seedDm := insertLegacyMember(t, database, legacyMember{discordUserID: "seed:dm", status: "PENDING"})
	insertLegacyMember(t, database, legacyMember{discordUserID: authorDiscordID, status: "ACTIVE"})
	if err := database.MigrateTo(context.Background(), 3); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if count := countRows(t, database, `SELECT count(*) FROM campaign_member WHERE role = 'DM' AND status = 'ACTIVE' AND account_id = $1`, seedDm); count != 1 {
		t.Fatalf("seed:dm is not the DM")
	}
}
