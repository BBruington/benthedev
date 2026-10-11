package core_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

// colonyTest is a fresh colony with the DM and one active player. The rules themselves are tested
// in internal/game (spec §5.12); these tests cover the write path around them: the version check,
// the timeline, undo and the projections.
type colonyTest struct {
	t        *testing.T
	context  context.Context
	service  *core.Service
	database *dbtest.Database
	dm       core.Viewer
	player   core.Viewer
}

func newColonyTest(t *testing.T) *colonyTest {
	t.Helper()
	service, database, _, _ := newService(t)
	testContext := context.Background()
	dmID, err := service.SignIn(testContext, core.DiscordIdentity{UserID: testDmDiscordID, Username: "dm"})
	if err != nil {
		t.Fatalf("sign in the DM: %v", err)
	}
	player, err := store.New(database.Pool).CreateMember(testContext, store.CreateMemberParams{
		DiscordUserID:   "seed:ana",
		DiscordUsername: "ana",
		DisplayName:     "Ana",
		Status:          store.MemberStatusACTIVE,
	})
	if err != nil {
		t.Fatalf("create a player: %v", err)
	}
	return &colonyTest{
		t:        t,
		context:  testContext,
		service:  service,
		database: database,
		dm:       service.NewViewer(core.ViewerIdentity{MemberID: dmID, DiscordUserID: testDmDiscordID}),
		player:   service.NewViewer(core.ViewerIdentity{MemberID: player.ID, DiscordUserID: player.DiscordUserID, Status: player.Status}),
	}
}

func (test *colonyTest) board() core.ColonyBoard {
	test.t.Helper()
	board, err := test.service.GetColonyBoard(test.context, test.dm)
	if err != nil {
		test.t.Fatalf("read the board: %v", err)
	}
	return board
}

func (test *colonyTest) amount(key string) int64 {
	test.t.Helper()
	for _, resource := range test.board().Resources {
		if resource.Key == key {
			return resource.Amount
		}
	}
	test.t.Fatalf("no resource %q on the board", key)
	return 0
}

// setHeadcounts sets role headcounts directly: there is no workforce operation until M2.
func (test *colonyTest) setHeadcounts(counts map[string]int32) {
	test.t.Helper()
	for key, count := range counts {
		if err := store.New(test.database.Pool).SetRoleUnnamedCount(test.context, store.SetRoleUnnamedCountParams{Key: key, UnnamedCount: count}); err != nil {
			test.t.Fatalf("set %s headcount: %v", key, err)
		}
	}
}

func (test *colonyTest) must(change core.ColonyChange, err error) core.ColonyChange {
	test.t.Helper()
	if err != nil {
		test.t.Fatalf("colony change: %v", err)
	}
	return change
}

func (test *colonyTest) advance(days int64) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.Advance(test.context, test.dm, core.AdvanceInput{Days: days, ExpectedVersion: test.board().Version}))
}

func (test *colonyTest) setTo(amounts map[string]int64) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.AdjustStock(test.context, test.dm, core.AdjustInput{
		ExpectedVersion: test.board().Version,
		Source:          core.AdjustSourceAdjustment,
		Deltas:          map[string]int64{},
		SetTo:           amounts,
	}))
}

func (test *colonyTest) schedule(intervalDays int64, nextDay *int64) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.SetShipmentSchedule(test.context, test.dm, core.ShipmentScheduleInput{
		ExpectedVersion: test.board().Version,
		IntervalDays:    intervalDays,
		NextDay:         nextDay,
	}))
}

func (test *colonyTest) correctDay(day int64) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.CorrectDay(test.context, test.dm, core.CorrectDayInput{ExpectedVersion: test.board().Version, Day: day}))
}

func (test *colonyTest) timeline(viewer core.Viewer, fromDay int64, toDay int64) []core.TimelineEntry {
	test.t.Helper()
	page, err := test.service.ListTimeline(test.context, viewer, core.TimelineRangeInput{FromDay: fromDay, ToDay: toDay})
	if err != nil {
		test.t.Fatalf("list the timeline: %v", err)
	}
	return page.Entries
}

