package core_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

func (test *colonyTest) npc(input core.NpcInput) core.Npc {
	test.t.Helper()
	npc, err := test.service.CreateNpc(test.context, test.dm, input)
	if err != nil {
		test.t.Fatalf("create NPC %s: %v", input.Name, err)
	}
	return npc
}

func (test *colonyTest) setCount(roleKey string, count int64) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.SetRoleCount(test.context, test.dm, core.SetRoleCountInput{ExpectedVersion: test.board().Version, RoleKey: roleKey, Count: count}))
}

func (test *colonyTest) join(npcID uuid.UUID, roleKey string, mode core.JoinMode) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.JoinColony(test.context, test.dm, npcID, core.JoinColonyInput{ExpectedVersion: test.board().Version, RoleKey: roleKey, Mode: mode}))
}

func (test *colonyTest) leave(npcID uuid.UUID, mode core.LeaveMode) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.LeaveColony(test.context, test.dm, npcID, core.LeaveColonyInput{ExpectedVersion: test.board().Version, Mode: mode}))
}

func (test *colonyTest) undo(change core.ColonyChange) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: change.GroupID}))
}

// headcount is one role's line on the board as viewer sees it.
func (test *colonyTest) headcount(viewer core.Viewer, roleKey string) core.RoleHeadcount {
	test.t.Helper()
	board, err := test.service.GetColonyBoard(test.context, viewer)
	if err != nil {
		test.t.Fatal(err)
	}
	for _, role := range board.Roles {
		if role.Key == roleKey {
			return role
		}
	}
	test.t.Fatalf("no role %s on the board", roleKey)
	return core.RoleHeadcount{}
}

// The board's Colonists tile always equals the sum of the role column (spec §10 M2 acceptance).
func (test *colonyTest) requireColonistsMatchRoles(viewer core.Viewer) int64 {
	test.t.Helper()
	board, err := test.service.GetColonyBoard(test.context, viewer)
	if err != nil {
		test.t.Fatal(err)
	}
	var sum int64
	for _, role := range board.Roles {
		if role.Total != role.UnnamedCount+role.NamedCount {
			test.t.Fatalf("%s: total %d is not unnamed %d + named %d", role.Key, role.Total, role.UnnamedCount, role.NamedCount)
		}
		sum += role.Total
	}
	if board.Colonists != sum {
		test.t.Fatalf("Colonists %d, but the roles add up to %d", board.Colonists, sum)
	}
	return board.Colonists
}

func (test *colonyTest) colonyRole(npcID uuid.UUID) *core.ColonyRoleReference {
	test.t.Helper()
	npc, err := test.service.GetNpc(test.context, test.dm, npcID)
	if err != nil {
		test.t.Fatal(err)
	}
	return npc.ColonyRole
}

// The M2 acceptance numbers (spec §5.5, §10).
func TestWorkforceAcceptance(t *testing.T) {
	test := newColonyTest(t)
	test.setCount("UNASSIGNED", 5)
	test.setCount("FARMER", 2)
	if colonists := test.requireColonistsMatchRoles(test.dm); colonists != 7 {
		t.Fatalf("colonists %d, want 7", colonists)
	}

	moved := test.must(test.service.TransferColonists(test.context, test.dm, core.TransferInput{
		ExpectedVersion: test.board().Version, FromRoleKey: "UNASSIGNED", ToRoleKey: "FARMER", Count: 3,
	}))
	if farmer := test.headcount(test.dm, "FARMER"); farmer.UnnamedCount != 5 || test.board().Unassigned != 2 {
		t.Fatalf("after moving 3: farmer %+v, unassigned %d", farmer, test.board().Unassigned)
	}
	if entry := moved.Entries[0]; entry.Kind != core.TimelineKindWorkforceChanged || entry.Dm.Visibility != core.TimelineVisibilityDm || entry.Title != "3 colonists: Unassigned → Farmer" {
		t.Fatalf("transfer entry = %+v", entry)
	}
	_, err := test.service.TransferColonists(test.context, test.dm, core.TransferInput{
		ExpectedVersion: test.board().Version, FromRoleKey: "UNASSIGNED", ToRoleKey: "FARMER", Count: 3,
	})
	operationError, typed := core.AsError(err)
	if !typed || operationError.Kind != core.KindInvalid || operationError.Message != "Unassigned has only 2 unnamed colonists, so 3 can't move." {
		t.Fatalf("moving more than exist: %v", err)
	}

	brannoc := test.npc(core.NpcInput{Name: "Brannoc", Revealed: true})
	thorne := test.npc(core.NpcInput{Name: "Thorne", Revealed: true})
	before := test.requireColonistsMatchRoles(test.dm)
	test.join(brannoc.ID, "FARMER", core.JoinModeNewColonist)
	if after := test.requireColonistsMatchRoles(test.dm); after != before+1 {
		t.Fatalf("a new colonist: %d → %d", before, after)
	}
	test.join(thorne.ID, "FARMER", core.JoinModePromoteUnnamed)
	if after := test.requireColonistsMatchRoles(test.dm); after != before+1 {
		t.Fatalf("promoting a Farmer changed Colonists to %d", after)
	}
	if farmer := test.headcount(test.dm, "FARMER"); farmer.UnnamedCount != 4 || farmer.NamedCount != 2 || farmer.Total != 6 {
		t.Fatalf("farmer = %+v", farmer)
	}

	// Upkeep counts the named colonists: 8 colonists eat 8 food a day.
	test.setTo(map[string]int64{"food": 100})
	test.advance(1)
	if food := test.amount("food"); food != 92 {
		t.Fatalf("food after a day = %d, want 92", food)
	}

	err = test.service.DeleteNpc(test.context, test.dm, brannoc.ID)
	operationError, typed = core.AsError(err)
	if !typed || operationError.Kind != core.KindConflict || operationError.Message != "Brannoc is a colonist. Remove them from the colony first." {
		t.Fatalf("deleting a colonist: %v", err)
	}
	test.leave(brannoc.ID, core.LeaveModeRemove)
	if after := test.requireColonistsMatchRoles(test.dm); after != before {
		t.Fatalf("after leaving: %d, want %d", after, before)
	}
	if err := test.service.DeleteNpc(test.context, test.dm, brannoc.ID); err != nil {
		t.Fatalf("delete after leaving: %v", err)
	}
	test.leave(thorne.ID, core.LeaveModeReturnToPool)
	if farmer := test.headcount(test.dm, "FARMER"); farmer.UnnamedCount != 5 || farmer.NamedCount != 0 {
		t.Fatalf("after returning to the pool: %+v", farmer)
	}
	if after := test.requireColonistsMatchRoles(test.dm); after != before {
		t.Fatalf("returning to the pool changed Colonists to %d", after)
	}
}

