package core_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// boardMax is a resource's max on the board: the effective one.
func (test *colonyTest) boardMax(key string) *int64 {
	test.t.Helper()
	for _, resource := range test.board().Resources {
		if resource.Key == key {
			return resource.MaxAmount
		}
	}
	test.t.Fatalf("no resource %q on the board", key)
	return nil
}

// resourceSettings is a resource as /settings lists it.
func (test *colonyTest) resourceSettings(key string) core.ResourceSettings {
	test.t.Helper()
	list, err := test.service.ListResources(test.context, test.dm)
	if err != nil {
		test.t.Fatalf("list resources: %v", err)
	}
	index := slices.IndexFunc(list.Resources, func(resource core.ResourceSettings) bool { return resource.Key == key })
	if index < 0 {
		test.t.Fatalf("no resource %q on /settings", key)
	}
	return list.Resources[index]
}

// updateResource replaces a resource's settings with a new max (nil for none) or archived flag.
func (test *colonyTest) updateResource(key string, maxAmount *int64, archived bool) (core.ResourceSettings, error) {
	test.t.Helper()
	current := test.resourceSettings(key)
	return test.service.UpdateResource(test.context, test.dm, key, core.ResourceInput{
		Label: current.Label, Icon: current.Icon, MinAmount: current.MinAmount, MaxAmount: maxAmount, SortOrder: current.SortOrder, Archived: archived,
	})
}

// standingBuilding raises a revealed blueprint of 4 Builder-days with output and finishes it with one
// advance, returning the building's ID.
func (test *colonyTest) standingBuilding(name string, output map[string]core.BlueprintOutput) uuid.UUID {
	test.t.Helper()
	blueprint := test.blueprint(core.NewBlueprintInput{Revealed: true, BlueprintInput: core.BlueprintInput{
		Name: name, Cost: map[string]int64{}, Labor: map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 4}}, Output: output,
	}})
	buildingID := *test.must(test.startBuilding(test.player, blueprint.ID, map[string]int64{"BUILDER": 4})).Entries[0].BuildingID
	test.advance(1)
	return buildingID
}

func (test *colonyTest) switchBuilding(buildingID uuid.UUID, active bool) (core.ColonyChange, error) {
	test.t.Helper()
	input := core.BuildingVersionInput{ExpectedVersion: test.board().Version}
	if active {
		return test.service.ActivateBuilding(test.context, test.dm, buildingID, input)
	}
	return test.service.IdleBuilding(test.context, test.dm, buildingID, input)
}

func newOutputTest(t *testing.T) *colonyTest {
	t.Helper()
	test := newConstructionTest(t)
	food := int64(100)
	if _, err := test.updateResource("food", &food, false); err != nil {
		t.Fatalf("cap food at 100: %v", err)
	}
	return test
}

