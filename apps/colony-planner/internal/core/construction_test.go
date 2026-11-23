package core_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

func (test *colonyTest) construction(viewer core.Viewer) core.ConstructionPage {
	test.t.Helper()
	page, err := test.service.ListConstruction(test.context, viewer)
	if err != nil {
		test.t.Fatalf("list construction: %v", err)
	}
	return page
}

// granary is the brief's sample blueprint: materials 30 and gold 20, 40 Builder-days with at most 4
// at once, and morale +5 when it finishes.
func granary(revealed bool) core.NewBlueprintInput {
	maxCrew := int64(4)
	return core.NewBlueprintInput{Revealed: revealed, BlueprintInput: core.BlueprintInput{
		Name:              "Granary",
		Description:       "A raised store for grain.",
		DmNotes:           "Rats come with it.",
		Cost:              map[string]int64{"materials": 30, "gold": 20},
		Labor:             map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 40, MaxCrew: &maxCrew}},
		CompletionEffects: core.Effects{Resources: map[string]int64{"morale": 5}},
	}}
}

func (test *colonyTest) blueprint(input core.NewBlueprintInput) core.BuildingBlueprint {
	test.t.Helper()
	blueprint, err := test.service.CreateBlueprint(test.context, test.dm, input)
	if err != nil {
		test.t.Fatalf("create blueprint %s: %v", input.Name, err)
	}
	return blueprint
}

func (test *colonyTest) startBuilding(viewer core.Viewer, blueprintID uuid.UUID, crews map[string]int64) (core.ColonyChange, error) {
	test.t.Helper()
	updatedAt := time.Time{}
	for _, blueprint := range test.construction(test.dm).Blueprints {
		if blueprint.ID == blueprintID {
			updatedAt = blueprint.UpdatedAt
		}
	}
	return test.service.StartBuilding(test.context, viewer, core.StartBuildingInput{
		ExpectedVersion: test.board().Version, BlueprintID: blueprintID, BlueprintUpdatedAt: updatedAt, Crews: crews,
	})
}

func (test *colonyTest) setCrews(viewer core.Viewer, buildingID uuid.UUID, crews map[string]int64) (core.ColonyChange, error) {
	test.t.Helper()
	return test.service.SetBuildingCrews(test.context, viewer, buildingID, core.BuildingCrewsInput{ExpectedVersion: test.board().Version, Crews: crews})
}

// building is the one building of the page, as viewer sees it.
func (test *colonyTest) building(viewer core.Viewer, buildingID uuid.UUID) (core.Building, bool) {
	test.t.Helper()
	page := test.construction(viewer)
	index := slices.IndexFunc(page.Buildings, func(building core.Building) bool { return building.ID == buildingID })
	if index < 0 {
		return core.Building{}, false
	}
	return page.Buildings[index], true
}

func roleLine(page core.ConstructionPage, roleKey string) core.ConstructionRole {
	for _, role := range page.Roles {
		if role.RoleKey == roleKey {
			return role
		}
	}
	return core.ConstructionRole{}
}

func newConstructionTest(t *testing.T) *colonyTest {
	t.Helper()
	test := newColonyTest(t)
	test.setHeadcounts(map[string]int32{"BUILDER": 4, "MAGE": 2, "UNASSIGNED": 1})
	test.setTo(map[string]int64{"materials": 50, "gold": 40, "food": 100, "morale": 80})
	return test
}

