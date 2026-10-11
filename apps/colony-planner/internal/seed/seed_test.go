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
