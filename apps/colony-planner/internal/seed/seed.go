// Package seed loads the fictional sample campaign that `colony seed` puts in a local database (spec
// §9). It grows with each milestone, and plays history through internal/core wherever an operation
// exists, so seeded timelines look exactly like real play.
package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BBruington/colony-planner/db"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

type Options struct {
	Pool *pgxpool.Pool
	Core *core.Service
	// DmDiscordID is DM_DISCORD_ID when set, so real Discord sign-in lands on the seeded DM.
	DmDiscordID string
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

// sampleMember is one fictional person. Their Discord IDs start with "seed:", which no real account
// can have.
type sampleMember struct {
	discordUserID   string
	discordUsername string
	displayName     string
	characterName   string
	status          store.MemberStatus
	joinedDaysAgo   int
	// lastSeenDaysAgo is negative for someone who hasn't been seen since joining.
	lastSeenDaysAgo int
}

func sampleMembers(dmDiscordID string) []sampleMember {
	return []sampleMember{
		{dmDiscordID, "sample.dm", "Sample DM", "", store.MemberStatusACTIVE, 60, 0},
		{"seed:ana", "ana.sample", "Ana", "Thorin", store.MemberStatusACTIVE, 52, 1},
		{"seed:bram", "bram.sample", "Bram", "Sela Windmere", store.MemberStatusACTIVE, 51, 2},
		{"seed:cass", "cass.sample", "Cass", "Pip Thistledown", store.MemberStatusACTIVE, 49, 6},
		{"seed:dov", "dov.sample", "Dov", "Maren Ashgrove", store.MemberStatusACTIVE, 45, 3},
		{"seed:emil", "emil.sample", "Emil", "", store.MemberStatusPENDING, 2, 2},
		{"seed:fen", "fen.sample", "Fen", "", store.MemberStatusPENDING, 1, 1},
		{"seed:gus", "gus.sample", "Gus", "Old Rusk", store.MemberStatusREMOVED, 40, 30},
	}
}

// Run fills a freshly migrated database. On a database that already has data it stops with
// ErrDatabaseInUse.
func Run(requestContext context.Context, options Options) error {
	if options.Hexes > 0 {
		return errors.New("--hexes needs the hex map, which arrives in M4")
	}
	if options.DmDiscordID == "" {
		return errors.New("seed: the DM needs a Discord ID (DM_DISCORD_ID or core.SeedDmDiscordID)")
	}
	queries := store.New(options.Pool)
	memberCount, err := queries.CountMembers(requestContext)
	if err != nil {
		return err
	}
	if memberCount > 0 {
		return ErrDatabaseInUse
	}

	// Members have no create operation (they arrive through sign-in), so they are inserted directly.
	var dm store.Member
	err = pgx.BeginFunc(requestContext, options.Pool, func(transaction pgx.Tx) error {
		transactionQueries := queries.WithTx(transaction)
		now := time.Now().UTC()
		for _, sample := range sampleMembers(options.DmDiscordID) {
			member, err := transactionQueries.CreateMember(requestContext, sampleMemberParameters(sample, now))
			if err != nil {
				return fmt.Errorf("seed member %s: %w", sample.displayName, err)
			}
			if sample.discordUserID == options.DmDiscordID {
				dm = member
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	dmViewer := options.Core.NewViewer(core.ViewerIdentity{
		MemberID:      dm.ID,
		DiscordUserID: dm.DiscordUserID,
		Status:        dm.Status,
	})
	if _, err := options.Core.UpdateSettings(requestContext, dmViewer, core.SettingsInput{
		CampaignName: "Sample Colony",
		ColonyName:   "Port Eldari",
		Timezone:     "America/New_York",
	}); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	if err := seedColony(requestContext, options.Core, queries, dmViewer); err != nil {
		return fmt.Errorf("seed the colony: %w", err)
	}
	return nil
}

// sampleHeadcounts are the role headcounts the sample colony starts with: 16 colonists. There is no
// workforce operation until M2, so they are set directly (spec §9, M1).
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
func seedColony(requestContext context.Context, service *core.Service, queries *store.Queries, dm core.Viewer) error {
	for key, count := range sampleHeadcounts {
		if err := queries.SetRoleUnnamedCount(requestContext, store.SetRoleUnnamedCountParams{Key: key, UnnamedCount: count}); err != nil {
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

func sampleMemberParameters(sample sampleMember, now time.Time) store.CreateMemberParams {
	day := 24 * time.Hour
	createdAt := now.Add(-time.Duration(sample.joinedDaysAgo) * day)
	parameters := store.CreateMemberParams{
		DiscordUserID:   sample.discordUserID,
		DiscordUsername: sample.discordUsername,
		DisplayName:     sample.displayName,
		Status:          sample.status,
		CreatedAt:       &createdAt,
	}
	if sample.characterName != "" {
		characterName := sample.characterName
		parameters.CharacterName = &characterName
	}
	if sample.status != store.MemberStatusPENDING {
		approvedAt := createdAt.Add(day)
		parameters.ApprovedAt = &approvedAt
	}
	if sample.lastSeenDaysAgo >= 0 {
		lastSeenAt := now.Add(-time.Duration(sample.lastSeenDaysAgo) * day)
		parameters.LastSeenAt = &lastSeenAt
	}
	return parameters
}
