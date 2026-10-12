package seed

import (
	"context"
	"errors"
	"log/slog"
	"slices"
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
	if err != nil || len(playerTimeline.Entries) != len(dmTimeline.Entries)-3 {
		t.Fatalf("the player sees %d of %d entries; want all but the DM-only adjustment, the hidden colonist's joining and Lady Vex's favor adjustment (%v)",
			len(playerTimeline.Entries), len(dmTimeline.Entries), err)
	}

	// M2: the cast and places, some hidden, and three founders named in their roles.
	for _, check := range []struct {
		viewer        core.Viewer
		npcs, places  int
		hiddenVisible bool
	}{{dm, 10, 5, true}, {player, 7, 4, false}} {
		roster, err := service.ListNpcs(testContext, check.viewer, core.NpcFilter{})
		if err != nil || len(roster.Npcs) != check.npcs {
			t.Fatalf("roster: %d NPCs, want %d (%v)", len(roster.Npcs), check.npcs, err)
		}
		places, err := service.ListPlaces(testContext, check.viewer, "")
		if err != nil || len(places.Places) != check.places {
			t.Fatalf("places: %d, want %d (%v)", len(places.Places), check.places, err)
		}
		found, err := service.ListNpcs(testContext, check.viewer, core.NpcFilter{Search: "Edric"})
		if err != nil || (len(found.Npcs) == 1) != check.hiddenVisible {
			t.Fatalf("searching a hidden NPC's true name found %d (%v)", len(found.Npcs), err)
		}
	}
	colonists, err := service.ListNpcs(testContext, dm, core.NpcFilter{ColonistsOnly: true})
	if err != nil || len(colonists.Npcs) != 3 {
		t.Fatalf("named colonists = %d, want 3 (%v)", len(colonists.Npcs), err)
	}

	// M3: quests in every state, Lady Vex on favor 5 (Example G), and a request in every state.
	for _, check := range []struct {
		viewer                          core.Viewer
		quests, patrons, actions, asked int
	}{{dm, 4, 3, 5, 4}, {player, 3, 1, 1, 4}} {
		quests, err := service.ListQuests(testContext, check.viewer)
		if err != nil || len(quests.Quests) != check.quests {
			t.Fatalf("quests: %d, want %d (%v)", len(quests.Quests), check.quests, err)
		}
		patrons, err := service.ListPatrons(testContext, check.viewer)
		if err != nil || len(patrons.Patrons) != check.patrons || patrons.Patrons[0].Name != "Lady Vex" || patrons.Patrons[0].Favor != 5 {
			t.Fatalf("patrons: %+v (%v)", patrons, err)
		}
		actions, err := service.ListPatronActions(testContext, check.viewer)
		if err != nil || len(actions) != check.actions {
			t.Fatalf("actions: %d, want %d (%v)", len(actions), check.actions, err)
		}
		requests, err := service.ListRequests(testContext, check.viewer, core.RequestFilter{})
		if err != nil || len(requests.Requests) != check.asked || requests.Requests[0].Status != core.RequestStatusPending {
			t.Fatalf("requests: %+v (%v)", requests, err)
		}
		statuses := map[core.RequestStatus]int{}
		for _, request := range requests.Requests {
			statuses[request.Status]++
			// Sela's decline note reaches only Sela and the DM, so Ana never reads it.
			if request.Status == core.RequestStatusDeclined && (request.DecisionNote != nil) != check.viewer.IsDM() {
				t.Fatalf("the decline note for this viewer = %v", request.DecisionNote)
			}
		}
		if statuses[core.RequestStatusApproved] != 1 || statuses[core.RequestStatusDeclined] != 1 || statuses[core.RequestStatusWithdrawn] != 1 {
			t.Fatalf("request statuses = %v", statuses)
		}
	}
	sela, err := service.EnterCampaign(testContext, callerOf("seed:bram"), sampleColony.Campaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	selaRequests, err := service.ListRequests(testContext, sela, core.RequestFilter{})
	if err != nil {
		t.Fatal(err)
	}
	declinedIndex := slices.IndexFunc(selaRequests.Requests, func(request core.Request) bool { return request.Status == core.RequestStatusDeclined })
	if declinedIndex < 0 || selaRequests.Requests[declinedIndex].DecisionNote == nil || *selaRequests.Requests[declinedIndex].DecisionNote != "Vex is busy this week." {
		t.Fatalf("Sela doesn't read her own decline note: %+v", selaRequests.Requests)
	}
	questStatuses := map[core.QuestStatus]int{}
	dmQuests, err := service.ListQuests(testContext, dm)
	if err != nil {
		t.Fatal(err)
	}
	for _, quest := range dmQuests.Quests {
		questStatuses[quest.Status]++
		if quest.Title == "Clear the Wolf Den" && (quest.RewardFavor != 2 || quest.Dm.FailureFavor != 2 || quest.Patron == nil) {
			t.Fatalf("Clear the Wolf Den = %+v", quest)
		}
	}
	if questStatuses[core.QuestStatusActive] != 2 || questStatuses[core.QuestStatusCompleted] != 1 || questStatuses[core.QuestStatusFailed] != 1 {
		t.Fatalf("quest statuses = %v", questStatuses)
	}
	var failedEntry bool
	for _, entry := range playerTimeline.Entries {
		failedEntry = failedEntry || (entry.Kind == core.TimelineKindQuestFailed && entry.Body == "Food −7 · Lady Vex favor −1")
	}
	if !failedEntry {
		t.Fatal("the failed quest's cost is not in the players' history")
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
	secondPatrons, err := service.ListPatrons(testContext, secondDm)
	if err != nil || len(secondPatrons.Patrons) != 1 || secondPatrons.Patrons[0].Favor != 3 {
		t.Fatalf("Second Sample's patrons = %+v, %v", secondPatrons, err)
	}
	secondRequests, err := service.ListRequests(testContext, secondDm, core.RequestFilter{})
	if err != nil || len(secondRequests.Requests) != 1 || secondRequests.Requests[0].Status != core.RequestStatusPending {
		t.Fatalf("Second Sample's requests = %+v, %v", secondRequests, err)
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