func kinds(entries []core.TimelineEntry) []core.TimelineKind {
	result := make([]core.TimelineKind, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Kind)
	}
	return result
}

func pointer(value int64) *int64 {
	return &value
}

// Example E: an identical second click, or a second tab, gets 409 and changes nothing.
func TestExampleEDoubleSubmitChangesTheColonyOnce(t *testing.T) {
	test := newColonyTest(t)
	test.correctDay(10)
	version := test.board().Version

	results := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	for range 2 {
		go func() {
			start.Wait()
			_, err := test.service.Advance(test.context, test.dm, core.AdvanceInput{Days: 10, ExpectedVersion: version})
			results <- err
		}()
	}
	start.Done()
	var succeeded, conflicted int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case isKind(err, core.KindConflict):
			conflicted++
			if want := "The colony changed (now Day 20). Review and retry."; err.Error() != want {
				t.Errorf("conflict message = %q, want %q", err.Error(), want)
			}
		default:
			t.Fatalf("advance: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded %d, conflicted %d; want exactly one of each", succeeded, conflicted)
	}
	board := test.board()
	if board.Day != 20 || board.Version != version+1 {
		t.Fatalf("board day %d version %d; want day 20, version %d", board.Day, board.Version, version+1)
	}
	if advanced := slices.Collect(func(yield func(core.TimelineKind) bool) {
		for _, kind := range kinds(test.timeline(test.dm, 0, 30)) {
			if kind == core.TimelineKindDaysAdvanced && !yield(kind) {
				return
			}
		}
	}); len(advanced) != 1 {
		t.Fatalf("%d DAYS_ADVANCED entries; want 1", len(advanced))
	}
}