func TestWorkforceChangesUndo(t *testing.T) {
	test := newColonyTest(t)
	test.setCount("FARMER", 4)
	brannoc := test.npc(core.NpcInput{Name: "Brannoc", Revealed: true})

	joined := test.join(brannoc.ID, "FARMER", core.JoinModePromoteUnnamed)
	test.undo(joined)
	if role := test.colonyRole(brannoc.ID); role != nil {
		t.Fatalf("undoing the join left them in %+v", role)
	}
	if farmer := test.headcount(test.dm, "FARMER"); farmer.UnnamedCount != 4 || farmer.NamedCount != 0 {
		t.Fatalf("undoing the promotion: %+v", farmer)
	}

	test.join(brannoc.ID, "FARMER", core.JoinModeNewColonist)
	changed := test.must(test.service.ChangeColonyRole(test.context, test.dm, brannoc.ID, core.ChangeColonyRoleInput{ExpectedVersion: test.board().Version, RoleKey: "HEALER"}))
	test.undo(changed)
	if role := test.colonyRole(brannoc.ID); role == nil || role.Key != "FARMER" {
		t.Fatalf("undoing the role change: %+v", role)
	}

	left := test.leave(brannoc.ID, core.LeaveModeReturnToPool)
	test.undo(left)
	if role := test.colonyRole(brannoc.ID); role == nil || role.Key != "FARMER" {
		t.Fatalf("undoing the leave: %+v", role)
	}
	if farmer := test.headcount(test.dm, "FARMER"); farmer.UnnamedCount != 4 || farmer.NamedCount != 1 {
		t.Fatalf("undoing the return to the pool: %+v", farmer)
	}
	entries := test.timeline(test.dm, 0, 0)
	if !slices.ContainsFunc(entries, func(entry core.TimelineEntry) bool {
		return entry.Kind == core.TimelineKindColonistLeft && entry.Dm.UndoneAt != nil
	}) {
		t.Fatal("the undone leave isn't marked undone")
	}

	transferred := test.must(test.service.TransferColonists(test.context, test.dm, core.TransferInput{
		ExpectedVersion: test.board().Version, FromRoleKey: "FARMER", ToRoleKey: "UNASSIGNED", Count: 2, Public: true,
	}))
	if transferred.Entries[0].Dm.Visibility != core.TimelineVisibilityPublic {
		t.Fatalf("an opted-in public transfer is %s", transferred.Entries[0].Dm.Visibility)
	}
	test.undo(transferred)
	if farmer := test.headcount(test.dm, "FARMER"); farmer.UnnamedCount != 4 || test.board().Unassigned != 0 {
		t.Fatalf("undoing the transfer: %+v, unassigned %d", farmer, test.board().Unassigned)
	}

	// Leaving, then deleting the NPC, leaves nothing for undo to put back.
	left = test.leave(brannoc.ID, core.LeaveModeRemove)
	if err := test.service.DeleteNpc(test.context, test.dm, brannoc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: left.GroupID}); !isKind(err, core.KindConflict) {
		t.Fatalf("undo after the NPC was deleted: %v", err)
	}
}

