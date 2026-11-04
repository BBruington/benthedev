package seed

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
)

func TestSeedFillsAFreshDatabaseOnce(t *testing.T) {
	database := dbtest.New(t)
	testContext := context.Background()
	service := core.New(core.Options{Pool: database.Pool, Logger: slog.New(slog.DiscardHandler), SiteOwnerDiscordID: core.SeedDmDiscordID})
	options := Options{Pool: database.Pool, Core: service, OwnerDiscordID: core.SeedDmDiscordID, MainlandArt: mainlandArt(t)}
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
	// The market: the default offers (Mages raised to 80 gp), the Boots the DM added, Thorin's order
	// out until Day 36, and Sela's cart.
	market, err := service.GetMarket(testContext, dm)
	if err != nil || market.VoyageDays != 14 {
		t.Fatalf("market = %+v, %v", market, err)
	}
	offers := map[string]int64{}
	specials := 0
	for _, item := range market.Items {
		if item.ResourceKey == nil && item.RoleKey == nil {
			specials++
		}
		offers[item.Name] = item.PriceGold
	}
	if specials != 1 || offers["Food"] != 1 || offers["Materials"] != 5 || offers["Farmer"] != 30 || offers["Mage"] != 80 || offers["Boots of Striding"] != 120 {
		t.Fatalf("market items = %+v", market.Items)
	}
	order := market.Order
	if order == nil || order.Status != core.MarketOrderStatusUnpaid || order.PlacedDay != 22 ||
		order.ArrivesDay != 36 || order.Docked || order.TotalGold != 70 || len(order.Lines) != 2 {
		t.Fatalf("the open order = %+v", market.Order)
	}
	quantities := map[string]int64{}
	for _, line := range order.Lines {
		quantities[line.Name] = line.Quantity
	}
	if quantities["Food"] != 10 || quantities["Farmer"] != 2 {
		t.Fatalf("the open order's lines = %+v", order.Lines)
	}
	if len(market.Cart.Lines) != 2 || market.Cart.TotalGold != 125 || len(market.History) != 0 {
		t.Fatalf("cart = %+v, history = %+v", market.Cart, market.History)
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

	// M4: Sample Colony's Mainland is the uploaded Eldari art with the sample pins (the hidden chapel's
	// for the DM only) and a small charted area; Second Sample has a few hexes and no map.
	for _, check := range []struct {
		viewer core.Viewer
		pins   int
	}{{dm, 5}, {player, 4}} {
		maps, err := service.ListMaps(testContext, check.viewer)
		if err != nil || len(maps.Maps) != 1 || !maps.Maps[0].IsMainland || maps.Maps[0].ImageID == nil || maps.Maps[0].Width != 1920 || maps.Maps[0].Height != 1080 {
			t.Fatalf("maps = %+v (%v)", maps, err)
		}
		mainland, err := service.GetMap(testContext, check.viewer, maps.Maps[0].ID)
		if err != nil || len(mainland.Pins) != check.pins {
			t.Fatalf("the Mainland has %d pins, want %d (%v)", len(mainland.Pins), check.pins, err)
		}
		if _, err := service.GetImage(testContext, check.viewer, *maps.Maps[0].ImageID); err != nil {
			t.Fatalf("the Mainland's art: %v", err)
		}
	}
	sampleHexes, err := service.ListHexes(testContext, player)
	if err != nil || len(sampleHexes.Hexes) != len(sampleColonyHexes) {
		t.Fatalf("Sample Colony's hexes = %d (%v)", len(sampleHexes.Hexes), err)
	}
	secondMaps, err := service.ListMaps(testContext, secondDm)
	if err != nil || len(secondMaps.Maps) != 0 {
		t.Fatalf("Second Sample's maps = %+v (%v)", secondMaps, err)
	}
	secondHexes, err := service.ListHexes(testContext, secondDm)
	if err != nil || len(secondHexes.Hexes) != len(secondSampleHexes) {
		t.Fatalf("Second Sample's hexes = %d (%v)", len(secondHexes.Hexes), err)
	}

	// M5: a weekly Friday game whose last three Fridays were played between the advances, so the
	// timeline shows bands over Days 0–7, 14–19 and 19–22, with one cancelled date ahead; Brannoc was
	// met at the first; Second Sample played its first Saturday over Days 0–9.
	history, err := service.ListPlayedSessions(testContext, player)
	if err != nil || len(history) != 3 {
		t.Fatalf("Sample Colony's played sessions = %+v, %v", history, err)
	}
	wantBands := []struct {
		title      string
		start, end int64
	}{{"The Saltmarsh Survey", 19, 22}, {"Wolves on the Coast Road", 14, 19}, {"The Landing", 0, 7}}
	for index, want := range wantBands {
		session := history[index]
		if session.Title != want.title || session.Recap == "" || session.ColonyDayStart == nil || *session.ColonyDayStart != want.start ||
			session.ColonyDayEnd == nil || *session.ColonyDayEnd != want.end {
			t.Fatalf("played session %d = %+v, want %s over Days %d–%d", index, session, want.title, want.start, want.end)
		}
	}
	bands, err := service.ListSessionBands(testContext, player, core.TimelineRangeInput{FromDay: 0, ToDay: 22})
	if err != nil || len(bands) != 3 {
		t.Fatalf("session bands = %+v, %v", bands, err)
	}
	sessionEntries := 0
	for _, entry := range playerTimeline.Entries {
		if entry.Kind == core.TimelineKindSessionPlayed && entry.SessionID != nil {
			sessionEntries++
		}
	}
	if sessionEntries != 3 {
		t.Fatalf("the player sees %d SESSION_PLAYED entries, want 3", sessionEntries)
	}
	upcoming, err := service.ListUpcomingSessions(testContext, player)
	if err != nil || len(upcoming) == 0 {
		t.Fatalf("upcoming = %+v, %v", upcoming, err)
	}
	cancelledDates := 0
	for _, session := range upcoming {
		local := session.StartsAt.In(newYork(t))
		if local.Weekday() != time.Friday || local.Hour() != 20 || local.Minute() != 0 {
			t.Fatalf("an upcoming date isn't Friday 8:00 PM in New York: %+v", session)
		}
		if session.Status == core.SessionStatusCancelled {
			cancelledDates++
		}
	}
	if cancelledDates != 1 || upcoming[0].Status == core.SessionStatusCancelled {
		t.Fatalf("upcoming = %+v; want the Friday after next cancelled", upcoming)
	}
	brannoc, err := service.ListNpcs(testContext, player, core.NpcFilter{Search: "Brannoc"})
	if err != nil || len(brannoc.Npcs) != 1 {
		t.Fatalf("Brannoc = %+v, %v", brannoc, err)
	}
	met, err := service.GetNpc(testContext, player, brannoc.Npcs[0].ID)
	if err != nil || met.FirstSession == nil || met.FirstSession.Title != "The Landing" {
		t.Fatalf("Brannoc's first session = %+v, %v", met.FirstSession, err)
	}
	secondHistory, err := service.ListPlayedSessions(testContext, secondDm)
	if err != nil || len(secondHistory) != 1 || *secondHistory[0].ColonyDayStart != 0 || *secondHistory[0].ColonyDayEnd != 9 {
		t.Fatalf("Second Sample's played sessions = %+v, %v", secondHistory, err)
	}
	secondSeries, err := service.ListSessionSeries(testContext, secondDm)
	if err != nil || len(secondSeries) != 1 || secondSeries[0].EveryWeeks == nil || *secondSeries[0].EveryWeeks != 2 {
		t.Fatalf("Second Sample's series = %+v, %v", secondSeries, err)
	}

	if err := Run(testContext, options); !errors.Is(err, ErrDatabaseInUse) {
		t.Fatalf("seeding twice = %v, want ErrDatabaseInUse", err)
	}
}

func newYork(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func TestSeedUsesTheConfiguredOwner(t *testing.T) {
	database := dbtest.New(t)
	testContext := context.Background()
	realOwnerID := "800000000000000008"
	service := core.New(core.Options{Pool: database.Pool, Logger: slog.New(slog.DiscardHandler), SiteOwnerDiscordID: realOwnerID})
	if err := Run(testContext, Options{Pool: database.Pool, Core: service, OwnerDiscordID: realOwnerID, MainlandArt: mainlandArt(t)}); err != nil {
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

// mainlandArt is the Eldari art `colony seed` reads from the repository root.
func mainlandArt(t *testing.T) []byte {
	t.Helper()
	art, err := os.ReadFile(filepath.Join("..", "..", MainlandArtPath))
	if err != nil {
		t.Fatal(err)
	}
	return art
}

// --hexes N paints N hexes in Sample Colony instead of the default handful, for the 5,000-hex check.
func TestSeedPaintsManyHexes(t *testing.T) {
	for _, hexes := range []int{-1, core.HexLimit + 1} {
		if err := Run(context.Background(), Options{Hexes: hexes, OwnerDiscordID: core.SeedDmDiscordID}); err == nil {
			t.Fatalf("--hexes %d was accepted", hexes)
		}
	}
	database := dbtest.New(t)
	testContext := context.Background()
	service := core.New(core.Options{Pool: database.Pool, Logger: slog.New(slog.DiscardHandler), SiteOwnerDiscordID: core.SeedDmDiscordID})
	const hexes = 450
	if err := Run(testContext, Options{Pool: database.Pool, Core: service, OwnerDiscordID: core.SeedDmDiscordID, Hexes: hexes, MainlandArt: mainlandArt(t)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	accountID, _, err := service.AccountIDForDiscordUser(testContext, core.SeedDmDiscordID)
	if err != nil {
		t.Fatal(err)
	}
	owner := core.Caller{AccountID: accountID, DiscordUserID: core.SeedDmDiscordID, IsSiteOwner: true}
	campaigns, err := service.MyCampaigns(testContext, owner)
	if err != nil || len(campaigns.Campaigns) != 1 {
		t.Fatalf("the owner's campaigns = %+v (%v)", campaigns, err)
	}
	dm, err := service.EnterCampaign(testContext, owner, campaigns.Campaigns[0].Campaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := service.ListHexes(testContext, dm)
	if err != nil || len(list.Hexes) != hexes {
		t.Fatalf("%d hexes, want %d (%v)", len(list.Hexes), hexes, err)
	}
	labelled := 0
	for _, hex := range list.Hexes {
		if hex.Label != "" {
			labelled++
		}
	}
	if labelled != len(sampleColonyHexLabels)+4 {
		t.Fatalf("%d labels, want the sample ones and a survey mark every %d hexes", labelled, sampleHexLabelEvery)
	}
}

// The spiral grows ring by ring: 1, then 6 more, then 12 more, each cell once.
func TestSpiralHexesGrowRingByRing(t *testing.T) {
	cells := spiralHexes(37)
	seen := map[core.HexCoordinate]bool{}
	for index, cell := range cells {
		distance := max(abs(cell.Q), abs(cell.R), abs(cell.Q+cell.R))
		wantRing := int64(0)
		switch {
		case index >= 19:
			wantRing = 3
		case index >= 7:
			wantRing = 2
		case index >= 1:
			wantRing = 1
		}
		if distance != wantRing || seen[cell] {
			t.Fatalf("cell %d = %+v (ring %d, repeated %v), want ring %d", index, cell, distance, seen[cell], wantRing)
		}
		seen[cell] = true
	}
}

func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
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