// Examples A and I: exact cadence, then undo restores and is refused once something newer exists.
func TestExamplesAAndIUndoRestoresAndOnlyTheNewestChange(t *testing.T) {
	test := newColonyTest(t)
	test.setHeadcounts(map[string]int32{"FARMER": 4, "BUILDER": 3, "SOLDIER": 2, "UNASSIGNED": 3})
	test.setTo(map[string]int64{"food": 100})
	test.schedule(7, nil)
	if shipments := test.board().Shipments; shipments == nil || shipments.NextDay != 7 || shipments.DaysUntil != 7 {
		t.Fatalf("interval 7 on day 0: shipments = %+v; want next Day 7, in 7", shipments)
	}
	test.advance(6)
	if shipments := test.board().Shipments; shipments.NextDay != 7 || shipments.DaysUntil != 1 {
		t.Fatalf("after 6 days: shipments = %+v; want next Day 7, in 1", shipments)
	}
	foodOnDay6 := test.amount("food")
	change := test.advance(1)
	if got := kinds(change.Entries); !slices.Equal(got, []core.TimelineKind{core.TimelineKindShipmentArrived, core.TimelineKindDaysAdvanced}) {
		t.Fatalf("advance 1 wrote %v", got)
	}
	if arrival := change.Entries[0]; arrival.Day != 7 || arrival.Arrivals == nil || arrival.Arrivals.First != 7 || arrival.Arrivals.Count != 1 {
		t.Fatalf("arrival entry = %+v; want Day 7", arrival)
	}
	board := test.board()
	if board.Day != 7 || board.Shipments.NextDay != 14 || board.Dm.LastChange == nil || board.Dm.LastChange.GroupID != change.GroupID {
		t.Fatalf("after the arrival: day %d, shipments %+v, last change %+v", board.Day, board.Shipments, board.Dm.LastChange)
	}

	undo := test.must(test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: change.GroupID}))
	board = test.board()
	if board.Day != 6 || board.Shipments.NextDay != 7 || test.amount("food") != foodOnDay6 || board.Dm.LastChange != nil {
		t.Fatalf("after undo: day %d, shipments %+v, food %d (want %d), last change %+v", board.Day, board.Shipments, test.amount("food"), foodOnDay6, board.Dm.LastChange)
	}
	if len(undo.Entries) != 1 || undo.Entries[0].Kind != core.TimelineKindUndo || undo.Entries[0].Dm.Visibility != core.TimelineVisibilityPublic {
		t.Fatalf("undo wrote %+v; want one PUBLIC UNDO entry", undo.Entries)
	}
	if want := "The DM rewound: 1 day passes · Day 7"; undo.Entries[0].Title != want {
		t.Errorf("UNDO title = %q, want %q", undo.Entries[0].Title, want)
	}
	for _, entry := range test.timeline(test.dm, 0, 10) {
		if entry.GroupID != nil && *entry.GroupID == change.GroupID && entry.Dm.UndoneAt == nil {
			t.Errorf("%s entry of the undone change is not marked undone", entry.Kind)
		}
	}
	if _, err := test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: change.GroupID}); !isKind(err, core.KindConflict) {
		t.Fatalf("a second undo = %v; want a conflict (one level only)", err)
	}

	advanced := test.advance(1)
	newer := test.setTo(map[string]int64{"gold": 25})
	if _, err := test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: advanced.GroupID}); !isKind(err, core.KindConflict) {
		t.Fatalf("undoing the advance after a newer change = %v; want a conflict", err)
	}
	if lastChange := test.board().Dm.LastChange; lastChange == nil || lastChange.GroupID != newer.GroupID {
		t.Fatalf("undo offers %+v; want only the newer adjustment", lastChange)
	}
	test.must(test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: newer.GroupID}))
	if gold := test.amount("gold"); gold != 0 {
		t.Fatalf("gold after undoing the adjustment = %d; want 0", gold)
	}
	// The adjustment was DM-only, so its UNDO is too.
	recent, err := test.service.RecentTimeline(test.context, test.dm)
	if err != nil {
		t.Fatalf("recent timeline: %v", err)
	}
	if newest := recent.Entries[0]; newest.Kind != core.TimelineKindUndo || newest.Dm.Visibility != core.TimelineVisibilityDm {
		t.Fatalf("newest entry = %+v; want a DM-only UNDO", newest)
	}
}

// Examples B and B2: the food floor, cargo recorded on its arrival, and the entries both write.
func TestExampleB2CargoRecordedOnItsArrival(t *testing.T) {
	test := newColonyTest(t)
	test.setHeadcounts(map[string]int32{"FARMER": 4, "BUILDER": 3, "SOLDIER": 2, "UNASSIGNED": 3})
	test.setTo(map[string]int64{"food": 100})
	test.schedule(7, nil)

	week := test.advance(7)
	if food := test.amount("food"); food != 16 {
		t.Fatalf("food on Day 7 = %d; want 16", food)
	}
	arrival := week.Entries[0]
	if arrival.Kind != core.TimelineKindShipmentArrived || !arrival.Dm.CanRecordCargo {
		t.Fatalf("first entry = %+v; want a SHIPMENT_ARRIVED entry that offers Record cargo", arrival)
	}
	if body := week.Entries[1].Body; body != "Food −84 · Shipment arrived Day 7" {
		t.Errorf("DAYS_ADVANCED body = %q", body)
	}
	cargo := test.must(test.service.AdjustStock(test.context, test.dm, core.AdjustInput{
		ExpectedVersion: test.board().Version,
		Source:          core.AdjustSourceShipment,
		ArrivalEntryID:  &arrival.ID,
		Deltas:          map[string]int64{"food": 30},
		SetTo:           map[string]int64{},
		Colonists:       4,
		Public:          true,
	}))
	board := test.board()
	if board.Colonists != 16 || board.Unassigned != 7 || test.amount("food") != 46 {
		t.Fatalf("after cargo: colonists %d, unassigned %d, food %d; want 16, 7, 46", board.Colonists, board.Unassigned, test.amount("food"))
	}
	recorded := cargo.Entries[0]
	if recorded.Kind != core.TimelineKindCargoRecorded || recorded.ArrivalEntryID == nil || *recorded.ArrivalEntryID != arrival.ID ||
		recorded.Day != 7 || recorded.Dm.Visibility != core.TimelineVisibilityPublic {
		t.Fatalf("cargo entry = %+v; want a public CARGO_RECORDED on Day 7 linked to the arrival", recorded)
	}
	if recorded.Deltas.Resources["food"] != 30 || recorded.Deltas.Colonists != 4 {
		t.Fatalf("cargo deltas = %+v", recorded.Deltas)
	}

	test.advance(3)
	board = test.board()
	if board.Day != 10 || test.amount("food") != 0 || board.Shipments.NextDay != 14 {
		t.Fatalf("Day %d, food %d, next shipment %d; want Day 10, food 0, next 14", board.Day, test.amount("food"), board.Shipments.NextDay)
	}
	var advancedDeltas []int64
	for _, entry := range test.timeline(test.player, 0, 13) {
		if entry.Kind == core.TimelineKindDaysAdvanced {
			advancedDeltas = append(advancedDeltas, entry.Deltas.Resources["food"])
		}
	}
	if !slices.Equal(advancedDeltas, []int64{-84, -46}) {
		t.Fatalf("DAYS_ADVANCED food deltas = %v; want [-84 -46]", advancedDeltas)
	}
}