func TestWorkforceChangesNeedTheCurrentVersion(t *testing.T) {
	test := newColonyTest(t)
	test.setCount("FARMER", 4)
	brannoc := test.npc(core.NpcInput{Name: "Brannoc"})
	stale := test.board().Version
	test.setCount("FARMER", 5)
	for name, attempt := range map[string]func() error{
		"transfer": func() error {
			_, err := test.service.TransferColonists(test.context, test.dm, core.TransferInput{ExpectedVersion: stale, FromRoleKey: "FARMER", ToRoleKey: "HEALER", Count: 1})
			return err
		},
		"set count": func() error {
			_, err := test.service.SetRoleCount(test.context, test.dm, core.SetRoleCountInput{ExpectedVersion: stale, RoleKey: "FARMER", Count: 1})
			return err
		},
		"join": func() error {
			_, err := test.service.JoinColony(test.context, test.dm, brannoc.ID, core.JoinColonyInput{ExpectedVersion: stale, RoleKey: "FARMER"})
			return err
		},
	} {
		if err := attempt(); !isKind(err, core.KindConflict) {
			t.Errorf("%s with a stale version: %v", name, err)
		}
	}
	if test.headcount(test.dm, "FARMER").UnnamedCount != 5 || test.colonyRole(brannoc.ID) != nil {
		t.Fatal("a refused change wrote something")
	}
	if _, err := test.service.TransferColonists(test.context, test.player, core.TransferInput{ExpectedVersion: test.board().Version, FromRoleKey: "FARMER", ToRoleKey: "HEALER", Count: 1}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player transferring: %v", err)
	}
}

