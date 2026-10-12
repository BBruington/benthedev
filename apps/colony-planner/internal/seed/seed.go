// Package seed loads the fictional sample campaigns that `colony seed` puts in a local database (spec
// §9; docs/design/multi-campaign.md §7.4). It grows with each milestone, and plays history through internal/core wherever an operation
// exists, so seeded timelines look exactly like real play.
package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BBruington/colony-planner/db"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

type Options struct {
	Pool *pgxpool.Pool
	Core *core.Service
	// OwnerDiscordID is SITE_OWNER_DISCORD_ID when set, so real Discord sign-in lands on the seeded
	// owner (Sample DM); otherwise core.SeedDmDiscordID.
	OwnerDiscordID string
	// Hexes is how many sample hexes to paint (M4); zero means the default handful.
	Hexes int
}

// ErrDatabaseInUse stops a seed on a database that already has data.
var ErrDatabaseInUse = errors.New("this database already has data; run `mise run reset` first")

// RequireLocalDevelopment is the safety check shared by seed and reset: both refuse production and
// any database that isn't on this machine.
func RequireLocalDevelopment(appEnvironment string, databaseURL string) error {
	if appEnvironment == "production" {
		return errors.New("refusing to run with APP_ENV=production")
	}
	if err := db.RequireLocalHost(databaseURL); err != nil {
		return fmt.Errorf("DATABASE_URL: %w", err)
	}
	return nil
}

// sampleAccount is one fictional person. Their Discord IDs start with "seed:", which no real account
// can have.
type sampleAccount struct {
	discordUserID   string
	discordUsername string
	displayName     string
	joinedDaysAgo   int
	// lastSeenDaysAgo is negative for someone who hasn't been seen since joining.
	lastSeenDaysAgo int
}

func sampleAccounts(ownerDiscordID string) []sampleAccount {
	return []sampleAccount{
		{ownerDiscordID, "sample.dm", "Sample DM", 60, 0},
		{"seed:ana", "ana.sample", "Ana", 52, 1},
		{"seed:bram", "bram.sample", "Bram", 51, 2},
		{"seed:cass", "cass.sample", "Cass", 49, 6},
		{"seed:dov", "dov.sample", "Dov", 45, 3},
		{"seed:emil", "emil.sample", "Emil", 2, 2},
		{"seed:fen", "fen.sample", "Fen", 1, 1},
		{"seed:gus", "gus.sample", "Gus", 40, 30},
		{"seed:hollis", "hollis.sample", "Hollis", 30, 0},
		{"seed:iko", "iko.sample", "Iko", 25, 4},
		{"seed:jun", "jun.sample", "Jun", 3, 3},
		{"seed:wren", "wren.sample", "Wren", 5, 1},
	}
}

// sampleMembership is one place in a sample campaign.
type sampleMembership struct {
	discordUserID string
	characterName string
	status        store.MemberStatus
	joinedDaysAgo int
}

// Sample Colony's players are M1's (so the M1 checklist's numbers hold), plus Hollis, who DMs Second
// Sample and plays Pell here.
var sampleColonyPlayers = []sampleMembership{
	{"seed:ana", "Thorin", store.MemberStatusACTIVE, 52},
	{"seed:bram", "Sela Windmere", store.MemberStatusACTIVE, 51},
	{"seed:cass", "Pip Thistledown", store.MemberStatusACTIVE, 49},
	{"seed:dov", "Maren Ashgrove", store.MemberStatusACTIVE, 45},
	{"seed:emil", "", store.MemberStatusPENDING, 2},
	{"seed:fen", "", store.MemberStatusPENDING, 1},
	{"seed:gus", "Old Rusk", store.MemberStatusREMOVED, 40},
	{"seed:hollis", "Pell", store.MemberStatusACTIVE, 28},
}

var secondSamplePlayers = []sampleMembership{
	{"seed:ana", "Brisa", store.MemberStatusACTIVE, 20},
	{"seed:iko", "Tamsin Vale", store.MemberStatusACTIVE, 24},
	{"seed:jun", "", store.MemberStatusPENDING, 3},
}