// Example F's colony part: CLAMP caps morale, the preview says so, and colonists land in Unassigned.
func TestExampleFRewardsClampAndColonistsLandInUnassigned(t *testing.T) {
	test := newColonyTest(t)
	test.setHeadcounts(map[string]int32{"FARMER": 10, "SOLDIER": 6})
	test.setTo(map[string]int64{"morale": 98})
	input := core.AdjustInput{
		Source:    core.AdjustSourceAdjustment,
		Deltas:    map[string]int64{"food": 20, "morale": 5},
		SetTo:     map[string]int64{},
		Colonists: 3,
		Reason:    "Clear the Wolf Den",
		Public:    true,
	}
	preview, err := test.service.PreviewAdjustStock(test.context, test.dm, input)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if want := "Food +20 · Morale 98 → 100 (capped) · 3 colonists (Unassigned)"; preview.Summary != want {
		t.Errorf("preview summary = %q, want %q", preview.Summary, want)
	}
	if preview.ColonistsBefore != 16 || preview.ColonistsAfter != 19 {
		t.Errorf("preview colonists %d → %d; want 16 → 19", preview.ColonistsBefore, preview.ColonistsAfter)
	}
	input.ExpectedVersion = preview.Version
	change := test.must(test.service.AdjustStock(test.context, test.dm, input))
	entry := change.Entries[0]
	if entry.Deltas.Resources["morale"] != 2 || entry.Deltas.Resources["food"] != 20 || entry.Deltas.Colonists != 3 {
		t.Fatalf("logged deltas = %+v; want only the applied ones", entry.Deltas)
	}
	if entry.Body != "Food +20 · Morale +2 · 3 colonists (Unassigned)\nClear the Wolf Den" {
		t.Errorf("entry body = %q", entry.Body)
	}
	board := test.board()
	if test.amount("morale") != 100 || board.Unassigned != 3 || board.Colonists != 19 {
		t.Fatalf("morale %d, unassigned %d, colonists %d; want 100, 3, 19", test.amount("morale"), board.Unassigned, board.Colonists)
	}
}