func TestIdleAndActiveMoveTheCapAndUndoPutsThemBack(t *testing.T) {
	test := newOutputTest(t)
	granaryID := test.standingBuilding("Granary", map[string]core.BlueprintOutput{"food": {StorageBonus: 200}})
	if max := test.boardMax("food"); max == nil || *max != 300 {
		t.Fatalf("board food max %v, want 300", max)
	}
	if settings := test.resourceSettings("food"); settings.MaxAmount == nil || *settings.MaxAmount != 100 || settings.StorageBonus != 200 {
		t.Fatalf("/settings food = %+v; want its own max of 100 and a bonus of 200", settings)
	}
	test.setTo(map[string]int64{"food": 250})

	_, err := test.service.IdleBuilding(test.context, test.player, granaryID, core.BuildingVersionInput{ExpectedVersion: test.board().Version})
	if !isKind(err, core.KindForbidden) {
		t.Fatalf("a player setting a building Idle: %v", err)
	}
	idled := test.must(test.switchBuilding(granaryID, false))
	entry := idled.Entries[0]
	if idled.Label != "Granary set Idle" || entry.Kind != core.TimelineKindBuildingIdled || entry.Dm.Visibility != core.TimelineVisibilityDm ||
		entry.BuildingID == nil || *entry.BuildingID != granaryID {
		t.Fatalf("idle = %q, %+v", idled.Label, entry)
	}
	if max := test.boardMax("food"); *max != 100 || test.amount("food") != 250 {
		t.Fatalf("after Idle: max %d, food %d; want 250 kept above a cap of 100", *max, test.amount("food"))
	}
	if building, _ := test.building(test.player, granaryID); building.Active || building.Added || building.Output["food"].StorageBonus != 200 {
		t.Fatalf("the Idle building = %+v", building)
	}
	if slices.ContainsFunc(test.timeline(test.player, 0, 30), func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingIdled }) {
		t.Fatal("players see the DM-only Idle entry")
	}
	_, err = test.switchBuilding(granaryID, false)
	requireMessage(t, err, core.KindConflict, "The Granary is already Idle.")

	activated := test.must(test.switchBuilding(granaryID, true))
	if activated.Entries[0].Kind != core.TimelineKindBuildingActivated || *test.boardMax("food") != 300 {
		t.Fatalf("activate = %+v, max %d", activated.Entries, *test.boardMax("food"))
	}
	undone := test.undo(activated)
	if undone.Label != "The DM rewound: Granary set Active" || undone.Entries[0].Dm.Visibility != core.TimelineVisibilityDm {
		t.Fatalf("undo = %q, %+v", undone.Label, undone.Entries)
	}
	if building, _ := test.building(test.dm, granaryID); building.Active || *test.boardMax("food") != 100 {
		t.Fatalf("after undoing Active: %+v, max %d", building, *test.boardMax("food"))
	}

	blueprint := test.shed("Shed", core.Effects{})
	projectID := *test.must(test.startBuilding(test.dm, blueprint.ID, map[string]int64{})).Entries[0].BuildingID
	_, err = test.switchBuilding(projectID, false)
	requireMessage(t, err, core.KindConflict, "Only a finished building can be set Idle or Active.")
}

func TestOutputRunsInTheAdvanceAndItsUndo(t *testing.T) {
	test := newOutputTest(t)
	test.standingBuilding("Mill", map[string]core.BlueprintOutput{"food": {DailyDelta: 10}, "materials": {DailyDelta: -3}})
	test.setTo(map[string]int64{"food": 50, "materials": 5})

	// 7 colonists eat 7 a day beside the Mill's 10; materials run out on the second day.
	preview, err := test.service.PreviewAdvance(test.context, test.dm, core.AdvancePreviewInput{Days: 3})
	if err != nil {
		t.Fatal(err)
	}
	want := []core.ResourceChangePreview{
		{Key: "food", Label: "Food", Before: 50, After: 59, Requested: 9},
		{Key: "materials", Label: "Materials", Before: 5, After: 0, Requested: -9, Clamped: true},
	}
	for index := range preview.Resources {
		preview.Resources[index].Icon = ""
	}
	if !slices.Equal(preview.Resources, want) {
		t.Fatalf("preview resources = %+v, want %+v", preview.Resources, want)
	}
	advanced := test.advance(3)
	days := advanced.Entries[slices.IndexFunc(advanced.Entries, func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindDaysAdvanced })]
	if !reflectEqualDeltas(days.Deltas.Resources, map[string]int64{"food": 9, "materials": -5}) {
		t.Fatalf("DAYS_ADVANCED deltas = %+v", days.Deltas)
	}
	if test.amount("food") != 59 || test.amount("materials") != 0 {
		t.Fatalf("food %d, materials %d after the advance", test.amount("food"), test.amount("materials"))
	}
	test.undo(advanced)
	if test.amount("food") != 50 || test.amount("materials") != 5 {
		t.Fatalf("food %d, materials %d after undo", test.amount("food"), test.amount("materials"))
	}
}

func TestUndoPutsStockBackAboveACapThatFell(t *testing.T) {
	test := newOutputTest(t)
	granaryID := test.standingBuilding("Granary", map[string]core.BlueprintOutput{"food": {StorageBonus: 200}})
	test.setTo(map[string]int64{"food": 250})
	test.must(test.switchBuilding(granaryID, false))
	advanced := test.advance(2)
	if test.amount("food") != 236 {
		t.Fatalf("food %d after two days of 7 eaten, want 236", test.amount("food"))
	}
	test.undo(advanced)
	if test.amount("food") != 250 {
		t.Fatalf("food %d after undo, want 250", test.amount("food"))
	}
}

