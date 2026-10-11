package seed

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

func TestSeedFillsAFreshDatabaseOnce(t *testing.T) {
	database := dbtest.New(t)
	testContext := context.Background()
	service := core.New(core.Options{Pool: database.Pool, Logger: slog.New(slog.DiscardHandler), DmDiscordID: core.SeedDmDiscordID})
	options := Options{Pool: database.Pool, Core: service, DmDiscordID: core.SeedDmDiscordID}

	if err := Run(testContext, options); err != nil {
		t.Fatalf("seed a fresh database: %v", err)
	}

	queries := store.New(database.Pool)
	members, err := queries.ListMembers(testContext)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	counts := map[store.MemberStatus]int{}
	characterNames := map[string]bool{}
	for _, member := range members {
		counts[member.Status]++
		if member.CharacterName != nil {
			characterNames[*member.CharacterName] = true
		}
		if member.DiscordUserID[:5] != "seed:" {
			t.Errorf("member %s has a Discord ID a real account could have: %s", member.DisplayName, member.DiscordUserID)
		}
	}
	// The DM plus four players are active; two wait; one was removed (spec §9, M0).
	if counts[store.MemberStatusACTIVE] != 5 || counts[store.MemberStatusPENDING] != 2 || counts[store.MemberStatusREMOVED] != 1 {
		t.Fatalf("member counts = %v", counts)
	}
	if !characterNames["Thorin"] {
		t.Fatal("Thorin is missing from the sample players")
	}

	dm, err := service.ViewerByDiscordID(testContext, core.SeedDmDiscordID)
	if err != nil || !dm.IsDM {
		t.Fatalf("the seeded DM is not the DM: %+v, %v", dm, err)
	}
	settings, err := service.GetSettings(testContext, dm)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if settings.CampaignName != "Sample Colony" || settings.Timezone != "America/New_York" {
		t.Fatalf("settings = %+v", settings)
	}

	// M1: a colony some weeks in, played through core (spec §9).
	board, err := service.GetColonyBoard(testContext, dm)
	if err != nil {
		t.Fatalf("read the board: %v", err)
	}
	if board.ColonyName != "Port Eldari" || board.Day != 22 || board.Colonists != 19 || board.Shipments == nil || board.Shipments.NextDay != 28 {
		t.Fatalf("board = %+v", board)
	}
	amounts := map[string]int64{}
	for _, resource := range board.Resources {
		amounts[resource.Key] = resource.Amount
	}
	if amounts["food"] != 63 || amounts["gold"] != 210 || amounts["materials"] != 160 || amounts["morale"] != 100 {
		t.Fatalf("stock = %v", amounts)
	}
	if board.Dm == nil || board.Dm.LastChange == nil || board.Dm.LastChange.Label != "3 days pass · Day 22" {
		t.Fatalf("undo slot = %+v; want the last advance", board.Dm)
	}
	dmTimeline, err := service.ListTimeline(testContext, dm, core.TimelineRangeInput{FromDay: 0, ToDay: 30})
	if err != nil {
		t.Fatalf("read the timeline: %v", err)
	}
	arrivals := map[string]bool{}
	var cargoLinked, dmOnly bool
	for _, entry := range dmTimeline.Entries {
		switch entry.Kind {
		case core.TimelineKindShipmentArrived:
			arrivals[entry.ID.String()] = true
		case core.TimelineKindCargoRecorded:
			cargoLinked = entry.ArrivalEntryID != nil && arrivals[entry.ArrivalEntryID.String()]
		case core.TimelineKindStockAdjusted:
			dmOnly = dmOnly || entry.Dm.Visibility == core.TimelineVisibilityDm
		}
	}
	if len(arrivals) != 3 || !cargoLinked || !dmOnly {
		t.Fatalf("history: %d arrivals, cargo linked %v, DM-only adjustment %v", len(arrivals), cargoLinked, dmOnly)
	}
	player, err := service.ViewerByDiscordID(testContext, "seed:ana")
	if err != nil {
		t.Fatalf("find a player: %v", err)
	}
	playerTimeline, err := service.ListTimeline(testContext, player, core.TimelineRangeInput{FromDay: 0, ToDay: 30})
	if err != nil {
		t.Fatalf("read the player's timeline: %v", err)
	}
	if len(playerTimeline.Entries) != len(dmTimeline.Entries)-1 {
		t.Fatalf("the player sees %d of %d entries; want all but the DM-only adjustment", len(playerTimeline.Entries), len(dmTimeline.Entries))
	}

	if err := Run(testContext, options); !errors.Is(err, ErrDatabaseInUse) {
		t.Fatalf("seeding twice = %v, want ErrDatabaseInUse", err)
	}
}

func TestSeedUsesTheConfiguredDm(t *testing.T) {
	database := dbtest.New(t)
	testContext := context.Background()
	realDmID := "800000000000000008"
	service := core.New(core.Options{Pool: database.Pool, Logger: slog.New(slog.DiscardHandler), DmDiscordID: realDmID})
	if err := Run(testContext, Options{Pool: database.Pool, Core: service, DmDiscordID: realDmID}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	dm, err := service.ViewerByDiscordID(testContext, realDmID)
	if err != nil || !dm.IsDM {
		t.Fatalf("with DM_DISCORD_ID set, the seeded DM should carry it: %+v, %v", dm, err)
	}
}

func TestSeedRefusesHexesBeforeTheMapExists(t *testing.T) {
	if err := Run(context.Background(), Options{Hexes: 5000, DmDiscordID: core.SeedDmDiscordID}); err == nil {
		t.Fatal("--hexes was accepted before M4")
	}
}

func TestRequireLocalDevelopment(t *testing.T) {
	for _, testCase := range []struct {
		appEnvironment string
		databaseURL    string
		allowed        bool
	}{
		{"development", "postgres://colony@127.0.0.1:54329/colony", true},
		{"test", "postgres://colony@localhost:54329/colony_test", true},
		{"production", "postgres://colony@127.0.0.1:54329/colony", false},
		{"development", "postgres://colony@ep-example.neon.tech/colony?sslmode=require", false},
		{"development", "postgres://colony@10.0.0.5:5432/colony", false},
		// pgx lets these override the host the URL appears to name.
		{"development", "postgres://colony@127.0.0.1:54329/colony?host=db.example.invalid", false},
		{"development", "postgres://colony@localhost:54329/colony?sslmode=disable&host=db.example.invalid", false},
		{"development", "postgres://colony@localhost:54329,db.example.invalid:5432/colony", false},
		{"development", "host=db.example.invalid port=5432 user=colony dbname=colony", false},
		{"development", "host=localhost port=54329 user=colony dbname=colony", true},
		{"development", "postgres://colony@127.0.0.1:54329/colony?host=localhost", true},
		{"development", "postgres://colony@localhost:54329/colony?service=colony-remote", false},
		{"development", "not a database address ::", false},
	} {
		err := RequireLocalDevelopment(testCase.appEnvironment, testCase.databaseURL)
		if (err == nil) != testCase.allowed {
			t.Errorf("%s against %s: allowed = %v, want %v", testCase.appEnvironment, testCase.databaseURL, err == nil, testCase.allowed)
		}
	}
}