// Run fills a freshly migrated database with two sample campaigns that share players, the site owner
// and a pending campaign request. On a database that already has accounts it stops with
// ErrDatabaseInUse.
func Run(requestContext context.Context, options Options) error {
	if options.Hexes > 0 {
		return errors.New("--hexes needs the hex map, which arrives in M4")
	}
	if options.OwnerDiscordID == "" {
		return errors.New("seed: the site owner needs a Discord ID (SITE_OWNER_DISCORD_ID or core.SeedDmDiscordID)")
	}
	queries := store.New(options.Pool)
	accountCount, err := queries.CountAccounts(requestContext)
	if err != nil {
		return err
	}
	if accountCount > 0 {
		return ErrDatabaseInUse
	}

	// Accounts arrive through sign-in, which can't backdate them, so they are inserted directly.
	now := time.Now().UTC()
	accounts := map[string]store.Account{}
	err = pgx.BeginFunc(requestContext, options.Pool, func(transaction pgx.Tx) error {
		transactionQueries := queries.WithTx(transaction)
		for _, sample := range sampleAccounts(options.OwnerDiscordID) {
			account, err := transactionQueries.CreateAccount(requestContext, sampleAccountParameters(sample, now))
			if err != nil {
				return fmt.Errorf("seed account %s: %w", sample.displayName, err)
			}
			accounts[sample.discordUserID] = account
		}
		return nil
	})
	if err != nil {
		return err
	}
	callerOf := func(discordUserID string) core.Caller {
		account := accounts[discordUserID]
		return core.Caller{AccountID: account.ID, DiscordUserID: account.DiscordUserID, IsSiteOwner: discordUserID == options.OwnerDiscordID}
	}
	owner := callerOf(options.OwnerDiscordID)

	// Sample Colony: the owner's own request is approved on submit.
	sampleColony, err := createCampaign(requestContext, options.Core, owner, owner, "Sample Colony", "sample-colony")
	if err != nil {
		return err
	}
	if err := addMemberships(requestContext, queries, sampleColony, accounts, sampleColonyPlayers, now); err != nil {
		return err
	}
	dm, err := options.Core.EnterCampaign(requestContext, owner, sampleColony)
	if err != nil {
		return err
	}
	if _, err := options.Core.UpdateSettings(requestContext, dm, core.SettingsInput{
		CampaignName: "Sample Colony",
		ColonyName:   "Port Eldari",
		Timezone:     "America/New_York",
	}); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	enterSampleColony := func(discordUserID string) (core.Viewer, error) {
		return options.Core.EnterCampaign(requestContext, callerOf(discordUserID), sampleColony)
	}
	if err := seedColony(requestContext, options.Core, queries, dm, enterSampleColony); err != nil {
		return fmt.Errorf("seed the colony: %w", err)
	}

	// Second Sample, through the real request and approval path: DM Hollis.
	hollis := callerOf("seed:hollis")
	secondSample, err := createCampaign(requestContext, options.Core, owner, hollis, "Second Sample", "second-sample")
	if err != nil {
		return err
	}
	if err := addMemberships(requestContext, queries, secondSample, accounts, secondSamplePlayers, now); err != nil {
		return err
	}
	secondDm, err := options.Core.EnterCampaign(requestContext, hollis, secondSample)
	if err != nil {
		return err
	}
	enterSecondSample := func(discordUserID string) (core.Viewer, error) {
		return options.Core.EnterCampaign(requestContext, callerOf(discordUserID), secondSample)
	}
	if err := seedSecondColony(requestContext, options.Core, queries, secondDm, enterSecondSample); err != nil {
		return fmt.Errorf("seed the second colony: %w", err)
	}

	// Wren asks to run a campaign; the owner hasn't decided.
	if _, err := options.Core.RequestCampaign(requestContext, callerOf("seed:wren"), core.CampaignRequestInput{
		CampaignName: "Sample Sky Isles",
		Slug:         "sample-sky-isles",
		Note:         "My Thursday group wants a sky-island colony. Could I run it here?",
	}); err != nil {
		return fmt.Errorf("seed Wren's request: %w", err)
	}
	return nil
}