func TestMarketPayAndAdjustUseTheEffectiveCap(t *testing.T) {
	test := newOutputTest(t)
	granaryID := test.standingBuilding("Granary", map[string]core.BlueprintOutput{"food": {StorageBonus: 200}})
	test.setShipping(1, 1)
	test.setTo(map[string]int64{"food": 297, "gold": 40})
	test.setCart(test.player, test.marketItemID("Food"), 10)
	order := test.placeOrder(test.player)
	test.advance(1)
	paid := test.must(test.payOrder(test.player, order.ID))
	if test.amount("food") != 300 || paid.Entries[0].Deltas.Resources["food"] != 10 {
		t.Fatalf("food %d after paying (deltas %+v); want 290 + 20 capped at 300", test.amount("food"), paid.Entries[0].Deltas)
	}

	test.must(test.switchBuilding(granaryID, false))
	_, err := test.service.AdjustStock(test.context, test.dm, core.AdjustInput{
		ExpectedVersion: test.board().Version, Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"food": 5}, SetTo: map[string]int64{},
	})
	requireMessage(t, err, core.KindInvalid, "Nothing would change: every amount is already where that would put it.")
	preview, err := test.service.PreviewAdjustStock(test.context, test.dm, core.AdjustInput{
		Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"food": 5}, SetTo: map[string]int64{},
	})
	if err != nil || len(preview.Resources) != 1 || preview.Resources[0].After != 300 || !preview.Resources[0].Clamped {
		t.Fatalf("adjust preview = %+v, %v; want food kept at 300, capped", preview.Resources, err)
	}
	test.must(test.service.AdjustStock(test.context, test.dm, core.AdjustInput{
		ExpectedVersion: test.board().Version, Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"food": -10}, SetTo: map[string]int64{},
	}))
	if test.amount("food") != 290 {
		t.Fatalf("food %d after taking 10 above the cap, want 290", test.amount("food"))
	}
}

func TestResourceSettingsReadTheEffectiveMax(t *testing.T) {
	test := newOutputTest(t)
	granaryID := test.standingBuilding("Granary", map[string]core.BlueprintOutput{"food": {StorageBonus: 200}})
	test.setTo(map[string]int64{"food": 250})

	// The DM's own max may sit below the stock while the Granary holds the rest, and an unchanged max is
	// never in the way, even once the Granary is Idle.
	if _, err := test.updateResource("food", pointerTo(int64(100)), false); err != nil {
		t.Fatalf("an edit that keeps the max: %v", err)
	}
	if _, err := test.updateResource("food", pointerTo(int64(60)), false); err != nil {
		t.Fatalf("a max of 60 plus the Granary's 200 holds 250: %v", err)
	}
	test.must(test.switchBuilding(granaryID, false))
	if _, err := test.updateResource("food", pointerTo(int64(60)), false); err != nil {
		t.Fatalf("an unchanged max with the Granary Idle: %v", err)
	}
	_, err := test.updateResource("food", pointerTo(int64(200)), false)
	requireFieldMessage(t, err, "maxAmount", "Food is at 250 now, above that maximum. Adjust it first.")
}

func TestAResourceAStandingBuildingNamesCannotBeArchived(t *testing.T) {
	test := newOutputTest(t)
	timber, err := test.service.CreateResource(test.context, test.dm, core.NewResourceInput{Label: "Timber", Icon: core.ColonyIconTrees})
	if err != nil {
		t.Fatal(err)
	}
	millID := test.standingBuilding("Sawmill", map[string]core.BlueprintOutput{timber.Key: {DailyDelta: 2}})
	test.must(test.switchBuilding(millID, false))
	_, err = test.updateResource(timber.Key, nil, true)
	requireFieldMessage(t, err, "archived", "The Sawmill makes timber; edit its blueprint or demolish it first.")

	test.must(test.refund(false, millID, map[string]int64{}))
	if _, err := test.updateResource(timber.Key, nil, true); err != nil {
		t.Fatalf("archiving timber once the Sawmill is demolished: %v", err)
	}
}