// Example H: a huge N writes exactly two entries, and undo puts everything back.
func TestExampleHHugeAdvanceWritesTwoEntries(t *testing.T) {
	test := newColonyTest(t)
	test.setHeadcounts(map[string]int32{"FARMER": 20})
	test.correctDay(40)
	test.schedule(7, pointer(42))
	test.setTo(map[string]int64{"food": 300})

	preview, err := test.service.PreviewAdvance(test.context, test.dm, core.AdvancePreviewInput{Days: 1_000_000})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if want := "Day 40 → 1,000,040 · Food 300 → 0 · 142,857 shipment arrivals (Days 42 – 1,000,034, every 7)"; preview.Summary != want {
		t.Errorf("preview = %q, want %q", preview.Summary, want)
	}
	change := test.must(test.service.Advance(test.context, test.dm, core.AdvanceInput{Days: 1_000_000, ExpectedVersion: preview.Version}))
	if got := kinds(change.Entries); !slices.Equal(got, []core.TimelineKind{core.TimelineKindShipmentArrived, core.TimelineKindDaysAdvanced}) {
		t.Fatalf("entries = %v; want one folded arrival and one DAYS_ADVANCED", got)
	}
	folded := change.Entries[0]
	if folded.Day != 1_000_034 || *folded.Arrivals != (core.ArrivalRun{First: 42, Every: 7, Count: 142_857, Last: 1_000_034}) {
		t.Fatalf("folded entry = day %d, %+v", folded.Day, folded.Arrivals)
	}
	if want := "142,857 shipments arrived, every 7 days, Days 42 – 1,000,034"; folded.Title != want {
		t.Errorf("folded title = %q, want %q", folded.Title, want)
	}
	if board := test.board(); board.Shipments.NextDay != 1_000_041 || board.Shipments.DaysUntil != 1 {
		t.Fatalf("shipments after = %+v; want next Day 1,000,041, in 1", board.Shipments)
	}
	test.must(test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: change.GroupID}))
	if board := test.board(); board.Day != 40 || board.Shipments.NextDay != 42 || test.amount("food") != 300 {
		t.Fatalf("after undo: day %d, shipments %+v, food %d", board.Day, board.Shipments, test.amount("food"))
	}
}

// Example D5 and clearing: Correct day skips shipment dates in a DM-only entry; clear hides the
// countdown, and later edits to food still work.
func TestCorrectDayAndClearSchedule(t *testing.T) {
	test := newColonyTest(t)
	test.correctDay(10)
	test.schedule(7, pointer(14))
	change := test.correctDay(30)
	entry := change.Entries[0]
	if entry.Title != "Day 10 → 30 (3 shipment dates skipped)" || entry.Dm.Visibility != core.TimelineVisibilityDm || entry.Day != 30 {
		t.Fatalf("DAY_CORRECTED = %+v", entry)
	}
	if next := test.board().Shipments.NextDay; next != 35 {
		t.Fatalf("next shipment = %d; want 35", next)
	}
	if _, err := test.service.CorrectDay(test.context, test.dm, core.CorrectDayInput{ExpectedVersion: test.board().Version, Day: 30}); !isKind(err, core.KindInvalid) {
		t.Fatalf("correcting to the same day = %v; want 422", err)
	}
	if _, err := test.service.SetShipmentSchedule(test.context, test.dm, core.ShipmentScheduleInput{ExpectedVersion: test.board().Version, IntervalDays: 7, NextDay: pointer(30)}); !hasField(err, "nextDay") {
		t.Fatalf("next arrival on today = %v; want 422 on nextDay", err)
	}
	if _, err := test.service.SetShipmentSchedule(test.context, test.dm, core.ShipmentScheduleInput{ExpectedVersion: test.board().Version, IntervalDays: 0}); !hasField(err, "intervalDays") {
		t.Fatalf("interval 0 = %v; want 422 on intervalDays", err)
	}
	test.must(test.service.ClearShipmentSchedule(test.context, test.dm, core.ClearShipmentScheduleInput{ExpectedVersion: test.board().Version}))
	if shipments := test.board().Shipments; shipments != nil {
		t.Fatalf("shipments after clear = %+v; want none", shipments)
	}
	if _, err := test.service.ClearShipmentSchedule(test.context, test.dm, core.ClearShipmentScheduleInput{ExpectedVersion: test.board().Version}); !isKind(err, core.KindConflict) {
		t.Fatalf("clearing twice = %v; want a conflict", err)
	}
	test.setTo(map[string]int64{"food": 12})
	if change := test.advance(3); len(change.Entries) != 1 {
		t.Fatalf("an advance with no schedule wrote %v; want no arrivals", kinds(change.Entries))
	}
}

