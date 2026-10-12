package seed

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
)

func TestSeedFillsAFreshDatabaseOnce(t *testing.T) {
	database := dbtest.New(t)
	testContext := context.Background()
	service := core.New(core.Options{Pool: database.Pool, Logger: slog.New(slog.DiscardHandler), SiteOwnerDiscordID: core.SeedDmDiscordID})
	options := Options{Pool: database.Pool, Core: service, OwnerDiscordID: core.SeedDmDiscordID}
	if err := Run(testContext, options); err != nil {
		t.Fatalf("seed: %v", err)
	}
	callerOf := func(discordUserID string) core.Caller {
		t.Helper()
		accountID, found, err := service.AccountIDForDiscordUser(testContext, discordUserID)
		if err != nil || !found {
			t.Fatalf("no seeded account %s: %v", discordUserID, err)
		}
		return core.Caller{AccountID: accountID, DiscordUserID: discordUserID, IsSiteOwner: service.IsSiteOwner(discordUserID)}
	}
	campaignBySlug := func(caller core.Caller) map[string]core.MyCampaign {
		t.Helper()
		campaigns, err := service.MyCampaigns(testContext, caller)
		if err != nil {
			t.Fatal(err)
		}
		bySlug := map[string]core.MyCampaign{}
		for _, campaign := range campaigns.Campaigns {
			bySlug[campaign.Campaign.Slug] = campaign
		}
		return bySlug
	}
	owner := callerOf(core.SeedDmDiscordID)
	sampleColony := campaignBySlug(owner)["sample-colony"]
	if sampleColony.Role != core.CampaignRoleDm {
		t.Fatalf("the owner's campaigns = %+v", campaignBySlug(owner))
	}
	dm, err := service.EnterCampaign(testContext, owner, sampleColony.Campaign.ID)
	if err != nil || !dm.IsDM() {
		t.Fatalf("the seeded owner is not Sample Colony's DM: %v", err)
	}
	groups, err := service.ListMembers(testContext, dm)
	if err != nil {
		t.Fatal(err)
	}
	// The DM, four M1 players and Hollis (Pell) are active; two wait; one was removed.
	if len(groups.Active) != 6 || len(groups.Pending) != 2 || len(groups.Removed) != 1 {
		t.Fatalf("Sample Colony's members: %d active, %d pending, %d removed", len(groups.Active), len(groups.Pending), len(groups.Removed))
	}
	settings, err := service.GetSettings(testContext, dm)
	if err != nil || settings.CampaignName != "Sample Colony" || settings.Timezone != "America/New_York" {
		t.Fatalf("settings = %+v, %v", settings, err)
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

	// Ana plays in both campaigns under two names.
	ana := callerOf("seed:ana")
	anaCampaigns := campaignBySlug(ana)
	if *anaCampaigns["sample-colony"].CharacterName != "Thorin" || *anaCampaigns["second-sample"].CharacterName != "Brisa" {
		t.Fatalf("Ana's campaigns = %+v", anaCampaigns)
	}
	player, err := service.EnterCampaign(testContext, ana, sampleColony.Campaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	playerTimeline, err := service.ListTimeline(testContext, player, core.TimelineRangeInput{FromDay: 0, ToDay: 30})
	if err != nil || len(playerTimeline.Entries) != len(dmTimeline.Entries)-1 {
		t.Fatalf("the player sees %d of %d entries; want all but the DM-only adjustment (%v)", len(playerTimeline.Entries), len(dmTimeline.Entries), err)
	}

	// Hollis DMs Second Sample and plays Pell in Sample Colony.
	hollis := callerOf("seed:hollis")
	hollisCampaigns := campaignBySlug(hollis)
	if hollisCampaigns["second-sample"].Role != core.CampaignRoleDm || hollisCampaigns["sample-colony"].Role != core.CampaignRolePlayer ||
		*hollisCampaigns["sample-colony"].CharacterName != "Pell" || hollisCampaigns["sample-colony"].Status != core.MemberStatusActive {
		t.Fatalf("Hollis's campaigns = %+v", hollisCampaigns)
	}
	secondDm, err := service.EnterCampaign(testContext, hollis, hollisCampaigns["second-sample"].Campaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.GetColonyBoard(testContext, secondDm)
	if err != nil {
		t.Fatal(err)
	}
	secondAmounts := map[string]int64{}
	for _, resource := range second.Resources {
		secondAmounts[resource.Key] = resource.Amount
	}
	if second.ColonyName != "Ridgehold" || second.Day != 9 || secondAmounts["food"] != 140 || secondAmounts["timber"] != 25 {
		t.Fatalf("Second Sample = %+v, %v", second, secondAmounts)
	}
	if jun := campaignBySlug(callerOf("seed:jun"))["second-sample"]; jun.Status != core.MemberStatusPending {
		t.Fatalf("Jun = %+v", jun)
	}
	requests, err := service.ListCampaignRequests(testContext, owner)
	if err != nil || len(requests.Requests) != 1 || requests.Requests[0].Request.Slug != "sample-sky-isles" || requests.Requests[0].DisplayName != "Wren" {
		t.Fatalf("open requests = %+v, %v", requests, err)
	}

	if err := Run(testContext, options); !errors.Is(err, ErrDatabaseInUse) {
		t.Fatalf("seeding twice = %v, want ErrDatabaseInUse", err)
	}
}

func TestSeedUsesTheConfiguredOwner(t *testing.T) {
	database := dbtest.New(t)
	testContext := context.Background()
	realOwnerID := "800000000000000008"
	service := core.New(core.Options{Pool: database.Pool, Logger: slog.New(slog.DiscardHandler), SiteOwnerDiscordID: realOwnerID})
	if err := Run(testContext, Options{Pool: database.Pool, Core: service, OwnerDiscordID: realOwnerID}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	accountID, found, err := service.AccountIDForDiscordUser(testContext, realOwnerID)
	if err != nil || !found {
		t.Fatalf("with SITE_OWNER_DISCORD_ID set, the seeded owner should carry it: %v", err)
	}
	campaigns, err := service.MyCampaigns(testContext, core.Caller{AccountID: accountID, DiscordUserID: realOwnerID, IsSiteOwner: true})
	if err != nil || len(campaigns.Campaigns) != 1 || campaigns.Campaigns[0].Role != core.CampaignRoleDm {
		t.Fatalf("the owner's campaigns = %+v, %v", campaigns, err)
	}
}

func TestSeedRefusesHexesBeforeTheMapExists(t *testing.T) {
	if err := Run(context.Background(), Options{Hexes: 5000, OwnerDiscordID: core.SeedDmDiscordID}); err == nil {
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