func TestPlayersSeeAHiddenBuildingsCapButNotTheBuilding(t *testing.T) {
	test := newOutputTest(t)
	hidden := granary(false)
	hidden.Name = "Secret Cellar"
	hidden.Cost = map[string]int64{}
	hidden.Output = map[string]core.BlueprintOutput{"food": {StorageBonus: 50}}
	cellar := test.blueprint(hidden)
	buildingID := *test.must(test.startBuilding(test.dm, cellar.ID, map[string]int64{"BUILDER": 4})).Entries[0].BuildingID
	test.advance(10)
	board, err := test.service.GetColonyBoard(test.context, test.player)
	if err != nil {
		t.Fatal(err)
	}
	food := board.Resources[slices.IndexFunc(board.Resources, func(resource core.ColonyResource) bool { return resource.Key == "food" })]
	if food.MaxAmount == nil || *food.MaxAmount != 150 {
		t.Fatalf("players' food max = %v, want 150", food.MaxAmount)
	}
	if _, found := test.building(test.player, buildingID); found {
		t.Fatal("players see the hidden building")
	}
}

func TestBlueprintOutputChecks(t *testing.T) {
	test := newOutputTest(t)
	input := granary(true)
	input.Output = map[string]core.BlueprintOutput{"food": {}, "stone": {DailyDelta: 1}, "gold": {StorageBonus: -1}}
	_, err := test.service.CreateBlueprint(test.context, test.dm, input)
	operationError, typed := core.AsError(err)
	if !typed || operationError.Message != "Check the blueprint." || len(operationError.Fields) != 3 {
		t.Fatalf("bad output = %v", err)
	}
	wantMessages := []string{
		"Give Food a daily change or a storage bonus, or remove it.",
		"A storage bonus for Gold is 0 or more, up to 2,147,483,647.",
		`There is no resource "stone".`,
	}
	for _, field := range operationError.Fields {
		if field.Field != "output" || !slices.Contains(wantMessages, field.Message) {
			t.Fatalf("field error %+v", field)
		}
	}

	input.Output = map[string]core.BlueprintOutput{"food": {DailyDelta: 6}, "materials": {DailyDelta: -1, StorageBonus: 50}}
	blueprint := test.blueprint(input)
	if blueprint.Output["food"] != (core.BlueprintOutput{DailyDelta: 6}) || blueprint.Output["materials"] != (core.BlueprintOutput{DailyDelta: -1, StorageBonus: 50}) {
		t.Fatalf("saved output = %+v", blueprint.Output)
	}
	updated, err := test.service.UpdateBlueprint(test.context, test.dm, blueprint.ID, input.BlueprintInput)
	if err != nil || len(updated.Output) != 2 {
		t.Fatalf("an edit keeping the output = %+v, %v", updated.Output, err)
	}
	input.Output = nil
	cleared, err := test.service.UpdateBlueprint(test.context, test.dm, blueprint.ID, input.BlueprintInput)
	if err != nil || cleared.Output == nil || len(cleared.Output) != 0 {
		t.Fatalf("an edit leaving output out clears it: %+v, %v", cleared.Output, err)
	}
	if page := test.construction(test.player); len(page.Blueprints) != 1 || page.Blueprints[0].Output == nil {
		t.Fatalf("players' blueprints = %+v", page.Blueprints)
	}
}

func pointerTo[Value any](value Value) *Value {
	return &value
}

func TestDemolishCapsItsRefundWithoutItsOwnBonus(t *testing.T) {
	test := newOutputTest(t)
	granaryID := test.standingBuilding("Granary", map[string]core.BlueprintOutput{"food": {StorageBonus: 200}})
	test.setTo(map[string]int64{"food": 100})
	demolished := test.must(test.refund(false, granaryID, map[string]int64{"food": 150}))
	if test.amount("food") != 100 || len(demolished.Entries[0].Deltas.Resources) != 0 || *test.boardMax("food") != 100 {
		t.Fatalf("food %d, refund deltas %+v, max %d; want the refund capped at the own max of 100",
			test.amount("food"), demolished.Entries[0].Deltas, *test.boardMax("food"))
	}
}

