package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

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
	// requester is the member whose declined request carries a DM-only note: its account name, Discord
	// ID, username (DM-only) and character name. No sweep caller signs in as them.
	requester [4]string
	// failureGold and failureFavor are the revealed quest's DM-only failure cost: numbers no other
	// planted value uses, so a player response holding either has leaked it.
	failureGold, failureFavor int64
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
			requester:      [4]string{"requesterA", "100000000000000007", "rhea." + canaryA, "Mira"},
			failureGold:    4321,
			failureFavor:   8765,
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
			requester:      [4]string{"requesterB", "200000000000000004", "tova." + canaryBdm, "Odile " + canaryB},
			failureGold:    5432,
			failureFavor:   9876,
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
		// COLONIST_LEFT and WORKFORCE_CHANGED entries are DM-only. One more joins while revealed and is
		// hidden after, so a PUBLIC entry links to a hidden NPC. Hidden rows' IDs are DM-only literals.
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
			createPlace := func(input core.NewPlaceInput) core.Place {
				place, err := testHarness.core.CreatePlace(testHarness.context, dmViewer, input)
				if err != nil {
					t.Fatalf("plant place %q: %v", input.Name, err)
				}
				return place
			}
			shown := createPlace(core.NewPlaceInput{Revealed: true, PlaceInput: core.PlaceInput{Name: "Harbor Market" + visible, Description: "Fish stalls" + visible, PlayerNotes: "Cheap rope" + visible, DmNotes: secret + " smugglers meet here"}})
			hidden := createPlace(core.NewPlaceInput{PlaceInput: core.PlaceInput{Name: secret + " Drowned Chapel", Description: secret, PlayerNotes: secret, DmNotes: secret}})
			side.campaign.pathIDs["placeId"] = shown.ID.String()
			side.campaign.keptPlaceID = hidden.ID
			createNpc := func(input core.NewNpcInput) core.Npc {
				npc, err := testHarness.core.CreateNpc(testHarness.context, dmViewer, input)
				if err != nil {
					t.Fatalf("plant NPC %q: %v", input.Name, err)
				}
				return npc
			}
			level := int64(3)
			revealed := createNpc(core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{
				Name: secret + " Selwyn", KnownName: "The Hooded Stranger" + visible,
				Aliases: []string{"Hood" + visible}, Labels: []string{"Stranger" + visible},
				Race: "Human" + visible, CharacterClass: "Rogue" + visible, Level: &level, Age: "Forty" + visible,
				Appearance: "Grey cloak" + visible, Personality: "Wry" + visible, Backstory: "Came by ship" + visible,
				PlayerNotes: "Owes us" + visible, DmNotes: secret + " is a spy",
				Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationNeutral,
				CurrentPlaceID: &hidden.ID, OriginPlaceID: &shown.ID,
			}})
			hiddenValues := core.NewNpcInput{NpcInput: core.NpcInput{
				Name: secret + " Mason", KnownName: secret + " Quiet", Aliases: []string{secret}, Labels: []string{secret},
				Race: secret, CharacterClass: secret, Level: &level, Age: secret, Appearance: secret, Personality: secret,
				Backstory: secret, PlayerNotes: secret, DmNotes: secret,
				Status: core.NpcStatusMissing, Health: core.NpcHealthSick, Relation: core.NpcRelationEnemy,
				CurrentPlaceID: &shown.ID, OriginPlaceID: &hidden.ID,
			}}
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
			hiddenSince := createNpc(core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: secret + " Wren"}})
			join(hiddenSince, roleKey)
			if _, err := testHarness.core.SetNpcRevealed(testHarness.context, dmViewer, hiddenSince.ID, false); err != nil {
				t.Fatalf("hide a colonist after their public entry: %v", err)
			}
			side.campaign.hiddenIDs = append(side.campaign.hiddenIDs, hiddenSince.ID.String())
		},
		dmProof: []string{"/npcs", "/places", "/people", "/timeline/recent"},
	},
	{
		feature: "quests, patrons and requests",
		// Patrons: a revealed one (the {patronId} row) whose dm_notes are DM-only; a hidden one whose every
		// value is; one revealed while a PUBLIC favor entry was written, then hidden; an archived one.
		// Actions on the revealed patron: a revealed one (the {actionId} row), a hidden one used with Use
		// action now, one hidden after a member asked for it, and an archived one; plus a revealed action
		// of the hidden patron. Quests: a revealed one (the {questId} row) whose dm_notes and failure cost
		// are DM-only; a hidden one whose every value is, given by the hidden NPC and patron; one posted to
		// the timeline and then hidden; one for the hidden patron completed (its favor line is DM-only);
		// one failed. Use action now on a hidden patron's hidden action, both renamed and revealed since; Use
		// action now on a revealed action, undone, and the action hidden since. A completed hidden quest, a
		// hidden patron's favor change and a hidden action's use, each undone with its UNDO entry made
		// PUBLIC. Requests by the requester: a pending one (the {requestId} row), a declined one and a
		// granted one whose decision notes are DM-only to everyone but the requester. Hidden rows' IDs are
		// DM-only literals.
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			campaign := side.campaign
			dmViewer := testHarness.enter(side.dm, campaign.id)
			secret, visible := side.dmSecret, side.playerVisible
			requestContext := testHarness.context
			service := testHarness.core
			version := func() int64 {
				board, err := service.GetColonyBoard(requestContext, dmViewer)
				if err != nil {
					t.Fatalf("read %s's board: %v", campaign.name, err)
				}
				return board.Version
			}
			check := func(what string, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("%s in %s: %v", what, campaign.name, err)
				}
			}
			gold := func(amount int64) core.Effects {
				return core.Effects{Resources: map[string]int64{"gold": amount}}
			}
			delta := func(amount int64) *int64 { return &amount }

			requester := testHarness.signUp(side.requester[0], side.requester[1], side.requester[2], "Requester")
			testHarness.join(campaign, side.dm, requester, "approve")
			characterName := side.requester[3]
			_, err := service.SetCharacterName(requestContext, dmViewer, campaign.members[requester.name], core.CharacterNameInput{CharacterName: &characterName})
			check("name the requester's character", err)
			campaign.requester = requester
			requesterViewer := testHarness.enter(requester, campaign.id)

			createPatron := func(input core.NewPatronInput) uuid.UUID {
				t.Helper()
				page, err := service.CreatePatron(requestContext, dmViewer, input)
				check("plant patron "+input.Name, err)
				return page.Patron.ID
			}
			adjustFavor := func(patronID uuid.UUID, amount int64, reason string, public bool) {
				t.Helper()
				_, err := service.AdjustFavor(requestContext, dmViewer, patronID, core.FavorAdjustInput{ExpectedVersion: version(), Delta: delta(amount), Reason: reason, Public: public})
				check("adjust favor", err)
			}
			shownPatron := createPatron(core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{
				Name: "Lady Vex" + visible, Description: "Grain factor" + visible, PlayerNotes: "Likes punctuality" + visible,
				DmNotes: secret + " owes the crown", SortOrder: 1,
			}})
			adjustFavor(shownPatron, 60, secret+" seed favor", false)
			hiddenPatron := createPatron(core.NewPatronInput{PatronInput: core.PatronInput{
				Name: secret + " Shadow Broker", Description: secret, PlayerNotes: secret, DmNotes: secret,
			}})
			adjustFavor(hiddenPatron, 20, secret+" hidden favor", true)
			hiddenSincePatron := createPatron(core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{Name: secret + " Faded Ally"}})
			adjustFavor(hiddenSincePatron, 4, "Helped at the docks", true)
			_, err = service.SetPatronRevealed(requestContext, dmViewer, hiddenSincePatron, false)
			check("hide a patron after their public favor entry", err)
			archivedPatron := createPatron(core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{Name: "Retired Patron" + visible}})
			_, err = service.SetPatronArchived(requestContext, dmViewer, archivedPatron, true)
			check("archive a patron", err)

			createAction := func(patronID uuid.UUID, revealed bool, name string, description string) uuid.UUID {
				t.Helper()
				action, err := service.CreatePatronAction(requestContext, dmViewer, core.NewPatronActionInput{PatronID: patronID, Revealed: revealed, PatronActionInput: core.PatronActionInput{
					Name: name, Description: description, FavorCost: 3, Effects: gold(2), SortOrder: 1,
				}})
				check("plant action "+name, err)
				return action.ID
			}
			shownAction := createAction(shownPatron, true, "Grain Barge"+visible, "A barge of grain"+visible)
			hiddenAction := createAction(shownPatron, false, secret+" Night Run", secret)
			hiddenPatronAction := createAction(hiddenPatron, true, secret+" Whisper Network", secret)
			hiddenSinceAction := createAction(shownPatron, true, secret+" Smuggler's Favor", secret)
			archivedAction := createAction(shownPatron, true, "Old Favor"+visible, "Retired"+visible)
			_, err = service.SetPatronActionArchived(requestContext, dmViewer, archivedAction, true)
			check("archive an action", err)

			ask := func(actionID uuid.UUID, note string) uuid.UUID {
				t.Helper()
				request, err := service.CreateRequest(requestContext, requesterViewer, core.RequestInput{ActionID: actionID, Note: note})
				check("the requester asks", err)
				return request.ID
			}
			pending := ask(shownAction, "For the winter"+visible)
			declined := ask(shownAction, "Again please"+visible)
			_, err = service.DeclineRequest(requestContext, dmViewer, declined, core.DeclineRequestInput{Note: secret + " decline note"})
			check("decline a request", err)
			granted := ask(shownAction, "")
			tweak := gold(1)
			_, err = service.ApproveRequest(requestContext, dmViewer, granted, core.ApproveRequestInput{AppliedEffects: &tweak, Note: secret + " approval note"})
			check("grant a request", err)
			hiddenSinceRequest := ask(hiddenSinceAction, "Quietly"+visible)
			_, err = service.SetPatronActionRevealed(requestContext, dmViewer, hiddenSinceAction, false)
			check("hide an action after a member asked for it", err)
			onBehalfOf := campaign.members[requester.name]
			var usedRequests []string
			for _, actionID := range []uuid.UUID{hiddenAction, hiddenPatronAction} {
				used, err := service.UseActionNow(requestContext, dmViewer, core.UseActionInput{ExpectedVersion: version(), ActionID: actionID, OnBehalfOf: &onBehalfOf, Note: secret + " used quietly"})
				check("use a hidden action now", err)
				for _, entry := range used.Entries {
					if entry.RequestID != nil {
						usedRequests = append(usedRequests, entry.RequestID.String())
					}
				}
			}
			if len(usedRequests) != 2 {
				t.Fatalf("using two hidden actions in %s linked %d requests, want 2", campaign.name, len(usedRequests))
			}
			// Used while its patron and action were hidden, then both renamed and revealed: the request
			// keeps the old names, so it stays away from players.
			veiledPatron := createPatron(core.NewPatronInput{PatronInput: core.PatronInput{Name: secret + " Veiled Guild", DmNotes: secret}})
			adjustFavor(veiledPatron, 5, "Kept quiet", false)
			veiledAction := createAction(veiledPatron, false, secret+" Dead Drop", secret)
			usedWhileHidden, err := service.UseActionNow(requestContext, dmViewer, core.UseActionInput{ExpectedVersion: version(), ActionID: veiledAction, OnBehalfOf: &onBehalfOf, Note: secret + " used before the reveal"})
			check("use a hidden patron's hidden action now", err)
			_, err = service.UpdatePatron(requestContext, dmViewer, veiledPatron, core.PatronInput{Name: "Merchant Guild" + visible})
			check("rename the hidden patron", err)
			_, err = service.UpdatePatronAction(requestContext, dmViewer, veiledAction, core.PatronActionInput{Name: "Courier" + visible, FavorCost: 3, Effects: gold(2), SortOrder: 1})
			check("rename the hidden action", err)
			_, err = service.SetPatronRevealed(requestContext, dmViewer, veiledPatron, true)
			check("reveal the renamed patron", err)
			_, err = service.SetPatronActionRevealed(requestContext, dmViewer, veiledAction, true)
			check("reveal the renamed action", err)
			var veiledRequest uuid.UUID
			for _, entry := range usedWhileHidden.Entries {
				if entry.RequestID != nil {
					veiledRequest = *entry.RequestID
					usedRequests = append(usedRequests, entry.RequestID.String())
				}
			}
			if veiledRequest == uuid.Nil {
				t.Fatalf("using the veiled action in %s linked no request", campaign.name)
			}
			// Use action now on a revealed action, undone (which deletes the request), then the action
			// hidden: the UNDO entry has no request to follow, so it must not name the action.
			rewoundAction := createAction(shownPatron, true, secret+" Rowboat", secret)
			rewound, err := service.UseActionNow(requestContext, dmViewer, core.UseActionInput{ExpectedVersion: version(), ActionID: rewoundAction, OnBehalfOf: &onBehalfOf})
			check("use a revealed action now", err)
			_, err = service.UndoLastChange(requestContext, dmViewer, core.UndoInput{GroupID: rewound.GroupID})
			check("undo the use", err)
			_, err = service.SetPatronActionRevealed(requestContext, dmViewer, rewoundAction, false)
			check("hide the action whose use was undone", err)

			createQuest := func(input core.NewQuestInput) uuid.UUID {
				t.Helper()
				quest, err := service.CreateQuest(requestContext, dmViewer, input)
				check("plant quest "+input.Title, err)
				return quest.ID
			}
			revealedNpc := uuid.MustParse(campaign.pathIDs["npcId"])
			shownQuest := createQuest(core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{
				Title: "Clear the Docks" + visible, Description: "Rats" + visible, Kind: core.QuestKindColony,
				GiverNpcID: &revealedNpc, PatronID: &shownPatron, RewardEffects: gold(5), RewardFavor: 2,
				FailureEffects: gold(-side.failureGold), FailureFavor: side.failureFavor,
				Loot:    []core.LootItem{{Name: "Rat King's Crown" + visible, Quantity: 1, Description: "Tarnished" + visible}},
				DmNotes: secret + " the rats are spies",
			}})
			hiddenQuest := createQuest(core.NewQuestInput{PostToTimeline: true, QuestInput: core.QuestInput{
				Title: secret + " Burn the Archive", Description: secret, Kind: core.QuestKindMainland,
				GiverNpcID: &campaign.hiddenNpcID, PatronID: &hiddenPatron, RewardEffects: gold(3), RewardFavor: 4,
				FailureEffects: gold(-2), FailureFavor: 3,
				Loot:    []core.LootItem{{Name: secret, Quantity: 2, Description: secret}},
				DmNotes: secret,
			}})
			hiddenSinceQuest := createQuest(core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{Title: secret + " Lost Map"}})
			_, err = service.SetQuestRevealed(requestContext, dmViewer, hiddenSinceQuest, core.QuestRevealInput{Revealed: false})
			check("hide a quest after it was posted", err)
			// Changes to the hidden quest, patron and action, each undone and its DM-only UNDO entry then
			// made PUBLIC: the entry follows the same links, so it stays away from players. Planted before
			// the last colony change, whose undo slot the sweep borrows.
			publishUndo := func(what string, change core.ColonyChange, err error) {
				t.Helper()
				check(what, err)
				undone, err := service.UndoLastChange(requestContext, dmViewer, core.UndoInput{GroupID: change.GroupID})
				check("undo "+what, err)
				for _, entry := range undone.Entries {
					_, err := service.UpdateTimelineEntry(requestContext, dmViewer, entry.ID, core.TimelineEntryInput{
						Day: entry.Day, Title: entry.Title, Body: entry.Body, Visibility: core.TimelineVisibilityPublic,
					})
					check("make the UNDO of "+what+" PUBLIC", err)
				}
			}
			completed, err := service.CompleteQuest(requestContext, dmViewer, hiddenQuest)
			publishUndo("complete the hidden quest", completed, err)
			adjusted, err := service.AdjustFavor(requestContext, dmViewer, hiddenPatron, core.FavorAdjustInput{ExpectedVersion: version(), Delta: delta(1), Reason: secret + " undone favor"})
			publishUndo("adjust the hidden patron's favor", adjusted, err)
			usedHidden, err := service.UseActionNow(requestContext, dmViewer, core.UseActionInput{ExpectedVersion: version(), ActionID: hiddenAction, OnBehalfOf: &onBehalfOf, Note: secret + " undone use"})
			publishUndo("use the hidden action now", usedHidden, err)
			brokerQuest := createQuest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{
				Title: "Deliver a Letter" + visible, PatronID: &hiddenPatron, RewardEffects: gold(1), RewardFavor: 5,
			}})
			_, err = service.CompleteQuest(requestContext, dmViewer, brokerQuest)
			check("complete a quest for the hidden patron", err)
			failedQuest := createQuest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{
				Title: "Escort the Barge" + visible, PatronID: &shownPatron, FailureEffects: gold(-1), FailureFavor: 1,
			}})
			_, err = service.FailQuest(requestContext, dmViewer, failedQuest)
			check("fail a quest", err)

			campaign.pathIDs["patronId"] = shownPatron.String()
			campaign.pathIDs["actionId"] = shownAction.String()
			campaign.pathIDs["questId"] = shownQuest.String()
			campaign.pathIDs["requestId"] = pending.String()
			campaign.declinedRequestID = declined
			campaign.hiddenQuestID = hiddenQuest
			campaign.hiddenPatronID = hiddenPatron
			campaign.hiddenRequestID = veiledRequest
			campaign.archivedIDs = []string{archivedPatron.String(), archivedAction.String()}
			campaign.failureCost = []int64{side.failureGold, side.failureFavor}
			campaign.hiddenIDs = append(campaign.hiddenIDs,
				hiddenPatron.String(), hiddenSincePatron.String(),
				hiddenAction.String(), hiddenPatronAction.String(), hiddenSinceAction.String(),
				hiddenQuest.String(), hiddenSinceQuest.String(), hiddenSinceRequest.String(), rewoundAction.String())
			campaign.hiddenIDs = append(campaign.hiddenIDs, usedRequests...)
		},
		dmProof: []string{"/quests", "/patrons", "/patron-actions", "/requests", "/timeline/recent"},
	},
	{
		feature: "maps, pins, hexes and images",
		// A revealed Mainland (the {mapId} row, its art the {imageId} row) carrying a pin of the hidden
		// place, and a hidden map with its own art pinning the revealed {placeId} place, so that place's
		// pin must be null for players. The hidden NPC gets a portrait, and one upload is used by nothing.
		// Every image only hidden things use is a DM-only literal. Hexes have no secrets: B's labels carry
		// canaryB, and B's labelled hex is uncharted in A, so pass 4's coordinates name nothing there.
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			dmViewer := testHarness.enter(side.dm, side.campaign.id)
			secret, visible := side.dmSecret, side.playerVisible
			// Distinct bytes per image and campaign, since identical bytes return the stored image.
			shade := uint8(10)
			if side.campaign == testHarness.campaignB {
				shade = 130
			}
			upload := func(offset uint8) core.UploadedImage {
				uploaded, err := testHarness.core.UploadImage(testHarness.context, dmViewer, testImage(t, shade+offset))
				if err != nil {
					t.Fatalf("upload an image to %s: %v", side.campaign.name, err)
				}
				return uploaded
			}
			mainlandArt, hiddenArt, hiddenPortrait, unused := upload(0), upload(1), upload(2), upload(3)
			createMap := func(input core.NewMapInput) core.WorldMap {
				created, err := testHarness.core.CreateMap(testHarness.context, dmViewer, input)
				if err != nil {
					t.Fatalf("plant map %q: %v", input.Name, err)
				}
				return created
			}
			mainland := createMap(core.NewMapInput{
				MapInput: core.MapInput{Name: "Mainland" + visible}, MapArtInput: core.MapArtInput{ImageID: &mainlandArt.ID},
				Revealed: true, Mainland: true,
			})
			hiddenMap := createMap(core.NewMapInput{MapInput: core.MapInput{Name: secret + " Sunken Isles"}, MapArtInput: core.MapArtInput{ImageID: &hiddenArt.ID}})
			pin := func(placeID uuid.UUID, mapID uuid.UUID) {
				if _, err := testHarness.core.SetPlacePin(testHarness.context, dmViewer, placeID, core.PinInput{MapID: mapID, X: 0.25, Y: 0.75}); err != nil {
					t.Fatalf("pin a place in %s: %v", side.campaign.name, err)
				}
			}
			pin(side.campaign.keptPlaceID, mainland.ID)
			pin(uuid.MustParse(side.campaign.pathIDs["placeId"]), hiddenMap.ID)
			if _, err := testHarness.core.SetNpcPortrait(testHarness.context, dmViewer, side.campaign.hiddenNpcID, core.NpcPortraitInput{ImageID: &hiddenPortrait.ID}); err != nil {
				t.Fatalf("give the hidden NPC a portrait: %v", err)
			}
			side.campaign.pathIDs["mapId"] = mainland.ID.String()
			side.campaign.pathIDs["imageId"] = mainlandArt.ID.String()
			side.campaign.hiddenImageIDs = append(side.campaign.hiddenImageIDs, hiddenArt.ID, hiddenPortrait.ID, unused.ID)
			side.campaign.hiddenIDs = append(side.campaign.hiddenIDs, hiddenMap.ID.String(), hiddenArt.ID.String(), hiddenPortrait.ID.String(), unused.ID.String())

			labelled := core.HexCoordinate{Q: 0, R: 0}
			if side.campaign == testHarness.campaignB {
				labelled = core.HexCoordinate{Q: 5, R: -3}
			}
			if err := testHarness.core.PaintHexes(testHarness.context, dmViewer, core.PaintHexesInput{Cells: []core.PaintedHex{
				{Q: labelled.Q, R: labelled.R, Terrain: core.TerrainForest},
				{Q: labelled.Q + 1, R: labelled.R, Terrain: core.TerrainWater},
			}}); err != nil {
				t.Fatalf("chart hexes in %s: %v", side.campaign.name, err)
			}
			if err := testHarness.core.SetHexLabel(testHarness.context, dmViewer, labelled, core.HexLabelInput{Label: "Camp" + visible}); err != nil {
				t.Fatalf("label a hex in %s: %v", side.campaign.name, err)
			}
			side.campaign.pathIDs["q"] = strconv.FormatInt(labelled.Q, 10)
			side.campaign.pathIDs["r"] = strconv.FormatInt(labelled.R, 10)
		},
		dmProof: []string{"/maps"},
	},
	{
		feature: "sessions",
		// Sessions have no DM-only values (spec §5.11), so B's carry canaryB in every text. The ICS token
		// is the campaign's: every member may read it (spec §6.2 shows players the link), and B's is
		// forbidden wherever canaryB is. The hidden NPC's first session is set, so its link is planted
		// beside a forbidden ID; the revealed NPC's names a session players may see.
		plant: func(t *testing.T, testHarness *harness, side campaignSide) {
			t.Helper()
			dmViewer := testHarness.enter(side.dm, side.campaign.id)
			visible := side.playerVisible
			firstDate := time.Now().UTC().AddDate(0, 0, -28)
			weekly := int64(1)
			series, err := testHarness.core.CreateSessionSeries(testHarness.context, dmViewer, core.SessionSeriesInput{
				Title: "Friday Game" + visible, Description: "Bring dice" + visible,
				Weekdays: []int64{int64(firstDate.Weekday())}, EveryWeeks: &weekly, FirstDate: firstDate.Format(time.DateOnly),
				StartLocal: "20:00", DurationMinutes: 240, Announce: true, Remind: true,
			})
			if err != nil {
				t.Fatalf("plant a series in %s: %v", side.campaign.name, err)
			}
			played, err := testHarness.core.MarkSessionPlayed(testHarness.context, dmViewer, core.SessionDateInput{SeriesID: series.ID, LocalDate: firstDate.Format(time.DateOnly)})
			if err != nil || played.ID == nil {
				t.Fatalf("play the first date in %s: %+v, %v", side.campaign.name, played, err)
			}
			if _, err := testHarness.core.SetSessionRecap(testHarness.context, dmViewer, *played.ID, core.SessionRecapInput{Recap: "We landed" + visible}); err != nil {
				t.Fatalf("write a recap in %s: %v", side.campaign.name, err)
			}
			for _, npcID := range []uuid.UUID{side.campaign.hiddenNpcID, uuid.MustParse(side.campaign.pathIDs["npcId"])} {
				if _, err := testHarness.core.SetNpcFirstSession(testHarness.context, dmViewer, npcID, core.NpcFirstSessionInput{SessionID: played.ID}); err != nil {
					t.Fatalf("set an NPC's first session in %s: %v", side.campaign.name, err)
				}
			}
			link, err := testHarness.core.GetCalendarLink(testHarness.context, dmViewer)
			if err != nil {
				t.Fatalf("read the calendar link of %s: %v", side.campaign.name, err)
			}
			side.campaign.icsToken = strings.TrimSuffix(strings.TrimPrefix(link.Path, "/calendar/"), ".ics")
			side.campaign.pathIDs["seriesId"] = series.ID.String()
			side.campaign.pathIDs["sessionId"] = played.ID.String()
		},
		// The one DM-only thing here is the hidden NPC's first-session link, on the DM's roster.
		dmProof: []string{"/npcs"},
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

// visibleValuesOfB are B's values only B's members may see: canaryB and the ICS token.
func (testHarness *harness) visibleValuesOfB() []string {
	return []string{canaryB, testHarness.campaignB.icsToken}
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

// testImage is a small PNG whose bytes differ with shade.
func testImage(t *testing.T, shade uint8) []byte {
	t.Helper()
	picture := image.NewGray(image.Rect(0, 0, 4, 3))
	for index := range picture.Pix {
		picture.Pix[index] = shade
	}
	picture.SetGray(0, 0, color.Gray{Y: shade + 1})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatalf("encode a test image: %v", err)
	}
	return encoded.Bytes()
}
