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
	// extraRole is a role only this campaign has ("" for none); roleKey names it, or FARMER.
	extraRole string
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
			extraRole:      "Brinewright",
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
		// every member, so B's carry canaryB. MemberDm's other fields (status, isDm, removalKind, the
		// times) can't hold a token, so each campaign gets a denied member whose username carries its DM
		// token: the removalKind DENIED then travels only beside a planted value (A's is removedA).
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			dmViewer := testHarness.enter(side.dm, side.campaign.id)
			if side.campaign == testHarness.campaignB {
				deniedB := testHarness.signUp("deniedB", "200000000000000003", "jun."+canaryBdm, "Jun")
				testHarness.join(side.campaign, side.dm, deniedB, "deny")
			}
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
		feature: "people, NPCs and places",
		// A revealed place (the {placeId} row) whose dm_notes are DM-only, and a hidden place whose every
		// value is. A revealed NPC (the {npcId} row, a named colonist so no sweep can delete it) whose true
		// name and dm_notes are DM-only behind a known name, standing in the hidden place. A hidden NPC
		// whose every value is DM-only, standing in the revealed place, who joins the colony and changes
		// role with PUBLIC asked for, and another who joins and leaves: their COLONIST_JOINED,
		// COLONIST_LEFT and WORKFORCE_CHANGED entries are DM-only. Hidden rows' IDs are DM-only literals.
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			dmViewer := testHarness.enter(side.dm, side.campaign.id)
			secret, visible := side.dmSecret, side.playerVisible
			version := func() int64 {
				board, err := testHarness.core.GetColonyBoard(testHarness.context, dmViewer)
				if err != nil {
					t.Fatalf("read %s's board: %v", side.campaign.name, err)
				}
				return board.Version
			}
			roleKey := "FARMER"
			if side.extraRole != "" {
				created, err := testHarness.core.CreateRole(testHarness.context, dmViewer, core.NewRoleInput{Label: side.extraRole, Icon: core.ColonyIconAnchor})
				if err != nil {
					t.Fatalf("add %s: %v", side.extraRole, err)
				}
				if _, err := testHarness.core.UpdateRole(testHarness.context, dmViewer, created.Key, core.RoleInput{
					Label:     side.extraRole + visible,
					Icon:      core.ColonyIconAnchor,
					SortOrder: created.SortOrder,
				}); err != nil {
					t.Fatalf("label %s: %v", side.extraRole, err)
				}
				roleKey = created.Key
			}
			side.campaign.pathIDs["roleKey"] = roleKey
			// Unnamed colonists for pass 4's transfers.
			if _, err := testHarness.core.SetRoleCount(testHarness.context, dmViewer, core.SetRoleCountInput{ExpectedVersion: version(), RoleKey: "UNASSIGNED", Count: 5}); err != nil {
				t.Fatalf("fill Unassigned: %v", err)
			}
			createPlace := func(input core.PlaceInput) core.Place {
				place, err := testHarness.core.CreatePlace(testHarness.context, dmViewer, input)
				if err != nil {
					t.Fatalf("plant place %q: %v", input.Name, err)
				}
				return place
			}
			shown := createPlace(core.PlaceInput{Name: "Harbor Market" + visible, Description: "Fish stalls" + visible, PlayerNotes: "Cheap rope" + visible, DmNotes: secret + " smugglers meet here", Revealed: true})
			hidden := createPlace(core.PlaceInput{Name: secret + " Drowned Chapel", Description: secret, PlayerNotes: secret, DmNotes: secret})
			side.campaign.pathIDs["placeId"] = shown.ID.String()
			side.campaign.keptPlaceID = hidden.ID
			createNpc := func(input core.NpcInput) core.Npc {
				npc, err := testHarness.core.CreateNpc(testHarness.context, dmViewer, input)
				if err != nil {
					t.Fatalf("plant NPC %q: %v", input.Name, err)
				}
				return npc
			}
			level := int64(3)
			revealed := createNpc(core.NpcInput{
				Name: secret + " Selwyn", KnownName: "The Hooded Stranger" + visible, Revealed: true,
				Aliases: []string{"Hood" + visible}, Labels: []string{"Stranger" + visible},
				Race: "Human" + visible, CharacterClass: "Rogue" + visible, Level: &level, Age: "Forty" + visible,
				Appearance: "Grey cloak" + visible, Personality: "Wry" + visible, Backstory: "Came by ship" + visible,
				PlayerNotes: "Owes us" + visible, DmNotes: secret + " is a spy",
				Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationNeutral,
				CurrentPlaceID: &hidden.ID, OriginPlaceID: &shown.ID,
			})
			hiddenValues := core.NpcInput{
				Name: secret + " Mason", KnownName: secret + " Quiet", Aliases: []string{secret}, Labels: []string{secret},
				Race: secret, CharacterClass: secret, Level: &level, Age: secret, Appearance: secret, Personality: secret,
				Backstory: secret, PlayerNotes: secret, DmNotes: secret,
				Status: core.NpcStatusMissing, Health: core.NpcHealthSick, Relation: core.NpcRelationEnemy,
				CurrentPlaceID: &shown.ID, OriginPlaceID: &hidden.ID,
			}
			hiddenColonist := createNpc(hiddenValues)
			hiddenValues.Name = secret + " Corrow"
			leaver := createNpc(hiddenValues)
			side.campaign.pathIDs["npcId"] = revealed.ID.String()
			side.campaign.hiddenNpcID = hiddenColonist.ID
			side.campaign.hiddenIDs = append(side.campaign.hiddenIDs, hidden.ID.String(), hiddenColonist.ID.String(), leaver.ID.String())
			join := func(npc core.Npc, joinRoleKey string) {
				if _, err := testHarness.core.JoinColony(testHarness.context, dmViewer, npc.ID, core.JoinColonyInput{ExpectedVersion: version(), RoleKey: joinRoleKey}); err != nil {
					t.Fatalf("%s joins the colony: %v", npc.DisplayName, err)
				}
			}
			join(revealed, roleKey)
			join(hiddenColonist, "HEALER")
			if _, err := testHarness.core.ChangeColonyRole(testHarness.context, dmViewer, hiddenColonist.ID, core.ChangeColonyRoleInput{ExpectedVersion: version(), RoleKey: roleKey, Public: true}); err != nil {
				t.Fatalf("move the hidden colonist: %v", err)
			}
			join(leaver, roleKey)
			if _, err := testHarness.core.LeaveColony(testHarness.context, dmViewer, leaver.ID, core.LeaveColonyInput{ExpectedVersion: version(), Mode: core.LeaveModeReturnToPool}); err != nil {
				t.Fatalf("the hidden colonist leaves: %v", err)
			}
		},
		dmProof: []string{"/npcs", "/places", "/people", "/timeline/recent"},
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
	values = append(values, campaign.hiddenIDs...)
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