func TestUndoRefusesStockAboveAMaxTheDmLoweredSince(t *testing.T) {
	test := newOutputTest(t)
	test.setTo(map[string]int64{"food": 90})
	advanced := test.advance(2)
	if _, err := test.updateResource("food", pointerTo(int64(80)), false); err != nil {
		t.Fatalf("lower food's max to 80 with 76 in store: %v", err)
	}
	_, err := test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: advanced.GroupID})
	requireMessage(t, err, core.KindConflict, "Undo would put Food back at 90, outside the bounds it has had since. Adjust it instead.")
	if _, err := test.updateResource("food", pointerTo(int64(95)), false); err != nil {
		t.Fatal(err)
	}
	test.undo(advanced)
	if test.amount("food") != 90 {
		t.Fatalf("food %d after undo, want 90", test.amount("food"))
	}
}

func TestStorageBonusIsZeroWithoutAMax(t *testing.T) {
	test := newOutputTest(t)
	test.standingBuilding("Warehouse", map[string]core.BlueprintOutput{"materials": {StorageBonus: 40}, "food": {StorageBonus: 10}})
	if materials := test.resourceSettings("materials"); materials.MaxAmount != nil || materials.StorageBonus != 0 {
		t.Fatalf("/settings materials = %+v; no max means no bonus", materials)
	}
	if food := test.resourceSettings("food"); food.StorageBonus != 10 {
		t.Fatalf("/settings food = %+v; want a bonus of 10", food)
	}
}

func (test *colonyTest) addBuilding(blueprintID uuid.UUID) (core.ColonyChange, error) {
	test.t.Helper()
	return test.service.AddBuilding(test.context, test.dm, blueprintID, core.AddBuildingInput{ExpectedVersion: test.board().Version})
}

func TestAddingABuildingIsInstantFreeAndUndoable(t *testing.T) {
	test := newOutputTest(t)
	input := granary(true)
	input.Output = map[string]core.BlueprintOutput{"food": {StorageBonus: 200, DailyDelta: 10}}
	blueprint := test.blueprint(input)
	test.correctDay(12)

	_, err := test.service.AddBuilding(test.context, test.player, blueprint.ID, core.AddBuildingInput{ExpectedVersion: test.board().Version})
	if !isKind(err, core.KindForbidden) {
		t.Fatalf("a player adding a building: %v", err)
	}
	added := test.must(test.addBuilding(blueprint.ID))
	entry := added.Entries[0]
	if len(added.Entries) != 1 || entry.Kind != core.TimelineKindBuildingAdded || entry.Title != "Granary added by the DM" ||
		entry.Dm.Visibility != core.TimelineVisibilityPublic || entry.BuildingID == nil {
		t.Fatalf("add entries = %+v", added.Entries)
	}
	buildingID := *entry.BuildingID
	building, found := test.building(test.player, buildingID)
	if !found || building.Status != core.BuildingStatusFinished || !building.Active || !building.Added || building.StartedDay != 12 || building.FinishedDay == nil ||
		*building.FinishedDay != 12 || len(building.Cost) != 0 || len(building.CompletionEffects.Resources) != 0 ||
		building.Labor[0].Progress != 40 || building.Labor[0].Crew != 0 || building.ProjectedFinishDay != nil {
		t.Fatalf("the added building = %+v", building)
	}
	if test.amount("materials") != 50 || test.amount("gold") != 40 || test.amount("morale") != 80 || *test.boardMax("food") != 300 {
		t.Fatalf("adding took or gave something: materials %d, gold %d, morale %d, food max %d",
			test.amount("materials"), test.amount("gold"), test.amount("morale"), *test.boardMax("food"))
	}
	if !slices.ContainsFunc(test.timeline(test.player, 0, 30), func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingAdded }) {
		t.Fatal("players don't see a revealed blueprint's added building")
	}
	// Output runs from the next day: 7 colonists eat 7 beside the Granary's 10.
	test.advance(1)
	if test.amount("food") != 103 {
		t.Fatalf("food %d after a day, want 103", test.amount("food"))
	}
	second := test.must(test.addBuilding(blueprint.ID))
	if *test.boardMax("food") != 500 {
		t.Fatalf("two Granaries cap food at %d, want 500", *test.boardMax("food"))
	}
	undone := test.undo(second)
	if undone.Label != "The DM rewound a change to the colony" || undone.Entries[0].BuildingID != nil || undone.Entries[0].Dm.Visibility != core.TimelineVisibilityPublic {
		t.Fatalf("undoing a revealed blueprint's add = %+v; want a PUBLIC generic UNDO", undone.Entries[0])
	}
	if _, found := test.building(test.dm, *second.Entries[0].BuildingID); found || *test.boardMax("food") != 300 {
		t.Fatalf("after undo: building found %v, food max %d", found, *test.boardMax("food"))
	}
}