func TestStartingABuildingTakesItsCostStrictAndUndoDeletesIt(t *testing.T) {
	test := newConstructionTest(t)
	blueprint := test.blueprint(granary(true))

	test.setTo(map[string]int64{"materials": 10})
	_, err := test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 2})
	requireMessage(t, err, core.KindConflict, "Not enough materials: the colony has 10, needs 30.")
	test.setTo(map[string]int64{"materials": 50})

	started := test.must(test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 2}))
	if len(started.Entries) != 1 || started.Entries[0].Kind != core.TimelineKindBuildingStarted || started.Entries[0].Title != "Construction started: Granary" ||
		started.Entries[0].Dm.Visibility != core.TimelineVisibilityDm || started.Entries[0].BuildingID == nil {
		t.Fatalf("start entries = %+v", started.Entries)
	}
	if test.amount("materials") != 20 || test.amount("gold") != 20 {
		t.Fatalf("materials %d, gold %d after Start", test.amount("materials"), test.amount("gold"))
	}
	buildingID := *started.Entries[0].BuildingID
	building, found := test.building(test.player, buildingID)
	if !found || building.Status != core.BuildingStatusConstructing || building.Cost["materials"] != 30 || building.Labor[0].Crew != 2 ||
		building.ProjectedFinishDay == nil || *building.ProjectedFinishDay != test.board().Day+20 {
		t.Fatalf("the started building = %+v", building)
	}
	if line := roleLine(test.construction(test.player), "BUILDER"); line.Headcount != 4 || line.Crewed != 2 || line.Idle != 2 {
		t.Fatalf("Builders = %+v", line)
	}
	if slices.ContainsFunc(test.timeline(test.player, 0, 30), func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingStarted }) {
		t.Fatal("players see the DM-only Start entry")
	}

	undone := test.undo(started)
	if undone.Label != "The DM rewound a change to the colony" {
		t.Fatalf("undo label %q", undone.Label)
	}
	if _, found := test.building(test.dm, buildingID); found || test.amount("materials") != 50 || test.amount("gold") != 40 {
		t.Fatalf("after undo: building found %v, materials %d, gold %d", found, test.amount("materials"), test.amount("gold"))
	}
}

func TestCrewGuardsRefuseStrandingAndOverbooking(t *testing.T) {
	test := newConstructionTest(t)
	blueprint := test.blueprint(granary(true))
	started := test.must(test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 4}))
	buildingID := *started.Entries[0].BuildingID

	_, err := test.service.SetRoleCount(test.context, test.dm, core.SetRoleCountInput{ExpectedVersion: test.board().Version, RoleKey: "BUILDER", Count: 3})
	requireMessage(t, err, core.KindInvalid, "4 Builders are on projects; take 1 off a project first.")
	_, err = test.service.TransferColonists(test.context, test.dm, core.TransferInput{ExpectedVersion: test.board().Version, FromRoleKey: "BUILDER", ToRoleKey: "UNASSIGNED", Count: 1})
	requireMessage(t, err, core.KindInvalid, "4 Builders are on projects; take 1 off a project first.")

	second := test.blueprint(core.NewBlueprintInput{Revealed: true, BlueprintInput: core.BlueprintInput{
		Name: "Shed", Cost: map[string]int64{}, Labor: map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 5}},
	}})
	_, err = test.startBuilding(test.player, second.ID, map[string]int64{"BUILDER": 1})
	requireMessage(t, err, core.KindInvalid, "No Builders are free.")
	_, err = test.setCrews(test.player, buildingID, map[string]int64{"BUILDER": 5})
	requireMessage(t, err, core.KindInvalid, "At most 4 Builders can work on this.")
	_, err = test.setCrews(test.player, buildingID, map[string]int64{"BUILDER": 4})
	requireMessage(t, err, core.KindInvalid, "Nothing would change: those are the crews it has.")

	moved := test.must(test.setCrews(test.player, buildingID, map[string]int64{"BUILDER": 2}))
	if moved.Label != "Granary crews: Builders 4 → 2" || moved.Entries[0].Dm.Visibility != core.TimelineVisibilityDm {
		t.Fatalf("set crews = %q, %+v", moved.Label, moved.Entries)
	}
	test.setCount("BUILDER", 2)
	_, err = test.service.SetRoleCount(test.context, test.dm, core.SetRoleCountInput{ExpectedVersion: test.board().Version, RoleKey: "BUILDER", Count: 1})
	requireMessage(t, err, core.KindInvalid, "2 Builders are on projects; take 1 off a project first.")
}