func hasField(err error, field string) bool {
	operationError, typed := core.AsError(err)
	if !typed || operationError.Kind != core.KindInvalid {
		return false
	}
	return slices.ContainsFunc(operationError.Fields, func(fieldError core.FieldError) bool { return fieldError.Field == field })
}

func TestAdjustRefusesBadInput(t *testing.T) {
	test := newColonyTest(t)
	version := test.board().Version
	arrivalID := uuid.New()
	for name, testCase := range map[string]struct {
		input core.AdjustInput
		field string
	}{
		"a key in both":            {core.AdjustInput{Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"food": 1}, SetTo: map[string]int64{"food": 5}}, "setTo"},
		"an unknown key":           {core.AdjustInput{Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"timber": 1}}, "deltas"},
		"cargo with no arrival":    {core.AdjustInput{Source: core.AdjustSourceShipment, Deltas: map[string]int64{"food": 1}}, "arrivalEntryId"},
		"cargo on a missing one":   {core.AdjustInput{Source: core.AdjustSourceShipment, ArrivalEntryID: &arrivalID, Deltas: map[string]int64{"food": 1}}, "arrivalEntryId"},
		"an adjustment naming one": {core.AdjustInput{Source: core.AdjustSourceAdjustment, ArrivalEntryID: &arrivalID, Deltas: map[string]int64{"food": 1}}, "arrivalEntryId"},
		"negative colonists":       {core.AdjustInput{Source: core.AdjustSourceAdjustment, Colonists: -1}, "colonists"},
		"a two-line reason":        {core.AdjustInput{Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"food": 1}, Reason: "one\ntwo"}, "reason"},
		"too much gold":            {core.AdjustInput{Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"gold": 1 << 40}}, "deltas"},
	} {
		testCase.input.ExpectedVersion = version
		if _, err := test.service.AdjustStock(test.context, test.dm, testCase.input); !hasField(err, testCase.field) {
			t.Errorf("%s: %v; want 422 on %s", name, err, testCase.field)
		}
	}
	if _, err := test.service.AdjustStock(test.context, test.dm, core.AdjustInput{ExpectedVersion: version, Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"morale": 5}}); !isKind(err, core.KindInvalid) {
		t.Errorf("an adjustment that changes nothing (morale is at its maximum) = %v; want 422", err)
	}
	if _, err := test.service.AdjustStock(test.context, test.dm, core.AdjustInput{ExpectedVersion: version + 1, Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"food": 1}}); !isKind(err, core.KindConflict) {
		t.Errorf("a stale version = %v; want 409", err)
	}
	if _, err := test.service.Advance(test.context, test.player, core.AdvanceInput{Days: 1, ExpectedVersion: version}); !isKind(err, core.KindForbidden) {
		t.Errorf("a player advanced: %v", err)
	}
	if test.board().Version != version {
		t.Fatal("a refused change bumped the version")
	}
}

// Players never see DM or undone entries; neither does the DM viewing as player (spec §2.2).
func TestTimelineProjection(t *testing.T) {
	test := newColonyTest(t)
	test.setHeadcounts(map[string]int32{"FARMER": 2})
	test.setTo(map[string]int64{"food": 50}) // DM-only STOCK_ADJUSTED
	public := test.advance(2)
	undone := test.advance(1)
	test.must(test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: undone.GroupID}))

	dmAsPlayer := test.dm
	dmAsPlayer.ViewAsPlayer = true
	for name, viewer := range map[string]core.Viewer{"player": test.player, "DM viewing as player": dmAsPlayer} {
		entries := test.timeline(viewer, 0, 6)
		recent, err := test.service.RecentTimeline(test.context, viewer)
		if err != nil {
			t.Fatalf("%s: recent: %v", name, err)
		}
		for _, entry := range append(entries, recent.Entries...) {
			if entry.Kind == core.TimelineKindStockAdjusted || (entry.GroupID != nil && *entry.GroupID == undone.GroupID) {
				t.Errorf("%s sees a hidden entry: %+v", name, entry)
			}
		}
		if got := kinds(entries); !slices.Equal(got, []core.TimelineKind{core.TimelineKindDaysAdvanced, core.TimelineKindUndo}) {
			t.Errorf("%s sees %v; want the public advance and the public UNDO", name, got)
		}
		if entries[0].ID != public.Entries[0].ID {
			t.Errorf("%s: first entry is not the public advance", name)
		}
	}
	if got := kinds(test.timeline(test.dm, 0, 6)); len(got) != 4 {
		t.Errorf("the DM sees %v; want all four entries", got)
	}
}

