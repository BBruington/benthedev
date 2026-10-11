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
		Timezone:     "America/New_York",
	}); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	return nil
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