func TestAdvanceFinishesABuildingAndUndoUnfinishesIt(t *testing.T) {
	test := newConstructionTest(t)
	test.correctDay(10)
	blueprint := test.blueprint(granary(true))
	started := test.must(test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 4}))
	buildingID := *started.Entries[0].BuildingID

	preview, err := test.service.PreviewAdvance(test.context, test.dm, core.AdvancePreviewInput{Days: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Finishes) != 1 || preview.Finishes[0] != (core.AdvanceFinish{BuildingID: buildingID, Name: "Granary", Day: 20}) ||
		!strings.Contains(preview.Summary, "The Granary is finished on Day 20") {
		t.Fatalf("preview = %+v", preview)
	}
	advanced := test.advance(12)
	finished := slices.IndexFunc(advanced.Entries, func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingFinished })
	if finished < 0 {
		t.Fatalf("no finish entry: %+v", advanced.Entries)
	}
	entry := advanced.Entries[finished]
	if entry.Day != 20 || entry.Title != "The Granary is finished" || entry.Dm.Visibility != core.TimelineVisibilityPublic ||
		entry.Deltas.Resources["morale"] != 5 || entry.BuildingID == nil || *entry.BuildingID != buildingID {
		t.Fatalf("finish entry = %+v", entry)
	}
	if test.amount("morale") != 85 {
		t.Fatalf("morale %d, want 85", test.amount("morale"))
	}
	building, _ := test.building(test.player, buildingID)
	if building.Status != core.BuildingStatusFinished || building.FinishedDay == nil || *building.FinishedDay != 20 ||
		building.Labor[0].Progress != 40 || building.Labor[0].Crew != 0 || building.ProjectedFinishDay != nil {
		t.Fatalf("finished building = %+v", building)
	}
	if line := roleLine(test.construction(test.player), "BUILDER"); line.Idle != 4 || line.Crewed != 0 {
		t.Fatalf("Builders after the finish = %+v", line)
	}
	if !slices.ContainsFunc(test.timeline(test.player, 0, 30), func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingFinished }) {
		t.Fatal("players don't see a revealed blueprint's finish")
	}

	test.undo(advanced)
	building, _ = test.building(test.dm, buildingID)
	if building.Status != core.BuildingStatusConstructing || building.FinishedDay != nil || building.Labor[0].Progress != 0 || building.Labor[0].Crew != 4 ||
		test.amount("morale") != 80 || test.board().Day != 10 {
		t.Fatalf("after undo: %+v, morale %d, day %d", building, test.amount("morale"), test.board().Day)
	}
}

func TestCancelAndDemolishGiveBackWhatTheDmTypes(t *testing.T) {
	test := newConstructionTest(t)
	blueprint := test.blueprint(granary(true))
	started := test.must(test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 3}))
	buildingID := *started.Entries[0].BuildingID
	refund := func(viewer core.Viewer, cancel bool, amounts map[string]int64) (core.ColonyChange, error) {
		input := core.BuildingRefundInput{ExpectedVersion: test.board().Version, Refund: amounts}
		if cancel {
			return test.service.CancelBuilding(test.context, viewer, buildingID, input)
		}
		return test.service.DemolishBuilding(test.context, viewer, buildingID, input)
	}

	_, err := refund(test.player, true, map[string]int64{})
	if !isKind(err, core.KindForbidden) {
		t.Fatalf("a player cancelling: %v", err)
	}
	_, err = refund(test.dm, false, map[string]int64{})
	requireMessage(t, err, core.KindConflict, "Only a finished building can be demolished.")
	_, err = refund(test.dm, true, map[string]int64{"materials": -1})
	requireMessage(t, err, core.KindInvalid, "Check the refund.")

	test.setTo(map[string]int64{"morale": 98})
	cancelled := test.must(refund(test.dm, true, map[string]int64{"materials": 25, "morale": 5}))
	if cancelled.Label != "Construction cancelled: Granary" || !reflectEqualDeltas(cancelled.Entries[0].Deltas.Resources, map[string]int64{"materials": 25, "morale": 2}) {
		t.Fatalf("cancel = %q, %+v", cancelled.Label, cancelled.Entries[0].Deltas)
	}
	building, _ := test.building(test.dm, buildingID)
	if building.Status != core.BuildingStatusCancelled || building.EndedDay == nil || building.Refund["morale"] != 2 || building.Labor[0].Crew != 0 {
		t.Fatalf("cancelled building = %+v", building)
	}
	if line := roleLine(test.construction(test.dm), "BUILDER"); line.Idle != 4 {
		t.Fatalf("Builders after cancelling = %+v", line)
	}

	test.undo(cancelled)
	building, _ = test.building(test.dm, buildingID)
	if building.Status != core.BuildingStatusConstructing || building.EndedDay != nil || building.Refund != nil || building.Labor[0].Crew != 3 || test.amount("materials") != 20 {
		t.Fatalf("after undoing the cancel: %+v, materials %d", building, test.amount("materials"))
	}
}