// Auto-logged entries are editable, never deletable; CUSTOM ones move and delete (spec §5.8).
func TestTimelineEditingRules(t *testing.T) {
	test := newColonyTest(t)
	test.setTo(map[string]int64{"gold": 10})
	adjusted := test.timeline(test.dm, 0, 0)[0]
	if err := test.service.DeleteCustomEntry(test.context, test.dm, adjusted.ID); !isKind(err, core.KindConflict) {
		t.Fatalf("deleting an auto entry = %v; want 409", err)
	}
	if _, err := test.service.UpdateTimelineEntry(test.context, test.dm, adjusted.ID, core.TimelineEntryInput{Day: 3, Title: "Moved", Visibility: core.TimelineVisibilityDm}); !hasField(err, "day") {
		t.Fatalf("moving an auto entry = %v; want 422 on day", err)
	}
	edited, err := test.service.UpdateTimelineEntry(test.context, test.dm, adjusted.ID, core.TimelineEntryInput{Day: 0, Title: " Gold from the wreck ", Body: "Found\r\nin the hold", Visibility: core.TimelineVisibilityPublic})
	if err != nil {
		t.Fatalf("edit an auto entry: %v", err)
	}
	if edited.Title != "Gold from the wreck" || edited.Body != "Found\nin the hold" || edited.Dm.Visibility != core.TimelineVisibilityPublic || edited.Dm.EditedAt == nil {
		t.Fatalf("edited = %+v", edited)
	}
	if test.amount("gold") != 10 {
		t.Fatal("editing an entry changed the stock")
	}

	custom, err := test.service.CreateCustomEntry(test.context, test.dm, core.TimelineEntryInput{Day: 400, Title: "The eclipse", Visibility: core.TimelineVisibilityPublic})
	if err != nil {
		t.Fatalf("create a custom entry: %v", err)
	}
	if !custom.Dm.CanDelete || custom.GroupID != nil {
		t.Fatalf("custom entry = %+v", custom)
	}
	if _, err := test.service.UpdateTimelineEntry(test.context, test.dm, custom.ID, core.TimelineEntryInput{Day: 401, Title: "The eclipse", Visibility: core.TimelineVisibilityDm}); err != nil {
		t.Fatalf("move a custom entry: %v", err)
	}
	if _, err := test.service.CreateCustomEntry(test.context, test.dm, core.TimelineEntryInput{Day: 1, Title: "Nul\x00", Visibility: core.TimelineVisibilityDm}); !hasField(err, "title") {
		t.Fatalf("a NUL title = %v; want 422", err)
	}
	if _, err := test.service.CreateCustomEntry(test.context, test.dm, core.TimelineEntryInput{Day: 1, Title: "Ok", Body: strings.Repeat("a", 2001), Visibility: core.TimelineVisibilityDm}); !hasField(err, "body") {
		t.Fatalf("a long body = %v; want 422", err)
	}
	if err := test.service.DeleteCustomEntry(test.context, test.dm, custom.ID); err != nil {
		t.Fatalf("delete a custom entry: %v", err)
	}
	if err := test.service.DeleteCustomEntry(test.context, test.dm, custom.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("deleting it again = %v; want 404", err)
	}
}

