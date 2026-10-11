package app

import (
	"slices"
	"testing"

	"github.com/BBruington/colony-planner/internal/core"
)

// campaignSide is how one campaign of the harness is planted. Campaign A's player-visible values carry
// no token (its players see them); campaign B's carry canaryB, so a reader outside B is caught seeing
// any of B. DM-only values carry canaryA in A and canaryBdm in B.
type campaignSide struct {
	campaign *campaignFixture
	dm       account
	// dmSecret marks DM-only values; playerVisible is appended to values every member may see.
	dmSecret      string
	playerVisible string
	guildID       string
	channelID     string
	colonyName    string
	// characterNames maps an account name to the character name it plays here.
	characterNames map[string]string
	// memberTarget is the account whose membership member-targeted routes act on.
	memberTarget string
	// extraResource is a resource only this campaign has ("" for none); resourceKey names it, or gold.
	extraResource string
}

func (testHarness *harness) sides() []campaignSide {
	return []campaignSide{
		{
			campaign:       testHarness.campaignA,
			dm:             testHarness.dmA,
			dmSecret:       canaryA,
			guildID:        guildA,
			channelID:      channelA,
			colonyName:     "Port Alpha",
			characterNames: map[string]string{"ana": "Thorin", "otherA": "Sela", "dmB": "Pell"},
			memberTarget:   "otherA",
		},
		{
			campaign:       testHarness.campaignB,
			dm:             testHarness.dmB,
			dmSecret:       canaryBdm,
			playerVisible:  " " + canaryB,
			guildID:        guildB,
			channelID:      channelB,
			colonyName:     "Ridgehold " + canaryB,
			characterNames: map[string]string{"ana": "Brisa " + canaryB, "dmA": "Vale " + canaryB, "bOnly": "Tamsin " + canaryB},
			memberTarget:   "bOnly",
			extraResource:  "Brine",
		},
	}
}

// secrecyFixture is one feature's share of the sweep (secrecy_test.go). The sweep can only catch a
// leak of a value that holds a token, and can only reach a handler's data path when every path
// parameter names a real row, so each feature that adds a Dm field, a hidden row or a path parameter
// adds or extends an entry here, planting into both campaigns:
//   - plant writes side.dmSecret into every DM-only value the feature stores (each field of its Dm
//     sub-struct, each row players must not see) and side.playerVisible into the values every member
//     sees, and records a real row of the campaign for each path parameter its routes use.
//   - dmProof lists GET path suffixes under /api/campaigns/{campaignId} on which the campaign's DM must
//     see a DM-only value, so TestDmSeesTheCanaries proves the planting happened and the sweep can fail.
type secrecyFixture struct {
	feature string
	plant   func(t *testing.T, testHarness *harness, side campaignSide)
	dmProof []string
}