func reflectEqualDeltas(got map[string]int64, want map[string]int64) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func TestHiddenBlueprintsAreHiddenRows(t *testing.T) {
	test := newConstructionTest(t)
	hidden := granary(false)
	hidden.Name = "Warded Tower"
	hidden.Labor = map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 4}, "MAGE": {Effort: 2}}
	tower := test.blueprint(hidden)

	_, err := test.startBuilding(test.player, tower.ID, map[string]int64{})
	requireMessage(t, err, core.KindInvalid, "That blueprint doesn't exist.")
	started := test.must(test.startBuilding(test.dm, tower.ID, map[string]int64{"BUILDER": 2, "MAGE": 2}))
	buildingID := *started.Entries[0].BuildingID

	page := test.construction(test.player)
	if len(page.Blueprints) != 0 || len(page.Buildings) != 0 {
		t.Fatalf("players see the hidden blueprint: %+v", page)
	}
	for _, role := range page.Roles {
		if role.RoleKey == "MAGE" {
			t.Fatalf("players see a role only the hidden blueprint needs: %+v", page.Roles)
		}
	}
	if line := roleLine(page, "BUILDER"); line.Crewed != 0 || line.Idle != 2 {
		t.Fatalf("hidden crews must count only as busy elsewhere: %+v", line)
	}
	_, err = test.setCrews(test.player, buildingID, map[string]int64{"BUILDER": 1})
	requireMessage(t, err, core.KindNotFound, "That building doesn't exist.")

	// It finishes in two days; players never see the finish while the blueprint is hidden.
	_, err = test.service.PreviewAdvance(test.context, test.dm.AsPlayer(), core.AdvancePreviewInput{Days: 3})
	requireMessage(t, err, core.KindForbidden, "View as player is on.")
	advanced := test.advance(3)
	finish := advanced.Entries[slices.IndexFunc(advanced.Entries, func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingFinished })]
	if slices.ContainsFunc(test.timeline(test.player, 0, 30), func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingFinished }) {
		t.Fatal("players see a hidden blueprint's finish")
	}
	_, err = test.service.UpdateTimelineEntry(test.context, test.dm, started.Entries[0].ID, core.TimelineEntryInput{
		Day: started.Entries[0].Day, Title: started.Entries[0].Title, Visibility: core.TimelineVisibilityPublic,
	})
	requireMessage(t, err, core.KindInvalid, "The Warded Tower blueprint is hidden from players. Reveal it first.")

	if _, err := test.service.SetBlueprintRevealed(test.context, test.dm, tower.ID, true); err != nil {
		t.Fatal(err)
	}
	visible := test.timeline(test.player, 0, 30)
	if !slices.ContainsFunc(visible, func(entry core.TimelineEntry) bool { return entry.ID == finish.ID }) {
		t.Fatal("revealing the blueprint doesn't show its finish")
	}
}