// createCampaign asks for a campaign as dm and approves it as the owner (the owner's own request is
// approved on submit), returning the new campaign's ID.
func createCampaign(requestContext context.Context, service *core.Service, owner core.Caller, dm core.Caller, name string, slug string) (uuid.UUID, error) {
	request, err := service.RequestCampaign(requestContext, dm, core.CampaignRequestInput{CampaignName: name, Slug: slug})
	if err != nil {
		return uuid.Nil, fmt.Errorf("request %s: %w", name, err)
	}
	if request.Status == core.CampaignRequestStatusPending {
		approved, err := service.ApproveCampaignRequest(requestContext, owner, request.ID, core.ApproveCampaignRequestInput{})
		if err != nil {
			return uuid.Nil, fmt.Errorf("approve %s: %w", name, err)
		}
		return approved.ID, nil
	}
	mine, err := service.MyCampaignRequest(requestContext, dm)
	if err != nil {
		return uuid.Nil, err
	}
	if mine.Request == nil || mine.Request.Campaign == nil {
		return uuid.Nil, fmt.Errorf("request %s: approved without a campaign", name)
	}
	return mine.Request.Campaign.ID, nil
}

// addMemberships inserts the sample players directly: joining through an invite can't backdate them.
func addMemberships(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, accounts map[string]store.Account, memberships []sampleMembership, now time.Time) error {
	day := 24 * time.Hour
	for _, sample := range memberships {
		createdAt := now.Add(-time.Duration(sample.joinedDaysAgo) * day)
		parameters := store.CreateCampaignMemberParams{
			CampaignID: campaignID,
			AccountID:  accounts[sample.discordUserID].ID,
			Role:       store.CampaignRolePLAYER,
			Status:     sample.status,
			CreatedAt:  &createdAt,
		}
		if sample.characterName != "" {
			characterName := sample.characterName
			parameters.CharacterName = &characterName
		}
		if sample.status != store.MemberStatusPENDING {
			approvedAt := createdAt.Add(day)
			parameters.ApprovedAt = &approvedAt
		}
		if _, err := queries.CreateCampaignMember(requestContext, parameters); err != nil {
			return fmt.Errorf("seed membership %s: %w", sample.discordUserID, err)
		}
	}
	return nil
}

// sampleHeadcounts are the role headcounts the sample colony starts with: 16 colonists, set directly
// before any history (spec §9, M1); three of them are then named (seedPeople).
var sampleHeadcounts = map[string]int32{
	"FARMER":     4,
	"HEALER":     1,
	"BLACKSMITH": 1,
	"SOLDIER":    2,
	"MINER":      2,
	"BUILDER":    3,
	"SCHOLAR":    1,
	"UNASSIGNED": 2,
}