var secrecyFixtures = []secrecyFixture{
	{
		feature: "members",
		// Usernames are DM-only (MemberDm) and newHarness plants them; character names are seen by
		// every member, so B's carry canaryB.
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			dmViewer := testHarness.enter(side.dm, side.campaign.id)
			for accountName, characterName := range side.characterNames {
				if _, err := testHarness.core.SetCharacterName(testHarness.context, dmViewer, side.campaign.members[accountName], core.CharacterNameInput{CharacterName: &characterName}); err != nil {
					t.Fatalf("name %s's character in %s: %v", accountName, side.campaign.name, err)
				}
			}
			side.campaign.pathIDs["memberId"] = side.campaign.members[side.memberTarget].String()
		},
		dmProof: []string{"/members"},
	},
	{
		feature: "settings and Discord",
		// The linked server, its channels and every /link code are DM-only.
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			dmViewer := testHarness.enter(side.dm, side.campaign.id)
			code, err := testHarness.core.CreateLinkCode(testHarness.context, dmViewer)
			if err != nil {
				t.Fatalf("link code: %v", err)
			}
			side.campaign.linkCodes = append(side.campaign.linkCodes, code.Code)
			testHarness.directory.mutex.Lock()
			testHarness.directory.channels[side.guildID] = []core.DiscordChannel{{ID: side.channelID, Name: "dm-room-" + side.dmSecret}}
			testHarness.directory.mutex.Unlock()
			if _, err := testHarness.core.LinkGuild(testHarness.context, core.LinkGuildInput{
				Code:          code.Code,
				GuildID:       side.guildID,
				GuildName:     "Server " + side.dmSecret,
				DiscordUserID: side.dm.caller.DiscordUserID,
			}); err != nil {
				t.Fatalf("link %s's server: %v", side.campaign.name, err)
			}
			channelID := side.channelID
			if _, err := testHarness.core.UpdateSettings(testHarness.context, dmViewer, core.SettingsInput{
				CampaignName:             side.campaign.name,
				ColonyName:               side.colonyName,
				Timezone:                 "UTC",
				DiscordAnnounceChannelID: &channelID,
				DiscordReminderChannelID: &channelID,
				DiscordDmChannelID:       &channelID,
			}); err != nil {
				t.Fatalf("plant %s's settings: %v", side.campaign.name, err)
			}
			// A live, unused code too: the sweep forbids every code ever issued.
			live, err := testHarness.core.CreateLinkCode(testHarness.context, dmViewer)
			if err != nil {
				t.Fatalf("live link code: %v", err)
			}
			side.campaign.linkCodes = append(side.campaign.linkCodes, live.Code)
		},
		dmProof: []string{"/settings", "/discord/channels"},
	},
	{
		feature: "colony",
		// A shipment arrival (for cargo bodies), a PUBLIC adjustment that is then undone and a DM-only
		// adjustment, whose reasons are DM-only; B also gets a resource of its own.
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			dmViewer := testHarness.enter(side.dm, side.campaign.id)
			version := func() int64 {
				board, err := testHarness.core.GetColonyBoard(testHarness.context, dmViewer)
				if err != nil {
					t.Fatalf("read %s's board: %v", side.campaign.name, err)
				}
				return board.Version
			}
			resourceKey := "gold"
			if side.extraResource != "" {
				created, err := testHarness.core.CreateResource(testHarness.context, dmViewer, core.NewResourceInput{Label: side.extraResource, Icon: core.ColonyIconDroplets})
				if err != nil {
					t.Fatalf("add %s: %v", side.extraResource, err)
				}
				if _, err := testHarness.core.UpdateResource(testHarness.context, dmViewer, created.Key, core.ResourceInput{
					Label:     side.extraResource + side.playerVisible,
					Icon:      core.ColonyIconDroplets,
					SortOrder: created.SortOrder,
				}); err != nil {
					t.Fatalf("label %s: %v", side.extraResource, err)
				}
				resourceKey = created.Key
			}
			side.campaign.pathIDs["resourceKey"] = resourceKey
			if _, err := testHarness.core.SetShipmentSchedule(testHarness.context, dmViewer, core.ShipmentScheduleInput{ExpectedVersion: version(), IntervalDays: 1}); err != nil {
				t.Fatalf("schedule shipments: %v", err)
			}
			advanced, err := testHarness.core.Advance(testHarness.context, dmViewer, core.AdvanceInput{Days: 1, ExpectedVersion: version()})
			if err != nil {
				t.Fatalf("advance: %v", err)
			}
			arrival := slices.IndexFunc(advanced.Entries, func(entry core.TimelineEntry) bool {
				return entry.Kind == core.TimelineKindShipmentArrived
			})
			if arrival < 0 {
				t.Fatalf("advancing %s brought no shipment: %+v", side.campaign.name, advanced.Entries)
			}
			side.campaign.arrivalEntryID = advanced.Entries[arrival].ID
			undone, err := testHarness.core.AdjustStock(testHarness.context, dmViewer, core.AdjustInput{
				ExpectedVersion: version(),
				Source:          core.AdjustSourceAdjustment,
				Deltas:          map[string]int64{resourceKey: 3},
				SetTo:           map[string]int64{},
				Reason:          side.dmSecret + " undone",
				Public:          true,
			})
			if err != nil {
				t.Fatalf("plant a public adjustment: %v", err)
			}
			if _, err := testHarness.core.UndoLastChange(testHarness.context, dmViewer, core.UndoInput{GroupID: undone.GroupID}); err != nil {
				t.Fatalf("undo it: %v", err)
			}
			if _, err := testHarness.core.AdjustStock(testHarness.context, dmViewer, core.AdjustInput{
				ExpectedVersion: version(),
				Source:          core.AdjustSourceAdjustment,
				Deltas:          map[string]int64{"gold": 7},
				SetTo:           map[string]int64{},
				Reason:          side.dmSecret + " paid the smuggler",
			}); err != nil {
				t.Fatalf("plant a DM-only adjustment: %v", err)
			}
		},
		dmProof: []string{"/timeline/recent", "/timeline?fromDay=0&toDay=30"},
	},
	{
		feature: "timeline",
		// A PUBLIC custom entry (the fixture row for {timelineEntryId}) and a DM-visibility one.
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			dmViewer := testHarness.enter(side.dm, side.campaign.id)
			public, err := testHarness.core.CreateCustomEntry(testHarness.context, dmViewer, core.TimelineEntryInput{
				Day:        1,
				Title:      "Gold found in the river" + side.playerVisible,
				Visibility: core.TimelineVisibilityPublic,
			})
			if err != nil {
				t.Fatalf("plant a public entry: %v", err)
			}
			side.campaign.pathIDs["timelineEntryId"] = public.ID.String()
			if _, err := testHarness.core.CreateCustomEntry(testHarness.context, dmViewer, core.TimelineEntryInput{
				Day:        1,
				Title:      side.dmSecret + " DM-only entry",
				Visibility: core.TimelineVisibilityDm,
			}); err != nil {
				t.Fatalf("plant a DM-only entry: %v", err)
			}
		},
		dmProof: []string{"/timeline?fromDay=0&toDay=30"},
	},
	{
		feature: "invite link",
		// The invite code is DM-only: the harness's forbidden lists hold it as a literal.
		plant:   func(*testing.T, *harness, campaignSide) {},
		dmProof: []string{"/invite"},
	},
}