func TestConstructionRoleAndArchiveGuards(t *testing.T) {
	test := newConstructionTest(t)
	settings, err := test.service.GetSettings(test.context, test.dm)
	if err != nil || settings.ConstructionRoleKey == nil || *settings.ConstructionRoleKey != "BUILDER" {
		t.Fatalf("approval's construction role = %v, %v", settings.ConstructionRoleKey, err)
	}
	role := func(key string, label string, archived bool) error {
		_, err := test.service.UpdateRole(test.context, test.dm, key, core.RoleInput{Label: label, Icon: core.ColonyIconHardHat, SortOrder: 60, Archived: archived})
		return err
	}
	test.setCount("BUILDER", 0)
	requireMessage(t, role("BUILDER", "Builder", true), core.KindInvalid, "The role needs fixing.")

	blueprint := test.blueprint(core.NewBlueprintInput{Revealed: true, BlueprintInput: core.BlueprintInput{
		Name: "Library", Cost: map[string]int64{}, Labor: map[string]core.BlueprintLaborInput{"SCHOLAR": {Effort: 5}},
	}})
	test.must(test.startBuilding(test.dm, blueprint.ID, map[string]int64{}))
	requireMessage(t, role("SCHOLAR", "Scholar", true), core.KindInvalid, "The role needs fixing.")

	input := core.SettingsInput{CampaignName: "Sample", ColonyName: "The Colony", Timezone: "UTC"}
	if updated, err := test.service.UpdateSettings(test.context, test.dm, input); err != nil || updated.ConstructionRoleKey != nil {
		t.Fatalf("clearing the construction role: %+v, %v", updated.ConstructionRoleKey, err)
	}
	if err := role("BUILDER", "Builder", true); err != nil {
		t.Fatalf("archiving the cleared construction role: %v", err)
	}
	builder := "BUILDER"
	input.ConstructionRoleKey = &builder
	_, err = test.service.UpdateSettings(test.context, test.dm, input)
	requireMessage(t, err, core.KindInvalid, "Some settings need fixing.")
}

func TestBlueprintChecks(t *testing.T) {
	test := newConstructionTest(t)
	test.blueprint(granary(false))
	duplicate := granary(true)
	duplicate.Name = "granary"
	_, err := test.service.CreateBlueprint(test.context, test.dm, duplicate)
	requireMessage(t, err, core.KindInvalid, "Check the blueprint.")
	empty := granary(true)
	empty.Name = "Empty"
	empty.Labor = map[string]core.BlueprintLaborInput{}
	_, err = test.service.CreateBlueprint(test.context, test.dm, empty)
	requireMessage(t, err, core.KindInvalid, "Check the blueprint.")
	if _, err := test.service.CreateBlueprint(test.context, test.player, granary(true)); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player creating a blueprint: %v", err)
	}
	page := test.construction(test.dm)
	if len(page.Blueprints) != 1 || page.Blueprints[0].Dm == nil || page.Blueprints[0].Dm.DmNotes != "Rats come with it." || page.Blueprints[0].Dm.Revealed {
		t.Fatalf("the DM's blueprints = %+v", page.Blueprints)
	}
}

func (test *colonyTest) refund(cancel bool, buildingID uuid.UUID, amounts map[string]int64) (core.ColonyChange, error) {
	test.t.Helper()
	input := core.BuildingRefundInput{ExpectedVersion: test.board().Version, Refund: amounts}
	if cancel {
		return test.service.CancelBuilding(test.context, test.dm, buildingID, input)
	}
	return test.service.DemolishBuilding(test.context, test.dm, buildingID, input)
}

// shed is a revealed blueprint of 4 Builder-days that costs nothing, with completion Effects.
func (test *colonyTest) shed(name string, completion core.Effects) core.BuildingBlueprint {
	test.t.Helper()
	return test.blueprint(core.NewBlueprintInput{Revealed: true, BlueprintInput: core.BlueprintInput{
		Name: name, Cost: map[string]int64{}, Labor: map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 4}}, CompletionEffects: completion,
	}})
}