func TestResourceSettings(t *testing.T) {
	test := newColonyTest(t)
	created, err := test.service.CreateResource(test.context, test.dm, core.NewResourceInput{Label: " Dragon Scales! ", Icon: core.ColonyIconGem})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Key != "dragon_scales" || created.Label != "Dragon Scales!" || created.Amount != 0 {
		t.Fatalf("created = %+v", created)
	}
	if _, err := test.service.CreateResource(test.context, test.dm, core.NewResourceInput{Label: "dragon scales!", Icon: core.ColonyIconGem}); !hasField(err, "label") {
		t.Fatalf("a duplicate label = %v; want 422", err)
	}
	ale, err := test.service.CreateResource(test.context, test.dm, core.NewResourceInput{Label: "Dragon-Scales", Icon: core.ColonyIconBeer, MinAmount: 5, MaxAmount: pointer(50)})
	if err != nil {
		t.Fatalf("create a second: %v", err)
	}
	if ale.Key != "dragon_scales_2" || ale.Amount != 5 {
		t.Fatalf("second = %+v; want key dragon_scales_2 starting at its minimum", ale)
	}
	if _, err := test.service.CreateResource(test.context, test.dm, core.NewResourceInput{Label: "Rope", Icon: core.ColonyIconGem, MinAmount: 5, MaxAmount: pointer(1)}); !hasField(err, "maxAmount") {
		t.Fatalf("a maximum below the minimum = %v; want 422", err)
	}

	test.setTo(map[string]int64{"food": 50})
	food := core.ResourceInput{Label: "Rations", Icon: core.ColonyIconWheat, MinAmount: 60, SortOrder: 20}
	if _, err := test.service.UpdateResource(test.context, test.dm, "food", food); !hasField(err, "minAmount") {
		t.Fatalf("bounds that exclude the amount = %v; want 422 on minAmount", err)
	}
	food.MinAmount = 0
	food.Archived = true
	if _, err := test.service.UpdateResource(test.context, test.dm, "food", food); !hasField(err, "archived") {
		t.Fatalf("archiving food = %v; want 422", err)
	}
	food.Archived = false
	renamed, err := test.service.UpdateResource(test.context, test.dm, "food", food)
	if err != nil || renamed.Label != "Rations" || renamed.Key != "food" {
		t.Fatalf("rename food: %+v, %v", renamed, err)
	}
	if _, err := test.service.UpdateResource(test.context, test.dm, "timber", food); !isKind(err, core.KindNotFound) {
		t.Fatalf("an unknown key = %v; want 404", err)
	}

	// A bounds change since the last change makes its undo impossible: 409, not a 500.
	change := test.setTo(map[string]int64{"gold": 80})
	if _, err := test.service.UpdateResource(test.context, test.dm, "gold", core.ResourceInput{Label: "Gold", Icon: core.ColonyIconCoins, MinAmount: 40, SortOrder: 10}); err != nil {
		t.Fatalf("raise gold's minimum: %v", err)
	}
	if _, err := test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: change.GroupID}); !isKind(err, core.KindConflict) {
		t.Fatalf("undo below the new minimum = %v; want 409", err)
	}

	archived := core.ResourceInput{Label: "Dragon-Scales", Icon: core.ColonyIconBeer, MinAmount: 5, MaxAmount: pointer(50), SortOrder: 60, Archived: true}
	if _, err := test.service.UpdateResource(test.context, test.dm, "dragon_scales_2", archived); err != nil {
		t.Fatalf("archive: %v", err)
	}
	for _, resource := range test.board().Resources {
		if resource.Key == "dragon_scales_2" {
			t.Fatal("the board shows an archived resource")
		}
	}
	if _, err := test.service.AdjustStock(test.context, test.dm, core.AdjustInput{ExpectedVersion: test.board().Version, Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"dragon_scales_2": 1}}); !hasField(err, "deltas") {
		t.Fatalf("adjusting an archived resource = %v; want 422", err)
	}
}
