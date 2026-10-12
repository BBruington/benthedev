package game

import (
	"errors"
	"reflect"
	"testing"
)

// The worked examples of docs/SPEC.md §5.12, verbatim. Parts that live in internal/core (versions,
// timeline rows, request and quest statuses, the undo slot) are noted where they occur and are tested
// there; these tests cover the colony numbers each example states.

func pointer[Value any](value Value) *Value {
	return &value
}

// colony builds a State with the seeded resources (gold, food, materials, morale capped at 100).
func colony(day int64, food int64, roles map[string]Role, schedule *Schedule) State {
	return State{
		Day:      day,
		Schedule: schedule,
		Resources: map[string]Resource{
			"gold":      {Label: "Gold", Amount: 50},
			"food":      {Label: "Food", Amount: food},
			"materials": {Label: "Materials", Amount: 30},
			"morale":    {Label: "Morale", Amount: 100, MaxAmount: pointer(int64(100))},
		},
		Roles: roles,
	}
}

// exampleBRoles is Example B's workforce: Farmer 3 unnamed + Brannoc (named) = 4, Builder 3,
// Soldier 2, Unassigned 3, so 12 colonists.
func exampleBRoles() map[string]Role {
	return map[string]Role{
		"FARMER":          {UnnamedCount: 3, NamedCount: 1},
		"BUILDER":         {UnnamedCount: 3},
		"SOLDIER":         {UnnamedCount: 2},
		UnassignedRoleKey: {UnnamedCount: 3},
	}
}

func unassignedOnly(count int64) map[string]Role {
	return map[string]Role{UnassignedRoleKey: {UnnamedCount: count}}
}

func mustAdvance(t *testing.T, state State, days int64) AdvanceResult {
	t.Helper()
	result, err := Advance(state, days)
	if err != nil {
		t.Fatalf("Advance(%d) from day %d: %v", days, state.Day, err)
	}
	return result
}

// unfoldedArrivalDays lists the days of the SHIPMENT_ARRIVED entries an advance writes one by one.
func unfoldedArrivalDays(t *testing.T, result AdvanceResult) []int64 {
	t.Helper()
	days := []int64{}
	for _, entry := range result.ArrivalEntries {
		if entry.Folded {
			t.Fatalf("unexpected folded entry %+v", entry)
		}
		days = append(days, entry.Day)
	}
	return days
}

func countdown(t *testing.T, state State) int64 {
	t.Helper()
	if state.Schedule == nil {
		t.Fatal("expected a shipment schedule")
	}
	return state.Schedule.NextDay - state.Day
}

func food(state State) int64 {
	return state.Resources[FoodResourceKey].Amount
}

func assertInvalid(t *testing.T, err error, field string, message string) {
	t.Helper()
	var invalidError *InvalidError
	if !errors.As(err, &invalidError) {
		t.Fatalf("want InvalidError %q, got %v", message, err)
	}
	if invalidError.Field != field || invalidError.Message != message {
		t.Fatalf("want %s: %q, got %s: %q", field, message, invalidError.Field, invalidError.Message)
	}
}