func TestDemolishingAFinishedBuildingAndItsUndo(t *testing.T) {
	test := newConstructionTest(t)
	shed := test.shed("Shed", core.Effects{})
	buildingID := *test.must(test.startBuilding(test.player, shed.ID, map[string]int64{"BUILDER": 4})).Entries[0].BuildingID
	test.advance(1)
	demolished := test.must(test.refund(false, buildingID, map[string]int64{"materials": 5}))
	if demolished.Label != "Shed demolished" || demolished.Entries[0].Kind != core.TimelineKindBuildingDemolished || demolished.Entries[0].Dm.Visibility != core.TimelineVisibilityDm {
		t.Fatalf("demolish = %q, %+v", demolished.Label, demolished.Entries)
	}
	building, _ := test.building(test.dm, buildingID)
	if building.Status != core.BuildingStatusDemolished || building.FinishedDay == nil || building.EndedDay == nil || building.Refund["materials"] != 5 || test.amount("materials") != 55 {
		t.Fatalf("demolished building = %+v, materials %d", building, test.amount("materials"))
	}
	_, err := test.refund(true, buildingID, map[string]int64{})
	requireMessage(t, err, core.KindConflict, "Only a building under construction can be cancelled.")

	test.undo(demolished)
	building, _ = test.building(test.dm, buildingID)
	if building.Status != core.BuildingStatusFinished || building.FinishedDay == nil || building.EndedDay != nil || building.Refund != nil || test.amount("materials") != 50 {
		t.Fatalf("after undoing the demolish: %+v, materials %d", building, test.amount("materials"))
	}
}

func TestUndoingSetCrewsPutsTheCrewsBack(t *testing.T) {
	test := newConstructionTest(t)
	blueprint := test.blueprint(granary(true))
	buildingID := *test.must(test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 1})).Entries[0].BuildingID
	moved := test.must(test.setCrews(test.player, buildingID, map[string]int64{"BUILDER": 4}))
	undone := test.undo(moved)
	if undone.Label != "The DM rewound: Granary crews: Builders 1 → 4" || undone.Entries[0].BuildingID == nil || *undone.Entries[0].BuildingID != buildingID {
		t.Fatalf("undo = %q, %+v", undone.Label, undone.Entries)
	}
	if building, _ := test.building(test.dm, buildingID); building.Labor[0].Crew != 1 {
		t.Fatalf("crew after undo = %+v", building.Labor)
	}
}

func TestUndoingAHiddenStartNamesNothingAndHiddenUndosStayDmOnly(t *testing.T) {
	test := newConstructionTest(t)
	hidden := granary(false)
	hidden.Name = "Warded Tower"
	tower := test.blueprint(hidden)
	started := test.must(test.startBuilding(test.dm, tower.ID, map[string]int64{"BUILDER": 1}))
	undone := test.undo(started)
	if undone.Label != "The DM rewound a change to the colony" || strings.Contains(undone.Entries[0].Title, "Warded") || undone.Entries[0].BuildingID != nil {
		t.Fatalf("undoing a hidden Start = %+v", undone.Entries[0])
	}

	buildingID := *test.must(test.startBuilding(test.dm, tower.ID, map[string]int64{"BUILDER": 1})).Entries[0].BuildingID
	crewed := test.must(test.setCrews(test.dm, buildingID, map[string]int64{"BUILDER": 2}))
	undo := test.undo(crewed)
	_, err := test.service.UpdateTimelineEntry(test.context, test.dm, undo.Entries[0].ID, core.TimelineEntryInput{
		Day: undo.Entries[0].Day, Title: undo.Entries[0].Title, Visibility: core.TimelineVisibilityPublic,
	})
	requireMessage(t, err, core.KindInvalid, "The Warded Tower blueprint is hidden from players. Reveal it first.")
}