// seedColony plays a colony some weeks in through the same core operations the API uses: founding
// stores, a 7-day shipment schedule, three weeks of advances, cargo recorded on the first arrival and a
// DM-only adjustment. It ends on Day 22 with 63 food for 19 colonists and the next ship on Day 28, and
// the last advance still in the undo slot.
func seedColony(requestContext context.Context, service *core.Service, queries *store.Queries, dm core.Viewer, enter enterAs) error {
	for key, count := range sampleHeadcounts {
		if err := queries.SetRoleUnnamedCount(requestContext, store.SetRoleUnnamedCountParams{CampaignID: dm.CampaignID(), Key: key, UnnamedCount: count}); err != nil {
			return fmt.Errorf("headcount %s: %w", key, err)
		}
	}
	if _, err := service.CreateCustomEntry(requestContext, dm, core.TimelineEntryInput{
		Day:        0,
		Title:      "Port Eldari is founded",
		Body:       "Sixteen settlers come ashore on the Eldari coast.",
		Visibility: core.TimelineVisibilityPublic,
	}); err != nil {
		return fmt.Errorf("founding entry: %w", err)
	}
	// M2's cast arrives with the founders; the named colonists are promoted from the founders' own
	// headcounts, so upkeep and every M1 number stay as they were.
	if err := seedPeople(requestContext, service, dm, sampleColonyPlaces, sampleColonyNpcs); err != nil {
		return err
	}
	version := func() (int64, error) {
		board, err := service.GetColonyBoard(requestContext, dm)
		return board.Version, err
	}
	steps := []struct {
		name string
		run  func(expectedVersion int64) (core.ColonyChange, error)
	}{
		{"founding stores", func(expectedVersion int64) (core.ColonyChange, error) {
			return service.AdjustStock(requestContext, dm, core.AdjustInput{
				ExpectedVersion: expectedVersion,
				Source:          core.AdjustSourceAdjustment,
				Deltas:          map[string]int64{},
				SetTo:           map[string]int64{"gold": 250, "food": 400, "materials": 120},
				Reason:          "The founders' stores, unloaded at landing",
				Public:          true,
			})
		}},
		{"shipment schedule", func(expectedVersion int64) (core.ColonyChange, error) {
			return service.SetShipmentSchedule(requestContext, dm, core.ShipmentScheduleInput{ExpectedVersion: expectedVersion, IntervalDays: 7})
		}},
		{"first week", advanceBy(requestContext, service, dm, 7)},
	}
	// The last of these steps is the first week, whose arrival the cargo names.
	var firstWeek core.ColonyChange
	for _, step := range steps {
		expectedVersion, err := version()
		if err != nil {
			return err
		}
		change, err := step.run(expectedVersion)
		if err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
		firstWeek = change
	}

	arrival := firstWeek.Entries[0]
	if arrival.Kind != core.TimelineKindShipmentArrived {
		return fmt.Errorf("the first week logged %s first, not a shipment arrival", arrival.Kind)
	}
	later := []struct {
		name string
		run  func(expectedVersion int64) (core.ColonyChange, error)
	}{
		{"cargo", func(expectedVersion int64) (core.ColonyChange, error) {
			return service.AdjustStock(requestContext, dm, core.AdjustInput{
				ExpectedVersion: expectedVersion,
				Source:          core.AdjustSourceShipment,
				ArrivalEntryID:  &arrival.ID,
				Deltas:          map[string]int64{"food": 60, "materials": 40},
				SetTo:           map[string]int64{},
				Colonists:       3,
				Reason:          "Grain, timber and three settlers from the mainland",
				Public:          true,
			})
		}},
		{"second week", advanceBy(requestContext, service, dm, 7)},
		{"five days", advanceBy(requestContext, service, dm, 5)},
		{"quests, patrons and requests", func(int64) (core.ColonyChange, error) {
			return core.ColonyChange{}, seedQuestsAndPatrons(requestContext, service, dm, enter)
		}},
		{"DM-only adjustment", func(expectedVersion int64) (core.ColonyChange, error) {
			return service.AdjustStock(requestContext, dm, core.AdjustInput{
				ExpectedVersion: expectedVersion,
				Source:          core.AdjustSourceAdjustment,
				Deltas:          map[string]int64{"gold": -40},
				SetTo:           map[string]int64{},
				Reason:          "Paid the smuggler at the north cove to keep quiet",
			})
		}},
		{"three days", advanceBy(requestContext, service, dm, 3)},
	}
	for _, step := range later {
		expectedVersion, err := version()
		if err != nil {
			return err
		}
		if _, err := step.run(expectedVersion); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

func advanceBy(requestContext context.Context, service *core.Service, dm core.Viewer, days int64) func(int64) (core.ColonyChange, error) {
	return func(expectedVersion int64) (core.ColonyChange, error) {
		return service.Advance(requestContext, dm, core.AdvanceInput{Days: days, ExpectedVersion: expectedVersion})
	}
}

func sampleAccountParameters(sample sampleAccount, now time.Time) store.CreateAccountParams {
	day := 24 * time.Hour
	createdAt := now.Add(-time.Duration(sample.joinedDaysAgo) * day)
	parameters := store.CreateAccountParams{
		DiscordUserID:   sample.discordUserID,
		DiscordUsername: sample.discordUsername,
		DisplayName:     sample.displayName,
		CreatedAt:       &createdAt,
	}
	if sample.lastSeenDaysAgo >= 0 {
		lastSeenAt := now.Add(-time.Duration(sample.lastSeenDaysAgo) * day)
		parameters.LastSeenAt = &lastSeenAt
	}
	return parameters
}

// secondSampleHeadcounts: six colonists on a ridge.
var secondSampleHeadcounts = map[string]int32{
	"FARMER":     2,
	"MINER":      3,
	"UNASSIGNED": 1,
}

// seedSecondColony gives Second Sample its own short history on different numbers, so isolation is
// easy to see: Ridgehold, Day 9, Food 140, an extra Timber resource and one DM-only adjustment.
func seedSecondColony(requestContext context.Context, service *core.Service, queries *store.Queries, dm core.Viewer, enter enterAs) error {
	if _, err := service.UpdateSettings(requestContext, dm, core.SettingsInput{
		CampaignName: "Second Sample",
		ColonyName:   "Ridgehold",
		Timezone:     "Europe/London",
	}); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	for key, count := range secondSampleHeadcounts {
		if err := queries.SetRoleUnnamedCount(requestContext, store.SetRoleUnnamedCountParams{CampaignID: dm.CampaignID(), Key: key, UnnamedCount: count}); err != nil {
			return fmt.Errorf("headcount %s: %w", key, err)
		}
	}
	if _, err := service.CreateResource(requestContext, dm, core.NewResourceInput{Label: "Timber", Icon: core.ColonyIconTrees}); err != nil {
		return fmt.Errorf("timber: %w", err)
	}
	if _, err := service.CreateCustomEntry(requestContext, dm, core.TimelineEntryInput{
		Day:        0,
		Title:      "Ridgehold is founded",
		Body:       "Six settlers climb to the ridge above the old mine.",
		Visibility: core.TimelineVisibilityPublic,
	}); err != nil {
		return fmt.Errorf("founding entry: %w", err)
	}
	if err := seedPeople(requestContext, service, dm, secondSamplePlaces, secondSampleNpcs); err != nil {
		return err
	}
	steps := []struct {
		name string
		run  func(expectedVersion int64) (core.ColonyChange, error)
	}{
		{"founding stores", func(expectedVersion int64) (core.ColonyChange, error) {
			return service.AdjustStock(requestContext, dm, core.AdjustInput{
				ExpectedVersion: expectedVersion,
				Source:          core.AdjustSourceAdjustment,
				Deltas:          map[string]int64{},
				SetTo:           map[string]int64{"gold": 90, "food": 194, "materials": 40, "timber": 25},
				Reason:          "What the mule train carried up",
				Public:          true,
			})
		}},
		{"nine days", advanceBy(requestContext, service, dm, 9)},
		{"DM-only adjustment", func(expectedVersion int64) (core.ColonyChange, error) {
			return service.AdjustStock(requestContext, dm, core.AdjustInput{
				ExpectedVersion: expectedVersion,
				Source:          core.AdjustSourceAdjustment,
				Deltas:          map[string]int64{"materials": -10},
				SetTo:           map[string]int64{},
				Reason:          "The ridge mine is empty",
			})
		}},
	}
	for _, step := range steps {
		board, err := service.GetColonyBoard(requestContext, dm)
		if err != nil {
			return err
		}
		if _, err := step.run(board.Version); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return seedSecondQuestsAndPatrons(requestContext, service, dm, enter)
}