func TestExampleAExactCadence(t *testing.T) {
	scheduled, err := SetSchedule(colony(0, 100, unassignedOnly(2), nil), ScheduleInput{IntervalDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	state := scheduled.State
	if state.Day != 0 || state.Schedule.NextDay != 7 || countdown(t, state) != 7 {
		t.Fatalf("set interval 7 on day 0: day %d, D %d, countdown %d", state.Day, state.Schedule.NextDay, countdown(t, state))
	}
	for _, row := range []struct {
		advance       int64
		dayAfter      int64
		nextDay       int64
		countdown     int64
		arrivalLogged []int64
	}{
		{advance: 6, dayAfter: 6, nextDay: 7, countdown: 1, arrivalLogged: []int64{}},
		{advance: 1, dayAfter: 7, nextDay: 14, countdown: 7, arrivalLogged: []int64{7}},
		{advance: 7, dayAfter: 14, nextDay: 21, countdown: 7, arrivalLogged: []int64{14}},
	} {
		result := mustAdvance(t, state, row.advance)
		state = result.After
		if state.Day != row.dayAfter || state.Schedule.NextDay != row.nextDay || countdown(t, state) != row.countdown {
			t.Fatalf("advance %d: day %d, D %d, countdown %d; want %d, %d, %d", row.advance,
				state.Day, state.Schedule.NextDay, countdown(t, state), row.dayAfter, row.nextDay, row.countdown)
		}
		if arrivals := unfoldedArrivalDays(t, result); !reflect.DeepEqual(arrivals, row.arrivalLogged) {
			t.Fatalf("advance %d: arrivals %v, want %v", row.advance, arrivals, row.arrivalLogged)
		}
	}
}

func TestExampleBAdvanceTenWithTheFoodFloor(t *testing.T) {
	start := colony(0, 100, exampleBRoles(), &Schedule{IntervalDays: 7, NextDay: 7})
	if colonists := Colonists(start.Roles); colonists != 12 {
		t.Fatalf("colonists %d, want 12", colonists)
	}

	result := mustAdvance(t, start, 10)
	end := result.After
	if end.Day != 10 || food(end) != 0 || Colonists(end.Roles) != 12 || end.Schedule.NextDay != 14 || countdown(t, end) != 4 {
		t.Fatalf("end: day %d, food %d, colonists %d, D %d", end.Day, food(end), Colonists(end.Roles), end.Schedule.NextDay)
	}
	if arrivals := unfoldedArrivalDays(t, result); !reflect.DeepEqual(arrivals, []int64{7}) {
		t.Fatalf("SHIPMENT_ARRIVED days %v, want [7]", arrivals)
	}
	if want := (Effects{Resources: map[string]int64{"food": -100}}); !reflect.DeepEqual(result.Applied, want) {
		t.Fatalf("DAYS_ADVANCED deltas %+v, want %+v", result.Applied, want)
	}

	// The day-by-day table: one advance per day gives the same food and the arrival on Day 7.
	wantFood := []int64{88, 76, 64, 52, 40, 28, 16, 4, 0, 0}
	state := start
	for day, want := range wantFood {
		daily := mustAdvance(t, state, 1)
		state = daily.After
		if food(state) != want {
			t.Fatalf("day %d: food %d, want %d", day+1, food(state), want)
		}
		wantArrivals := []int64{}
		if state.Day == 7 {
			wantArrivals = []int64{7}
		}
		if arrivals := unfoldedArrivalDays(t, daily); !reflect.DeepEqual(arrivals, wantArrivals) {
			t.Fatalf("day %d: arrivals %v, want %v", state.Day, arrivals, wantArrivals)
		}
	}
	if !reflect.DeepEqual(state, end) {
		t.Fatalf("ten single days %+v differ from one advance of ten %+v", state, end)
	}
}

func TestExampleB2RecordingCargoOnArrival(t *testing.T) {
	start := colony(0, 100, exampleBRoles(), &Schedule{IntervalDays: 7, NextDay: 7})

	week := mustAdvance(t, start, 7)
	if week.After.Day != 7 || food(week.After) != 16 {
		t.Fatalf("advance 7: day %d, food %d; want day 7, food 16", week.After.Day, food(week.After))
	}
	if arrivals := unfoldedArrivalDays(t, week); !reflect.DeepEqual(arrivals, []int64{7}) {
		t.Fatalf("SHIPMENT_ARRIVED days %v, want [7]", arrivals)
	}
	if want := (Effects{Resources: map[string]int64{"food": -84}}); !reflect.DeepEqual(week.Applied, want) {
		t.Fatalf("DAYS_ADVANCED on Day 7 deltas %+v, want %+v", week.Applied, want)
	}

	cargo, err := ApplyEffects(week.After, Effects{Resources: map[string]int64{"food": 30}, Colonists: 4}, Clamp)
	if err != nil {
		t.Fatal(err)
	}
	afterCargo := cargo.After
	if food(afterCargo) != 46 || afterCargo.Roles[UnassignedRoleKey].UnnamedCount != 7 || Colonists(afterCargo.Roles) != 16 {
		t.Fatalf("after cargo: food %d, Unassigned %d, colonists %d; want 46, 7, 16",
			food(afterCargo), afterCargo.Roles[UnassignedRoleKey].UnnamedCount, Colonists(afterCargo.Roles))
	}
	if want := (Effects{Resources: map[string]int64{"food": 30}, Colonists: 4}); !reflect.DeepEqual(cargo.Applied, want) {
		t.Fatalf("CARGO_RECORDED deltas %+v, want %+v", cargo.Applied, want)
	}

	wantFood := []int64{30, 14, 0}
	state := afterCargo
	for _, want := range wantFood {
		state = mustAdvance(t, state, 1).After
		if food(state) != want {
			t.Fatalf("day %d: food %d, want %d", state.Day, food(state), want)
		}
	}

	rest := mustAdvance(t, afterCargo, 3)
	if rest.After.Day != 10 || food(rest.After) != 0 || rest.After.Schedule.NextDay != 14 || len(rest.ArrivalEntries) != 0 {
		t.Fatalf("advance 3: day %d, food %d, D %d, arrivals %v", rest.After.Day, food(rest.After), rest.After.Schedule.NextDay, rest.ArrivalEntries)
	}
	if want := (Effects{Resources: map[string]int64{"food": -46}}); !reflect.DeepEqual(rest.Applied, want) {
		t.Fatalf("DAYS_ADVANCED on Day 10 deltas %+v, want %+v", rest.Applied, want)
	}
	if !reflect.DeepEqual(rest.After, state) {
		t.Fatalf("advance 3 %+v differs from three single days %+v", rest.After, state)
	}
}

func TestExampleCOneAdvanceCrossesSeveralShipments(t *testing.T) {
	result := mustAdvance(t, colony(3, 500, unassignedOnly(10), &Schedule{IntervalDays: 7, NextDay: 7}), 30)
	if arrivals := unfoldedArrivalDays(t, result); !reflect.DeepEqual(arrivals, []int64{7, 14, 21, 28}) {
		t.Fatalf("arrivals %v, want [7 14 21 28]", arrivals)
	}
	if result.After.Day != 33 || result.After.Schedule.NextDay != 35 || countdown(t, result.After) != 2 {
		t.Fatalf("day %d, D %d; want 33, 35 (in 2)", result.After.Day, result.After.Schedule.NextDay)
	}
	if food(result.After) != 200 {
		t.Fatalf("food %d, want 200", food(result.After))
	}
	if result.ArrivalCount != 4 || len(result.ArrivalEntries) != 4 {
		t.Fatalf("want four SHIPMENT_ARRIVED entries, got %d (count %d)", len(result.ArrivalEntries), result.ArrivalCount)
	}
}

func TestExampleD1FirstScheduleDefaultsToDayPlusInterval(t *testing.T) {
	result, err := SetSchedule(colony(20, 100, unassignedOnly(1), nil), ScheduleInput{IntervalDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	if result.Before != nil || *result.After != (Schedule{IntervalDays: 7, NextDay: 27}) || countdown(t, result.State) != 7 {
		t.Fatalf("schedule %+v, want interval 7, next day 27", result.After)
	}
}

func TestExampleD2LoweringTheIntervalKeepsTheNextDay(t *testing.T) {
	result, err := SetSchedule(colony(10, 100, unassignedOnly(1), &Schedule{IntervalDays: 7, NextDay: 14}), ScheduleInput{IntervalDays: 3})
	if err != nil {
		t.Fatal(err)
	}
	if *result.After != (Schedule{IntervalDays: 3, NextDay: 14}) {
		t.Fatalf("schedule %+v, want interval 3, next day 14", result.After)
	}
	advanced := mustAdvance(t, result.State, 10)
	if arrivals := unfoldedArrivalDays(t, advanced); !reflect.DeepEqual(arrivals, []int64{14, 17, 20}) {
		t.Fatalf("arrivals %v, want [14 17 20]", arrivals)
	}
	if advanced.After.Schedule.NextDay != 23 {
		t.Fatalf("D %d, want 23", advanced.After.Schedule.NextDay)
	}
}

func TestExampleD3NextDayMustBeInTheFuture(t *testing.T) {
	state := colony(10, 100, unassignedOnly(1), nil)
	accepted, err := SetSchedule(state, ScheduleInput{IntervalDays: 7, NextDay: pointer(int64(11))})
	if err != nil || accepted.After.NextDay != 11 {
		t.Fatalf("D = 11 on day 10: %+v, %v", accepted.After, err)
	}
	_, err = SetSchedule(state, ScheduleInput{IntervalDays: 7, NextDay: pointer(int64(10))})
	assertInvalid(t, err, "nextShipmentDay", "The next shipment must arrive after Day 10")
}

func TestExampleD4ClearAndReEnable(t *testing.T) {
	cleared := ClearSchedule(colony(10, 100, unassignedOnly(1), &Schedule{IntervalDays: 7, NextDay: 14}))
	if cleared.State.Schedule != nil || cleared.After != nil || *cleared.Before != (Schedule{IntervalDays: 7, NextDay: 14}) {
		t.Fatalf("clear: %+v", cleared)
	}
	advanced := mustAdvance(t, cleared.State, 20)
	if advanced.ArrivalCount != 0 || len(advanced.ArrivalEntries) != 0 || advanced.After.Schedule != nil {
		t.Fatalf("advance with no schedule logged arrivals: %+v", advanced.ArrivalEntries)
	}
	reEnabled, err := SetSchedule(advanced.After, ScheduleInput{IntervalDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	if reEnabled.State.Day != 30 || *reEnabled.After != (Schedule{IntervalDays: 7, NextDay: 37}) {
		t.Fatalf("re-enable on day %d: %+v, want next day 37", reEnabled.State.Day, reEnabled.After)
	}
}

func TestExampleD5CorrectDaySkipsShipmentDates(t *testing.T) {
	state := colony(10, 100, exampleBRoles(), &Schedule{IntervalDays: 7, NextDay: 14})
	correction, err := CorrectDay(state, 30)
	if err != nil {
		t.Fatal(err)
	}
	if correction.State.Day != 30 || correction.State.Schedule.NextDay != 35 || correction.SkippedShipments != 3 {
		t.Fatalf("day %d, D %d, skipped %d; want 30, 35, 3", correction.State.Day, correction.State.Schedule.NextDay, correction.SkippedShipments)
	}
	if food(correction.State) != 100 || correction.FromDay != 10 || correction.ToDay != 30 {
		t.Fatalf("correct day ran upkeep or lost its days: %+v", correction)
	}
}

// Example E (double submit) is a write-path guard: the version and its 409 live in internal/core, which
// tests it. The rule's part is that the first advance lands on Day 20.
func TestExampleEFirstSubmitLandsOnDayTwenty(t *testing.T) {
	result := mustAdvance(t, colony(10, 500, exampleBRoles(), &Schedule{IntervalDays: 7, NextDay: 14}), 10)
	if result.After.Day != 20 {
		t.Fatalf("day %d, want 20", result.After.Day)
	}
}

func TestExampleFQuestRewardClampsAndLandsInUnassigned(t *testing.T) {
	roles := map[string]Role{
		"FARMER":          {UnnamedCount: 3, NamedCount: 1},
		"BUILDER":         {UnnamedCount: 5},
		"SOLDIER":         {UnnamedCount: 4},
		"HEALER":          {UnnamedCount: 3},
		UnassignedRoleKey: {UnnamedCount: 0},
	}
	state := colony(23, 0, roles, nil)
	state.Resources["morale"] = Resource{Label: "Morale", Amount: 98, MaxAmount: pointer(int64(100))}
	if Colonists(state.Roles) != 16 {
		t.Fatalf("before: colonists %d, want 16", Colonists(state.Roles))
	}

	reward, err := ApplyEffects(state, Effects{Resources: map[string]int64{"food": 20, "morale": 5}, Colonists: 3}, Clamp)
	if err != nil {
		t.Fatal(err)
	}
	after := reward.After
	if food(after) != 20 || after.Resources["morale"].Amount != 100 || after.Roles[UnassignedRoleKey].UnnamedCount != 3 || Colonists(after.Roles) != 19 {
		t.Fatalf("after: food %d, morale %d, Unassigned %d, colonists %d", food(after), after.Resources["morale"].Amount,
			after.Roles[UnassignedRoleKey].UnnamedCount, Colonists(after.Roles))
	}
	wantChanges := []ResourceChange{
		{Key: "food", Before: 0, After: 20, Requested: 20},
		{Key: "morale", Before: 98, After: 100, Requested: 5, Clamped: true},
	}
	if !reflect.DeepEqual(reward.Changes, wantChanges) {
		t.Fatalf("preview changes %+v, want %+v (morale capped)", reward.Changes, wantChanges)
	}
	if want := (Effects{Resources: map[string]int64{"food": 20, "morale": 2}, Colonists: 3}); !reflect.DeepEqual(reward.Applied, want) {
		t.Fatalf("QUEST_COMPLETED deltas %+v, want %+v", reward.Applied, want)
	}

	vex, err := AddFavor(Patron{Name: "Lady Vex", Favor: 1}, 2)
	if err != nil || vex.Favor != 3 {
		t.Fatalf("Lady Vex favor %d, %v; want 3", vex.Favor, err)
	}

	// The whole of Complete's rule in one call: the reward in CLAMP mode and reward_favor to the giver.
	// The status guard behind "a second Complete gets 409 Already resolved" lives in internal/core.
	completed, err := ResolveQuest(state, &Patron{Name: "Lady Vex", Favor: 1}, Effects{Resources: map[string]int64{"food": 20, "morale": 5}, Colonists: 3}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(completed.Effects, reward) || completed.PatronBefore.Favor != 1 || completed.PatronAfter.Favor != 3 {
		t.Fatalf("Complete: %+v, favor %d → %d; want Example F's numbers", completed.Effects.Applied, completed.PatronBefore.Favor, completed.PatronAfter.Favor)
	}
}

func TestExampleGStrictShortfall(t *testing.T) {
	state := colony(12, 20, unassignedOnly(4), nil)
	grainBarge := Effects{Resources: map[string]int64{"food": 7}}
	vex := Patron{Name: "Lady Vex", Favor: 5}

	granted, err := Spend(state, vex, 3, grainBarge)
	if err != nil {
		t.Fatal(err)
	}
	if granted.PatronAfter.Favor != 2 || food(granted.Effects.After) != 27 {
		t.Fatalf("after Grant: favor %d, food %d; want 2, 27", granted.PatronAfter.Favor, food(granted.Effects.After))
	}

	// Requesting again (422) and the variant's second approval (409) are the same check; the variant's
	// two pending requests run end to end in internal/core.
	_, err = Spend(granted.Effects.After, granted.PatronAfter, 3, grainBarge)
	var shortfallError *ShortfallError
	if !errors.As(err, &shortfallError) || err.Error() != "Not enough favor: Lady Vex has 2, needs 3" {
		t.Fatalf("second request: %v", err)
	}

	// §5.4's resource shortfall text.
	_, err = Spend(colony(12, 4, unassignedOnly(4), nil), vex, 0, Effects{Resources: map[string]int64{"food": -10}})
	if err == nil || err.Error() != "Not enough food: the colony has 4, needs 10" {
		t.Fatalf("food shortfall: %v", err)
	}
}

func TestExampleHHugeNFolds(t *testing.T) {
	result := mustAdvance(t, colony(40, 300, unassignedOnly(20), &Schedule{IntervalDays: 7, NextDay: 42}), 1_000_000)
	if result.Before.Day != 40 || result.After.Day != 1_000_040 || food(result.Before) != 300 || food(result.After) != 0 {
		t.Fatalf("preview: Day %d → %d, Food %d → %d", result.Before.Day, result.After.Day, food(result.Before), food(result.After))
	}
	if result.ArrivalCount != 142_857 {
		t.Fatalf("arrival count %d, want 142857", result.ArrivalCount)
	}
	if result.After.Schedule.NextDay != 1_000_041 || countdown(t, result.After) != 1 {
		t.Fatalf("D %d, want 1,000,041 (in 1)", result.After.Schedule.NextDay)
	}
	want := []ArrivalEntry{{Day: 1_000_034, Folded: true, Run: ArrivalRun{First: 42, Every: 7, Count: 142_857}}}
	if !reflect.DeepEqual(result.ArrivalEntries, want) {
		t.Fatalf("SHIPMENT_ARRIVED entries %+v, want exactly one folded %+v", result.ArrivalEntries, want)
	}
}

// Example I's state part: undo restores the snapshot the handler took from Before. Marking entries
// undone, the UNDO entry and the version check live in internal/core.
func TestExampleIUndoRestoresBefore(t *testing.T) {
	start := colony(0, 100, unassignedOnly(2), &Schedule{IntervalDays: 7, NextDay: 7})
	atDaySix := mustAdvance(t, start, 6).After
	advanceOne := mustAdvance(t, atDaySix, 1)
	if advanceOne.After.Day != 7 || food(advanceOne.After) == food(atDaySix) {
		t.Fatalf("advance 1 did not move the colony: %+v", advanceOne.After)
	}
	restored := advanceOne.Before
	if restored.Day != 6 || restored.Schedule.NextDay != 7 || food(restored) != food(atDaySix) {
		t.Fatalf("restored day %d, D %d, food %d; want 6, 7, %d", restored.Day, restored.Schedule.NextDay, food(restored), food(atDaySix))
	}
	if !reflect.DeepEqual(restored, atDaySix) {
		t.Fatalf("Before %+v is not the state that was advanced %+v", restored, atDaySix)
	}
}