// IDs and keys in request bodies are loaded with the viewer's campaign (CLAUDE.md Invariant 11).
func TestPeopleRefuseAnotherCampaignsIDsAndKeys(t *testing.T) {
	test := newColonyTest(t)
	otherCaller := test.caller("600000000000000007", "other")
	_, otherDm := test.campaign(otherCaller, "Other", "other")
	otherPlace, err := test.service.CreatePlace(test.context, otherDm, core.PlaceInput{Name: "Their Docks", Revealed: true})
	if err != nil {
		t.Fatal(err)
	}
	otherNpc, err := test.service.CreateNpc(test.context, otherDm, core.NpcInput{Name: "Their Captain", Revealed: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.CreateRole(test.context, otherDm, core.NewRoleInput{Label: "Sailor", Icon: core.ColonyIconAnchor}); err != nil {
		t.Fatal(err)
	}

	_, err = test.service.CreateNpc(test.context, test.dm, core.NpcInput{Name: "Brannoc", CurrentPlaceID: &otherPlace.ID})
	if operationError, typed := core.AsError(err); !typed || operationError.Kind != core.KindInvalid || len(operationError.Fields) != 1 || operationError.Fields[0].Field != "currentPlaceId" {
		t.Fatalf("another campaign's place: %v", err)
	}
	if _, err := test.service.JoinColony(test.context, test.dm, otherNpc.ID, core.JoinColonyInput{ExpectedVersion: test.board().Version, RoleKey: "FARMER"}); !isKind(err, core.KindNotFound) {
		t.Fatalf("another campaign's NPC: %v", err)
	}
	if _, err := test.service.GetNpc(test.context, test.dm, otherNpc.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("reading another campaign's NPC: %v", err)
	}
	if _, err := test.service.GetPlace(test.context, test.dm, otherPlace.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("reading another campaign's place: %v", err)
	}
	brannoc := test.npc(core.NpcInput{Name: "Brannoc"})
	if _, err := test.service.JoinColony(test.context, test.dm, brannoc.ID, core.JoinColonyInput{ExpectedVersion: test.board().Version, RoleKey: "SAILOR"}); !isKind(err, core.KindInvalid) {
		t.Fatalf("another campaign's role: %v", err)
	}
	if _, err := test.service.TransferColonists(test.context, test.dm, core.TransferInput{ExpectedVersion: test.board().Version, FromRoleKey: "UNASSIGNED", ToRoleKey: "SAILOR", Count: 1}); !isKind(err, core.KindInvalid) {
		t.Fatalf("transfer into another campaign's role: %v", err)
	}
	if _, err := test.service.UpdateRole(test.context, test.dm, "SAILOR", core.RoleInput{Label: "Sailor", Icon: core.ColonyIconAnchor}); !isKind(err, core.KindNotFound) {
		t.Fatalf("editing another campaign's role: %v", err)
	}
	if err := test.service.DeleteNpc(test.context, test.dm, otherNpc.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("deleting another campaign's NPC: %v", err)
	}
	if err := test.service.DeletePlace(test.context, test.dm, otherPlace.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("deleting another campaign's place: %v", err)
	}
	list, err := test.service.ListNpcs(test.context, test.dm, core.NpcFilter{PlaceID: &otherPlace.ID})
	if err != nil || len(list.Npcs) != 0 {
		t.Fatalf("filtering by another campaign's place: %+v, %v", list, err)
	}
}

// Hidden NPCs and places never reach players, and players never get a true name (spec §5.9, Open
// Question 8).
func TestNpcSecrecy(t *testing.T) {
	test := newColonyTest(t)
	chapel, err := test.service.CreatePlace(test.context, test.dm, core.PlaceInput{Name: "Drowned Chapel", DmNotes: "cult", Revealed: false})
	if err != nil {
		t.Fatal(err)
	}
	docks, err := test.service.CreatePlace(test.context, test.dm, core.PlaceInput{Name: "Docks", Revealed: true})
	if err != nil {
		t.Fatal(err)
	}
	stranger := test.npc(core.NpcInput{Name: "Selwyn Hart", KnownName: "The Hooded Stranger", Revealed: true, Aliases: []string{"the grey one"},
		DmNotes: "spy", CurrentPlaceID: &chapel.ID, OriginPlaceID: &docks.ID})
	corrow := test.npc(core.NpcInput{Name: "Mother Corrow", Revealed: false, CurrentPlaceID: &docks.ID})
	if !strings.Contains(stranger.Dm.Name, "Selwyn") || stranger.DisplayName != "The Hooded Stranger" || stranger.Dm.Revealed != true {
		t.Fatalf("the DM's view = %+v", stranger)
	}
	if corrow.Dm.Revealed {
		t.Fatal("a new NPC starts revealed unless the DM says so")
	}

	asPlayer := []core.Viewer{test.player, test.dm.AsPlayer()}
	for _, viewer := range asPlayer {
		roster, err := test.service.ListNpcs(test.context, viewer, core.NpcFilter{})
		if err != nil || len(roster.Npcs) != 1 || roster.Npcs[0].ID != stranger.ID {
			t.Fatalf("the player's roster = %+v, %v", roster, err)
		}
		seen := roster.Npcs[0]
		if seen.DisplayName != "The Hooded Stranger" || seen.CurrentPlace != nil || seen.OriginPlace == nil || seen.OriginPlace.ID != docks.ID {
			t.Fatalf("the player's stranger = %+v", seen)
		}
		for search, want := range map[string]int{"selwyn": 0, "hooded": 1, "grey one": 1, "corrow": 0, "%": 0} {
			found, err := test.service.ListNpcs(test.context, viewer, core.NpcFilter{Search: search})
			if err != nil || len(found.Npcs) != want {
				t.Errorf("player search %q found %d, want %d (%v)", search, len(found.Npcs), want, err)
			}
		}
		if _, err := test.service.GetNpc(test.context, viewer, corrow.ID); !isKind(err, core.KindNotFound) {
			t.Fatalf("a player reading a hidden NPC: %v", err)
		}
		if _, err := test.service.GetPlace(test.context, viewer, chapel.ID); !isKind(err, core.KindNotFound) {
			t.Fatalf("a player reading a hidden place: %v", err)
		}
		places, err := test.service.ListPlaces(test.context, viewer, "")
		if err != nil || len(places.Places) != 1 || places.Places[0].ID != docks.ID {
			t.Fatalf("the player's places = %+v, %v", places, err)
		}
		here, err := test.service.GetPlace(test.context, viewer, docks.ID)
		if err != nil || len(here.NpcsHere) != 0 {
			t.Fatalf("a hidden NPC is listed at the docks: %+v, %v", here, err)
		}
		hidden, err := test.service.ListNpcs(test.context, viewer, core.NpcFilter{Revealed: new(false)})
		if err != nil || len(hidden.Npcs) != 1 || hidden.Npcs[0].ID != stranger.ID {
			t.Fatalf("a player's Hidden filter is ignored, so it still shows only revealed NPCs: %+v, %v", hidden, err)
		}
	}
	for search, want := range map[string]int{"selwyn": 1, "hooded": 1, "corrow": 1} {
		found, err := test.service.ListNpcs(test.context, test.dm, core.NpcFilter{Search: search})
		if err != nil || len(found.Npcs) != want {
			t.Errorf("DM search %q found %d, want %d (%v)", search, len(found.Npcs), want, err)
		}
	}
	hidden, err := test.service.ListNpcs(test.context, test.dm, core.NpcFilter{Revealed: new(false)})
	if err != nil || len(hidden.Npcs) != 1 || hidden.Npcs[0].ID != corrow.ID {
		t.Fatalf("the DM's Hidden filter = %+v, %v", hidden, err)
	}

	// Beny Bot: the player projection whoever asks, and a true name finds nothing.
	for _, viewer := range []core.Viewer{test.dm, test.player} {
		suggestions, err := test.service.SuggestRevealedNpcs(test.context, viewer, "the h", 25)
		if err != nil || len(suggestions) != 1 || suggestions[0].Dm != nil || suggestions[0].DisplayName != "The Hooded Stranger" {
			t.Fatalf("suggestions = %+v, %v", suggestions, err)
		}
		for _, prefix := range []string{"sel", "mother", "%"} {
			if none, err := test.service.SuggestRevealedNpcs(test.context, viewer, prefix, 25); err != nil || len(none) != 0 {
				t.Fatalf("suggestions for %q = %+v, %v", prefix, none, err)
			}
		}
		found, err := test.service.FindRevealedNpc(test.context, viewer, "  the hooded STRANGER ")
		if err != nil || found.ID != stranger.ID || found.Dm != nil {
			t.Fatalf("find by display name = %+v, %v", found, err)
		}
		for _, name := range []string{"Selwyn Hart", "Mother Corrow", "The Hooded"} {
			if _, err := test.service.FindRevealedNpc(test.context, viewer, name); !isKind(err, core.KindNotFound) {
				t.Fatalf("find %q: %v", name, err)
			}
		}
	}
}

// Named colonists: the entries name them by display name, are public only when they are revealed, and
// a hidden colonist is counted among the unnamed for players (spec §5.5, §5.8).
func TestNamedColonistsAndTheirEntries(t *testing.T) {
	test := newColonyTest(t)
	test.setCount("BUILDER", 3)
	mason := test.npc(core.NpcInput{Name: "Edric Vael", KnownName: "The Quiet Mason", Revealed: false})
	stranger := test.npc(core.NpcInput{Name: "Selwyn Hart", KnownName: "The Hooded Stranger", Revealed: true})

	hiddenJoin := test.join(mason.ID, "BUILDER", core.JoinModePromoteUnnamed)
	publicJoin := test.join(stranger.ID, "BUILDER", core.JoinModeNewColonist)
	if entry := hiddenJoin.Entries[0]; entry.Kind != core.TimelineKindColonistJoined || entry.Dm.Visibility != core.TimelineVisibilityDm || strings.Contains(entry.Title, "Edric") {
		t.Fatalf("the hidden colonist's entry = %+v", entry)
	}
	if entry := publicJoin.Entries[0]; entry.Dm.Visibility != core.TimelineVisibilityPublic || entry.Title != "The Hooded Stranger joined the colony as Builder" {
		t.Fatalf("the revealed colonist's entry = %+v", entry)
	}

	dmBuilder := test.headcount(test.dm, "BUILDER")
	playerBuilder := test.headcount(test.player, "BUILDER")
	if dmBuilder.UnnamedCount != 2 || dmBuilder.NamedCount != 2 || dmBuilder.Total != 4 {
		t.Fatalf("the DM's builders = %+v", dmBuilder)
	}
	if playerBuilder.UnnamedCount != 3 || playerBuilder.NamedCount != 1 || playerBuilder.Total != 4 {
		t.Fatalf("the player's builders = %+v", playerBuilder)
	}
	test.requireColonistsMatchRoles(test.player)

	workforce, err := test.service.GetWorkforce(test.context, test.player)
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(workforce.Roles, func(role core.WorkforceRole) bool { return role.Key == "BUILDER" })
	if index < 0 || len(workforce.Roles[index].NamedColonists) != 1 || workforce.Roles[index].NamedColonists[0].DisplayName != "The Hooded Stranger" {
		t.Fatalf("the player's named builders = %+v", workforce.Roles)
	}
	for _, entry := range test.timeline(test.player, 0, 0) {
		if strings.Contains(entry.Title, "Mason") || strings.Contains(entry.Title, "Selwyn") || strings.Contains(entry.Title, "Edric") {
			t.Fatalf("a player sees %q", entry.Title)
		}
	}
	dmWorkforce, err := test.service.GetWorkforce(test.context, test.dm)
	if err != nil || dmWorkforce.Colonists != 4 {
		t.Fatalf("the DM's workforce = %+v, %v", dmWorkforce, err)
	}

	// Change role stays DM-only for a hidden NPC even when the DM opts in.
	changed := test.must(test.service.ChangeColonyRole(test.context, test.dm, mason.ID, core.ChangeColonyRoleInput{ExpectedVersion: test.board().Version, RoleKey: "MINER", Public: true}))
	if changed.Entries[0].Dm.Visibility != core.TimelineVisibilityDm || changed.Entries[0].Title != "The Quiet Mason: Builder → Miner" {
		t.Fatalf("the hidden colonist's role change = %+v", changed.Entries[0])
	}
	// Marking an NPC DEAD changes no counts.
	before := test.board().Colonists
	if _, err := test.service.UpdateNpc(test.context, test.dm, stranger.ID, core.NpcInput{Name: "Selwyn Hart", KnownName: "The Hooded Stranger", Revealed: true, Status: core.NpcStatusDead, Health: core.NpcHealthDead}); err != nil {
		t.Fatal(err)
	}
	if after := test.board().Colonists; after != before {
		t.Fatalf("marking an NPC dead changed Colonists %d → %d", before, after)
	}
	if role := test.colonyRole(stranger.ID); role == nil || role.Key != "BUILDER" {
		t.Fatalf("editing a colonist's record changed their role: %+v", role)
	}
}

func TestRoleSettings(t *testing.T) {
	test := newColonyTest(t)
	sailor, err := test.service.CreateRole(test.context, test.dm, core.NewRoleInput{Label: " Ship's Crew ", Icon: core.ColonyIconAnchor})
	if err != nil || sailor.Key != "SHIP_S_CREW" || sailor.Label != "Ship's Crew" {
		t.Fatalf("new role = %+v, %v", sailor, err)
	}
	if _, err := test.service.CreateRole(test.context, test.dm, core.NewRoleInput{Label: "farmer", Icon: core.ColonyIconWheat}); !isKind(err, core.KindInvalid) {
		t.Fatalf("a second Farmer: %v", err)
	}
	brannoc := test.npc(core.NpcInput{Name: "Brannoc"})
	test.join(brannoc.ID, sailor.Key, core.JoinModeNewColonist)
	if _, err := test.service.UpdateRole(test.context, test.dm, sailor.Key, core.RoleInput{Label: "Sailor", Icon: core.ColonyIconShip, SortOrder: 5, Archived: true}); !isKind(err, core.KindInvalid) {
		t.Fatalf("archiving a role with a named colonist: %v", err)
	}
	test.leave(brannoc.ID, core.LeaveModeRemove)
	archived, err := test.service.UpdateRole(test.context, test.dm, sailor.Key, core.RoleInput{Label: "Sailor", Icon: core.ColonyIconShip, SortOrder: 5, Archived: true})
	if err != nil || !archived.Archived || archived.Label != "Sailor" || archived.Key != sailor.Key {
		t.Fatalf("archive an empty role = %+v, %v", archived, err)
	}
	if _, err := test.service.JoinColony(test.context, test.dm, brannoc.ID, core.JoinColonyInput{ExpectedVersion: test.board().Version, RoleKey: sailor.Key}); !isKind(err, core.KindInvalid) {
		t.Fatalf("joining an archived role: %v", err)
	}
	if _, err := test.service.UpdateRole(test.context, test.dm, "UNASSIGNED", core.RoleInput{Label: "Idle", Icon: core.ColonyIconUsers}); !isKind(err, core.KindInvalid) {
		t.Fatalf("renaming Unassigned: %v", err)
	}
	if _, err := test.service.UpdateRole(test.context, test.dm, "UNASSIGNED", core.RoleInput{Label: "Unassigned", Icon: core.ColonyIconUsers, Archived: true}); !isKind(err, core.KindInvalid) {
		t.Fatalf("archiving Unassigned: %v", err)
	}
	if moved, err := test.service.UpdateRole(test.context, test.dm, "UNASSIGNED", core.RoleInput{Label: "Unassigned", Icon: core.ColonyIconTent, SortOrder: 1}); err != nil || moved.Icon != core.ColonyIconTent {
		t.Fatalf("re-iconing Unassigned = %+v, %v", moved, err)
	}
	test.setCount("FARMER", 1)
	if _, err := test.service.UpdateRole(test.context, test.dm, "FARMER", core.RoleInput{Label: "Farmer", Icon: core.ColonyIconWheat, Archived: true}); !isKind(err, core.KindInvalid) {
		t.Fatalf("archiving a role with unnamed colonists: %v", err)
	}
	roles, err := test.service.ListRoles(test.context, test.dm)
	if err != nil || !slices.ContainsFunc(roles.Roles, func(role core.RoleSettings) bool { return role.Key == sailor.Key && role.Archived }) {
		t.Fatalf("roles = %+v, %v", roles, err)
	}
	if _, err := test.service.ListRoles(test.context, test.player); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player listing role settings: %v", err)
	}
}

func TestNpcAndPlaceRecords(t *testing.T) {
	test := newColonyTest(t)
	_, err := test.service.CreateNpc(test.context, test.dm, core.NpcInput{
		Name: "  ", Level: new(int64(101)), Status: "ASLEEP", Aliases: []string{"a\tb"},
	})
	operationError, typed := core.AsError(err)
	if !typed || operationError.Kind != core.KindInvalid {
		t.Fatalf("a bad NPC: %v", err)
	}
	fields := []string{}
	for _, field := range operationError.Fields {
		fields = append(fields, field.Field)
	}
	slices.Sort(fields)
	if !slices.Equal(fields, []string{"aliases", "level", "name", "status"}) {
		t.Fatalf("field errors = %v", fields)
	}

	docks, err := test.service.CreatePlace(test.context, test.dm, core.PlaceInput{Name: "Docks", Revealed: true})
	if err != nil {
		t.Fatal(err)
	}
	npc := test.npc(core.NpcInput{Name: "Pip", Aliases: []string{"Pip", " pip ", "Little Pip", ""}, CurrentPlaceID: &docks.ID})
	if !slices.Equal(npc.Aliases, []string{"Pip", "Little Pip"}) || npc.Status != core.NpcStatusUnknown || npc.Health != core.NpcHealthHealthy || npc.CurrentPlace == nil {
		t.Fatalf("new NPC = %+v", npc)
	}
	revealed, err := test.service.SetNpcRevealed(test.context, test.dm, npc.ID, true)
	if err != nil || !revealed.Dm.Revealed {
		t.Fatalf("reveal = %+v, %v", revealed, err)
	}
	if err := test.service.DeletePlace(test.context, test.dm, docks.ID); err != nil {
		t.Fatal(err)
	}
	if after, err := test.service.GetNpc(test.context, test.dm, npc.ID); err != nil || after.CurrentPlace != nil {
		t.Fatalf("the NPC of a deleted place = %+v, %v", after, err)
	}
	hidden, err := test.service.SetPlaceRevealed(test.context, test.dm, docks.ID, false)
	if !isKind(err, core.KindNotFound) {
		t.Fatalf("revealing a deleted place = %+v, %v", hidden, err)
	}
	if _, err := test.service.CreatePlace(test.context, test.player, core.PlaceInput{Name: "Mine"}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player creating a place: %v", err)
	}
	if _, err := test.service.CreateNpc(test.context, test.player, core.NpcInput{Name: "Mine"}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player creating an NPC: %v", err)
	}
}

// Timeline entries about people never tell players more than /people does: set-count titles use role
// totals, an entry follows its NPC's visibility, and an entry naming a true name can't go public.
func TestPeopleEntriesKeepNpcSecrets(t *testing.T) {
	test := newColonyTest(t)
	test.setCount("BUILDER", 3)
	mason := test.npc(core.NpcInput{Name: "Edric Vael", KnownName: "The Quiet Mason", Revealed: false})
	test.join(mason.ID, "BUILDER", core.JoinModeNewColonist)

	before := test.headcount(test.player, "BUILDER").Total
	setCount := test.must(test.service.SetRoleCount(test.context, test.dm, core.SetRoleCountInput{ExpectedVersion: test.board().Version, RoleKey: "BUILDER", Count: 5, Public: true}))
	after := test.headcount(test.player, "BUILDER").Total
	if want := "Builder: " + strconv.FormatInt(before, 10) + " → " + strconv.FormatInt(after, 10) + " colonists"; setCount.Entries[0].Title != want {
		t.Fatalf("the public set-count entry reads %q, but players count %q", setCount.Entries[0].Title, want)
	}

	corrow := test.npc(core.NpcInput{Name: "Mother Corrow", Revealed: false})
	joined := test.join(corrow.ID, "SOLDIER", core.JoinModeNewColonist).Entries[0]
	makePublic := func(title string) error {
		_, err := test.service.UpdateTimelineEntry(test.context, test.dm, joined.ID, core.TimelineEntryInput{Day: joined.Day, Title: title, Visibility: core.TimelineVisibilityPublic})
		return err
	}
	if err := makePublic(joined.Title); !hasField(err, "visibility") {
		t.Fatalf("an entry about a hidden NPC went public: %v", err)
	}
	if _, err := test.service.UpdateNpc(test.context, test.dm, corrow.ID, core.NpcInput{Name: "Mother Corrow", KnownName: "The Drowned Mother", Revealed: true}); err != nil {
		t.Fatal(err)
	}
	if err := makePublic(joined.Title); !hasField(err, "visibility") {
		t.Fatalf("an entry naming %q went public: %v", joined.Title, err)
	}
	if err := makePublic("The Drowned Mother joined the colony as Soldier"); err != nil {
		t.Fatalf("an entry by the known name: %v", err)
	}

	stranger := test.npc(core.NpcInput{Name: "Selwyn Hart", KnownName: "The Hooded Stranger", Revealed: true})
	test.join(stranger.ID, "FARMER", core.JoinModeNewColonist)
	playerSees := func(fragment string) bool {
		return slices.ContainsFunc(test.timeline(test.player, 0, 0), func(entry core.TimelineEntry) bool { return strings.Contains(entry.Title, fragment) })
	}
	if !playerSees("The Hooded Stranger joined") {
		t.Fatal("a revealed colonist's entry is missing")
	}
	if _, err := test.service.SetNpcRevealed(test.context, test.dm, stranger.ID, false); err != nil {
		t.Fatal(err)
	}
	if playerSees("The Hooded Stranger") {
		t.Fatal("an entry about an NPC the DM hid since still reaches players")
	}
	if _, err := test.service.SetNpcRevealed(test.context, test.dm, stranger.ID, true); err != nil {
		t.Fatal(err)
	}
	if !playerSees("The Hooded Stranger joined") {
		t.Fatal("revealing the NPC again didn't bring back their entry")
	}
}

// Creates at the limit can't race past it: the count and the insert happen under the colony lock.
func TestNpcAndPlaceLimitsHoldUnderConcurrentCreates(t *testing.T) {
	test := newColonyTest(t)
	for table, limit := range map[string]int{"npc": core.NpcLimit, "place": core.PlaceLimit} {
		plant := `INSERT INTO ` + table + ` (campaign_id, name) SELECT $1, 'Planted ' || n FROM generate_series(1, $2::int) AS n`
		if _, err := test.database.Pool.Exec(test.context, plant, test.campaignID, limit-2); err != nil {
			t.Fatal(err)
		}
	}
	creates := map[string]func() error{
		"NPC": func() error {
			_, err := test.service.CreateNpc(test.context, test.dm, core.NpcInput{Name: "Latecomer"})
			return err
		},
		"place": func() error {
			_, err := test.service.CreatePlace(test.context, test.dm, core.PlaceInput{Name: "Latecomer"})
			return err
		},
	}
	for kind, create := range creates {
		results := make(chan error, 6)
		for range 6 {
			go func() { results <- create() }()
		}
		created := 0
		for range 6 {
			err := <-results
			if err == nil {
				created++
			} else if !isKind(err, core.KindConflict) {
				t.Fatalf("a %s create failed with %v", kind, err)
			}
		}
		if created != 2 {
			t.Fatalf("%d %s creates passed the limit check, want 2", created, kind)
		}
	}
}

// Entry text keeps the display name it was written with, so renames, undo and deletion must not let
// it reach players under a name they don't know (Invariant 15).
func TestNpcEntriesNeverNameNpcsByUnknownNames(t *testing.T) {
	test := newColonyTest(t)
	test.setCount("FARMER", 2)
	edit := func(entry core.TimelineEntry, title string, visibility core.TimelineVisibility) error {
		_, err := test.service.UpdateTimelineEntry(test.context, test.dm, entry.ID, core.TimelineEntryInput{Day: entry.Day, Title: title, Visibility: visibility})
		return err
	}
	update := func(npc core.Npc, name string, knownName string, revealed bool) {
		t.Helper()
		if _, err := test.service.UpdateNpc(test.context, test.dm, npc.ID, core.NpcInput{Name: name, KnownName: knownName, Revealed: revealed}); err != nil {
			t.Fatal(err)
		}
	}
	playerSees := func(fragment string) bool {
		return slices.ContainsFunc(test.timeline(test.player, 0, 0), func(entry core.TimelineEntry) bool { return strings.Contains(entry.Title, fragment) })
	}

	// A spelling fix to the true name leaves the entry's old true name unknown to the party.
	tinker := test.npc(core.NpcInput{Name: "Edgar Vale"})
	joined := test.join(tinker.ID, "FARMER", core.JoinModeNewColonist).Entries[0]
	update(tinker, "Edgar Vale", "The Tinker", true)
	update(tinker, "Edgar Vael", "The Tinker", true)
	if err := edit(joined, joined.Title, core.TimelineVisibilityPublic); !hasField(err, "visibility") {
		t.Fatalf("%q went public after a rename: %v", joined.Title, err)
	}

	// Editing text that is already PUBLIC is checked too.
	changed := test.must(test.service.ChangeColonyRole(test.context, test.dm, tinker.ID, core.ChangeColonyRoleInput{ExpectedVersion: test.board().Version, RoleKey: "BUILDER", Public: true})).Entries[0]
	if changed.Title != "The Tinker: Farmer → Builder" || changed.Dm.Visibility != core.TimelineVisibilityPublic {
		t.Fatalf("the public role change = %+v", changed)
	}
	if err := edit(changed, "Edgar Vael: Farmer → Builder", core.TimelineVisibilityPublic); !hasField(err, "visibility") {
		t.Fatalf("a public entry was edited to the true name: %v", err)
	}
	if err := edit(changed, "The Tinker: Farmer → Builder, finally", core.TimelineVisibilityPublic); err != nil {
		t.Fatalf("an edit by the known name: %v", err)
	}

	// A DM-only UNDO carries the hidden NPC it names.
	wynn := test.npc(core.NpcInput{Name: "Wynn Hollow"})
	rewound := test.undo(test.join(wynn.ID, "FARMER", core.JoinModeNewColonist)).Entries[0]
	if rewound.Title != "The DM rewound: Wynn Hollow joined the colony as Farmer" {
		t.Fatalf("the undo entry = %+v", rewound)
	}
	if err := edit(rewound, rewound.Title, core.TimelineVisibilityPublic); !hasField(err, "visibility") {
		t.Fatalf("an undo naming a hidden NPC went public: %v", err)
	}

	// Giving a revealed NPC a known name takes their true name off the players' timeline, and an undo
	// never writes it again.
	tam := test.npc(core.NpcInput{Name: "Grey Tam", Revealed: true})
	tamJoined := test.join(tam.ID, "FARMER", core.JoinModeNewColonist)
	if !playerSees("Grey Tam joined") {
		t.Fatal("a revealed colonist's entry is missing")
	}
	update(tam, "Grey Tam", "The Grey One", true)
	if playerSees("Grey Tam") {
		t.Fatal("players still read the true name after the DM set a known name")
	}
	test.undo(tamJoined)
	if playerSees("Grey Tam") {
		t.Fatal("the undo showed players the true name")
	}

	// Deleting a hidden NPC doesn't bring their entries back, and their names stay off PUBLIC.
	pip := test.npc(core.NpcInput{Name: "Pip Lark", Revealed: true})
	pipJoined := test.join(pip.ID, "FARMER", core.JoinModeNewColonist).Entries[0]
	if _, err := test.service.SetNpcRevealed(test.context, test.dm, pip.ID, false); err != nil {
		t.Fatal(err)
	}
	test.leave(pip.ID, core.LeaveModeRemove)
	if err := test.service.DeleteNpc(test.context, test.dm, pip.ID); err != nil {
		t.Fatal(err)
	}
	if playerSees("Pip Lark") {
		t.Fatal("deleting a hidden NPC showed players their entries again")
	}
	if err := edit(pipJoined, pipJoined.Title, core.TimelineVisibilityPublic); !hasField(err, "visibility") {
		t.Fatalf("a deleted hidden NPC's entry went public: %v", err)
	}

	// A revealed NPC's entries under the name players know stay editable after the delete.
	orsolya := test.npc(core.NpcInput{Name: "Orsolya Dane", Revealed: true})
	orsolyaJoined := test.join(orsolya.ID, "FARMER", core.JoinModeNewColonist).Entries[0]
	test.leave(orsolya.ID, core.LeaveModeRemove)
	if err := test.service.DeleteNpc(test.context, test.dm, orsolya.ID); err != nil {
		t.Fatal(err)
	}
	if err := edit(orsolyaJoined, "Orsolya Dane joined the colony as a Farmer", core.TimelineVisibilityPublic); err != nil || !playerSees("Orsolya Dane joined") {
		t.Fatalf("a deleted revealed NPC's entry: %v", err)
	}
}