// plantCanaries runs every feature's fixture in both campaigns.
func plantCanaries(t *testing.T, testHarness *harness) {
	t.Helper()
	for _, side := range testHarness.sides() {
		for _, fixture := range secrecyFixtures {
			fixture.plant(t, testHarness, side)
		}
		side.campaign.pathIDs["campaignId"] = side.campaign.id.String()
		side.campaign.pathIDs["campaignRequestId"] = side.campaign.requestID.String()
		side.campaign.pathIDs["inviteCode"] = side.campaign.inviteCode
	}
}

// dmOnlyValues are the values only a campaign's DM may ever see.
func (testHarness *harness) dmOnlyValues(campaign *campaignFixture) []string {
	values := []string{campaign.inviteCode}
	values = append(values, campaign.linkCodes...)
	if campaign == testHarness.campaignA {
		return append(values, canaryA, guildA, channelA)
	}
	return append(values, canaryBdm, guildB, channelB)
}

// refreshInviteCode re-reads a campaign's invite code after a sweep pass may have reset it, keeping the
// old one forbidden too.
func (testHarness *harness) refreshInviteCode(campaign *campaignFixture, dm account) {
	testHarness.t.Helper()
	invite, err := testHarness.core.Invite(testHarness.context, testHarness.enter(dm, campaign.id))
	if err != nil {
		testHarness.t.Fatalf("read the invite: %v", err)
	}
	if invite.InviteCode != campaign.inviteCode {
		campaign.linkCodes = append(campaign.linkCodes, campaign.inviteCode)
		campaign.inviteCode = invite.InviteCode
	}
}