func TestAddingABuildingOfAHiddenOrArchivedBlueprint(t *testing.T) {
	test := newOutputTest(t)
	hidden := granary(false)
	hidden.Name = "Warded Tower"
	tower := test.blueprint(hidden)
	added := test.must(test.addBuilding(tower.ID))
	buildingID := *added.Entries[0].BuildingID
	if _, found := test.building(test.player, buildingID); found {
		t.Fatal("players see a hidden blueprint's added building")
	}
	if slices.ContainsFunc(test.timeline(test.player, 0, 30), func(entry core.TimelineEntry) bool { return entry.Kind == core.TimelineKindBuildingAdded }) {
		t.Fatal("players see a hidden blueprint's BUILDING_ADDED entry")
	}
	entry := added.Entries[0]
	renamed := hidden.BlueprintInput
	renamed.Name = "Lookout"
	if _, err := test.service.UpdateBlueprint(test.context, test.dm, tower.ID, renamed); err != nil {
		t.Fatal(err)
	}
	entries := test.timeline(test.dm, 0, 30)
	if index := slices.IndexFunc(entries, func(row core.TimelineEntry) bool { return row.ID == entry.ID }); entries[index].Dm.Visibility != core.TimelineVisibilityDm {
		t.Fatalf("a hidden blueprint's rename leaves its added entry %+v", entries[index])
	}
	_, err := test.service.UpdateTimelineEntry(test.context, test.dm, entry.ID, core.TimelineEntryInput{Day: entry.Day, Title: entry.Title, Visibility: core.TimelineVisibilityPublic})
	requireMessage(t, err, core.KindInvalid, "The Lookout blueprint is hidden from players. Reveal it first.")

	if _, err := test.service.SetBlueprintArchived(test.context, test.dm, tower.ID, true); err != nil {
		t.Fatal(err)
	}
	_, err = test.addBuilding(tower.ID)
	requireMessage(t, err, core.KindConflict, "The Lookout blueprint is archived.")
	_, err = test.addBuilding(uuid.New())
	requireMessage(t, err, core.KindNotFound, "That blueprint doesn't exist.")
}

func TestUndoingAHiddenAddLeavesPlayersNothing(t *testing.T) {
	test := newOutputTest(t)
	hidden := granary(false)
	hidden.Name = "Warded Tower"
	tower := test.blueprint(hidden)
	added := test.must(test.addBuilding(tower.ID))
	undone := test.undo(added)
	if undone.Entries[0].Dm.Visibility != core.TimelineVisibilityDm || undone.Entries[0].Title != "The DM rewound a change to the colony" {
		t.Fatalf("undoing a hidden add = %+v; want a DM-only generic UNDO", undone.Entries[0])
	}
	for _, entry := range test.timeline(test.player, 0, 30) {
		if entry.Kind == core.TimelineKindUndo || strings.Contains(entry.Title, "Warded Tower") {
			t.Fatalf("players see %+v", entry)
		}
	}
	entries := test.timeline(test.dm, 0, 30)
	if index := slices.IndexFunc(entries, func(entry core.TimelineEntry) bool { return entry.ID == added.Entries[0].ID }); index < 0 ||
		entries[index].Dm.Visibility != core.TimelineVisibilityDm || entries[index].BuildingID != nil {
		t.Fatalf("the undone BUILDING_ADDED entry = %+v; want it DM-only once its link is gone", entries)
	}
}