func TestRenamingAHiddenBlueprintHidesItsPublicEntries(t *testing.T) {
	test := newConstructionTest(t)
	hidden := test.shed("Warded Tower", core.Effects{})
	if _, err := test.service.SetBlueprintRevealed(test.context, test.dm, hidden.ID, false); err != nil {
		t.Fatal(err)
	}
	test.must(test.startBuilding(test.dm, hidden.ID, map[string]int64{"BUILDER": 4}))
	advanced := test.advance(1)
	finish := advanced.Entries[slices.IndexFunc(advanced.Entries, func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingFinished })]
	if finish.Dm.Visibility != core.TimelineVisibilityPublic {
		t.Fatalf("finish entry = %+v", finish)
	}
	input := core.BlueprintInput{Name: "Lookout", Cost: map[string]int64{}, Labor: map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 4}}}
	if _, err := test.service.UpdateBlueprint(test.context, test.dm, hidden.ID, input); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetBlueprintRevealed(test.context, test.dm, hidden.ID, true); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(test.timeline(test.player, 0, 30), func(entry core.TimelineEntry) bool { return entry.ID == finish.ID }) {
		t.Fatal("revealing a renamed blueprint shows players the old name")
	}
	entries := test.timeline(test.dm, 0, 30)
	if index := slices.IndexFunc(entries, func(entry core.TimelineEntry) bool { return entry.ID == finish.ID }); entries[index].Dm.Visibility != core.TimelineVisibilityDm {
		t.Fatalf("the renamed blueprint's finish = %+v", entries[index])
	}
}

func TestStartRefusesAStaleSheetAndAnArchivedCostResource(t *testing.T) {
	test := newConstructionTest(t)
	blueprint := test.blueprint(granary(true))
	_, err := test.service.StartBuilding(test.context, test.player, core.StartBuildingInput{
		ExpectedVersion: test.board().Version, BlueprintID: blueprint.ID, BlueprintUpdatedAt: blueprint.UpdatedAt.Add(-time.Second), Crews: map[string]int64{},
	})
	requireMessage(t, err, core.KindConflict, "The blueprint changed. Review it and start again.")

	timber, err := test.service.CreateResource(test.context, test.dm, core.NewResourceInput{Label: "Timber", Icon: core.ColonyIconTrees})
	if err != nil {
		t.Fatal(err)
	}
	test.setTo(map[string]int64{timber.Key: 10})
	timbered := test.blueprint(core.NewBlueprintInput{Revealed: true, BlueprintInput: core.BlueprintInput{
		Name: "Cabin", Cost: map[string]int64{timber.Key: 5}, Labor: map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 4}},
	}})
	if _, err := test.service.UpdateResource(test.context, test.dm, timber.Key, core.ResourceInput{Label: "Timber", Icon: core.ColonyIconTrees, SortOrder: timber.SortOrder, Archived: true}); err != nil {
		t.Fatalf("archive timber: %v", err)
	}
	_, err = test.startBuilding(test.player, timbered.ID, map[string]int64{})
	requireMessage(t, err, core.KindInvalid, "Timber is archived.")
}

func TestCompletionColonistsLandInUnassignedAndUndoTakesThemBack(t *testing.T) {
	test := newConstructionTest(t)
	shed := test.shed("Bunkhouse", core.Effects{Colonists: 3})
	test.must(test.startBuilding(test.player, shed.ID, map[string]int64{"BUILDER": 4}))
	advanced := test.advance(2)
	finish := advanced.Entries[slices.IndexFunc(advanced.Entries, func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingFinished })]
	if finish.Deltas.Colonists != 3 || test.headcount(test.dm, "UNASSIGNED").Total != 4 {
		t.Fatalf("finish deltas %+v, Unassigned %d", finish.Deltas, test.headcount(test.dm, "UNASSIGNED").Total)
	}
	test.undo(advanced)
	if test.headcount(test.dm, "UNASSIGNED").Total != 1 {
		t.Fatalf("Unassigned after undo = %d", test.headcount(test.dm, "UNASSIGNED").Total)
	}
}

func TestUndoRefusesToRestartAProjectWhoseRoleWasArchived(t *testing.T) {
	test := newConstructionTest(t)
	test.setHeadcounts(map[string]int32{"SCHOLAR": 1})
	library := test.blueprint(core.NewBlueprintInput{Revealed: true, BlueprintInput: core.BlueprintInput{
		Name: "Library", Cost: map[string]int64{}, Labor: map[string]core.BlueprintLaborInput{"SCHOLAR": {Effort: 3}},
	}})
	buildingID := *test.must(test.startBuilding(test.dm, library.ID, map[string]int64{"SCHOLAR": 1})).Entries[0].BuildingID
	cancelled := test.must(test.refund(true, buildingID, map[string]int64{}))
	if err := store.New(test.database.Pool).SetRoleUnnamedCount(test.context, store.SetRoleUnnamedCountParams{CampaignID: test.campaignID, Key: "SCHOLAR", UnnamedCount: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.UpdateRole(test.context, test.dm, "SCHOLAR", core.RoleInput{Label: "Scholar", Icon: core.ColonyIconBookOpen, SortOrder: 70, Archived: true}); err != nil {
		t.Fatalf("archive Scholar: %v", err)
	}
	_, err := test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: cancelled.GroupID})
	requireMessage(t, err, core.KindConflict, "Undo can't put the Library back under construction: its Scholar role was archived since.")
}

func TestNamedColonistsCannotStrandCrews(t *testing.T) {
	test := newConstructionTest(t)
	mason := test.npc(core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Mason"}})
	test.join(mason.ID, "BUILDER", core.JoinModePromoteUnnamed)
	blueprint := test.blueprint(granary(true))
	test.must(test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 4}))

	_, err := test.service.ChangeColonyRole(test.context, test.dm, mason.ID, core.ChangeColonyRoleInput{ExpectedVersion: test.board().Version, RoleKey: "FARMER"})
	requireMessage(t, err, core.KindInvalid, "4 Builders are on projects; take 1 off a project first.")
	_, err = test.service.LeaveColony(test.context, test.dm, mason.ID, core.LeaveColonyInput{ExpectedVersion: test.board().Version, Mode: core.LeaveModeRemove})
	requireMessage(t, err, core.KindInvalid, "4 Builders are on projects; take 1 off a project first.")
	// Promoting and returning to the pool keep the headcount, so they never strand a crew.
	test.leave(mason.ID, core.LeaveModeReturnToPool)
	test.join(mason.ID, "BUILDER", core.JoinModePromoteUnnamed)
}

// A member-level change answers a player without the hidden resource's delta.
func TestPlayersStartingABuildingDontSeeAHiddenCost(t *testing.T) {
	test := newConstructionTest(t)
	resources, err := test.service.ListResources(test.context, test.dm)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	index := slices.IndexFunc(resources.Resources, func(resource core.ResourceSettings) bool { return resource.Key == "morale" })
	morale := resources.Resources[index]
	if _, err := test.service.UpdateResource(test.context, test.dm, "morale", core.ResourceInput{
		Label: morale.Label, Icon: morale.Icon, MinAmount: morale.MinAmount, MaxAmount: morale.MaxAmount, SortOrder: morale.SortOrder, Hidden: true,
	}); err != nil {
		t.Fatalf("hide morale: %v", err)
	}
	input := granary(true)
	input.Cost = map[string]int64{"materials": 30, "morale": 3}
	blueprint := test.blueprint(input)
	started := test.must(test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 2}))
	if _, found := started.Entries[0].Deltas.Resources["morale"]; found || started.Entries[0].Deltas.Resources["materials"] != -30 {
		t.Errorf("the player's Start entry has deltas %v; want materials only", started.Entries[0].Deltas.Resources)
	}
	if test.amount("morale") != 77 {
		t.Errorf("morale is %d; want 77 after the hidden cost", test.amount("morale"))
	}
}
