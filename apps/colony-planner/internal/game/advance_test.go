package game

import (
	"math"
	"reflect"
	"slices"
	"testing"
)

// Fixed-case equivalence checks for the span engine (spec §5.2 "Guarantees"), over hand-picked states.

func equivalenceStates() map[string]State {
	foodWithFloor := colony(5, 37, unassignedOnly(3), &Schedule{IntervalDays: 4, NextDay: 6})
	foodWithFloor.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 37, MinAmount: 10}
	return map[string]State{
		"example B":                  colony(0, 100, exampleBRoles(), &Schedule{IntervalDays: 7, NextDay: 7}),
		"no schedule":                colony(12, 140, unassignedOnly(12), nil),
		"daily shipments":            colony(3, 9, unassignedOnly(1), &Schedule{IntervalDays: 1, NextDay: 4}),
		"no colonists":               colony(0, 5, map[string]Role{UnassignedRoleKey: {}}, &Schedule{IntervalDays: 30, NextDay: 2}),
		"food at the floor":          colony(40, 0, unassignedOnly(20), &Schedule{IntervalDays: 7, NextDay: 42}),
		"food floor above zero":      foodWithFloor,
		"next arrival far in future": colony(1, 1000, exampleBRoles(), &Schedule{IntervalDays: 3, NextDay: 50}),
		"one project":                constructionExample(),
		"projects that feed upkeep":  projectsThatFeedUpkeep(),
		"output at floors and caps":  outputAtFloorsAndCaps(),
		"stock above its cap":        stockAboveItsCap(),
		"finishes that raise caps":   finishesThatRaiseCaps(),
		"food output beside upkeep":  foodOutputBesideUpkeep(),
	}
}

// outputAtFloorsAndCaps has standing buildings whose output runs materials down to its floor, morale
// up to its cap, gold (no cap) up and food up less than the colony eats, beside an Idle building
// whose output must change nothing.
func outputAtFloorsAndCaps() State {
	state := colony(8, 20, unassignedOnly(3), &Schedule{IntervalDays: 5, NextDay: 10})
	state.Resources["materials"] = Resource{Label: "Materials", Amount: 30, MinAmount: 4}
	state.Resources["morale"] = Resource{Label: "Morale", Amount: 81, MaxAmount: pointer(int64(100))}
	state.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 20, MaxAmount: pointer(int64(50))}
	state.Buildings = []StandingBuilding{
		{BuildingID: buildingID(1), Active: true, Output: map[string]ResourceOutput{"materials": {DailyDelta: -3}, "morale": {DailyDelta: 4}}},
		{BuildingID: buildingID(2), Active: true, Output: map[string]ResourceOutput{"gold": {DailyDelta: 7}, FoodResourceKey: {DailyDelta: 2}}},
		{BuildingID: buildingID(3), Output: map[string]ResourceOutput{"materials": {DailyDelta: 50}, FoodResourceKey: {DailyDelta: 40, StorageBonus: 100}}},
	}
	return state
}

// stockAboveItsCap is the K13 case: an Idle Granary left food and materials above their caps, a Mill
// still makes food and a quarry materials, so food falls by what the colony eats until it is under
// its cap, and materials stay where they are.
func stockAboveItsCap() State {
	state := colony(30, 250, unassignedOnly(5), nil)
	state.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 250, MaxAmount: pointer(int64(100))}
	state.Resources["materials"] = Resource{Label: "Materials", Amount: 90, MaxAmount: pointer(int64(60))}
	state.Buildings = []StandingBuilding{
		{BuildingID: buildingID(1), Output: map[string]ResourceOutput{FoodResourceKey: {StorageBonus: 200}, "materials": {StorageBonus: 50}}},
		{BuildingID: buildingID(2), Active: true, Output: map[string]ResourceOutput{FoodResourceKey: {DailyDelta: 10}, "materials": {DailyDelta: 2}}},
	}
	return state
}

// finishesThatRaiseCaps has a Mill standing and two projects: a Granary whose storage bonus raises
// food's cap from the end of its finish day and whose own output starts the day after, and a
// feast hall finishing later whose completion food fills the raised cap.
func finishesThatRaiseCaps() State {
	state := colony(2, 95, map[string]Role{
		"BUILDER":         {Label: "Builder", UnnamedCount: 3},
		UnassignedRoleKey: {Label: "Unassigned", UnnamedCount: 1},
	}, nil)
	state.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 95, MaxAmount: pointer(int64(100))}
	state.Buildings = []StandingBuilding{{BuildingID: buildingID(9), Active: true, Output: map[string]ResourceOutput{FoodResourceKey: {DailyDelta: 9}}}}
	state.Projects = []Project{
		{BuildingID: buildingID(1), Lines: []LaborLine{{RoleKey: "BUILDER", Effort: 8, Crew: 2}},
			Output: map[string]ResourceOutput{FoodResourceKey: {StorageBonus: 200}, "gold": {DailyDelta: -6}}},
		{BuildingID: buildingID(2), Lines: []LaborLine{{RoleKey: "BUILDER", Effort: 11, Crew: 1}},
			CompletionEffects: Effects{Resources: map[string]int64{FoodResourceKey: 500}}},
	}
	return state
}

// foodOutputBesideUpkeep has food made faster than it is eaten, toward a cap it then sits just under.
func foodOutputBesideUpkeep() State {
	state := colony(0, 3, unassignedOnly(4), &Schedule{IntervalDays: 2, NextDay: 1})
	state.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 3, MinAmount: 1, MaxAmount: pointer(int64(60))}
	state.Buildings = []StandingBuilding{
		{BuildingID: buildingID(1), Active: true, Output: map[string]ResourceOutput{FoodResourceKey: {DailyDelta: 6}}},
		{BuildingID: buildingID(2), Active: true, Output: map[string]ResourceOutput{FoodResourceKey: {DailyDelta: 5}, "morale": {DailyDelta: -1}}},
	}
	return state
}

// projectsThatFeedUpkeep has two projects, one with two lines finishing on different days, whose
// completion Effects bring colonists (so upkeep eats more from the next day) and food, and a stalled
// project whose Mage line has no crew.
func projectsThatFeedUpkeep() State {
	state := colony(3, 60, map[string]Role{
		"BUILDER":         {Label: "Builder", UnnamedCount: 5},
		"MAGE":            {Label: "Mage", UnnamedCount: 2},
		UnassignedRoleKey: {Label: "Unassigned", UnnamedCount: 3},
	}, &Schedule{IntervalDays: 4, NextDay: 5})
	state.Projects = []Project{
		{BuildingID: buildingID(1), Lines: []LaborLine{
			{RoleKey: "BUILDER", Effort: 13, Crew: 3},
			{RoleKey: "MAGE", Effort: 7, MaxCrew: pointer(int64(2)), Crew: 2},
		}, CompletionEffects: Effects{Resources: map[string]int64{"morale": -10}, Colonists: 4}},
		{BuildingID: buildingID(2), Lines: []LaborLine{{RoleKey: "BUILDER", Effort: 30, Progress: 5, Crew: 2}},
			CompletionEffects: Effects{Resources: map[string]int64{"food": 50, "morale": 20}}},
		{BuildingID: buildingID(3), Lines: []LaborLine{
			{RoleKey: "BUILDER", Effort: 4},
			{RoleKey: "MAGE", Effort: 9},
		}},
	}
	return state
}

// referenceAdvance is the naive day-by-day loop the closed forms must match: each resource's building
// output (never lowering an amount above its cap), upkeep, shipments, then each line's crew working one
// day, a finished line freeing its crew and a finished project standing and applying its completion
// Effects.
func referenceAdvance(state State, days int64) (State, []int64, []BuildingFinish) {
	after := state.Clone()
	arrivals := []int64{}
	finishes := []BuildingFinish{}
	for range days {
		after.Day++
		outputs := map[string]int64{}
		for _, building := range after.Buildings {
			for key, output := range building.Output {
				if building.Active {
					outputs[key] += output.DailyDelta
				}
			}
		}
		caps := map[string]int64{}
		for key := range outputs {
			caps[key] = MaxStored
			if effective := EffectiveMax(after, key); effective != nil {
				caps[key] = *effective
			}
		}
		for key, delta := range outputs {
			resource, found := after.Resources[key]
			if !found || resource.Archived {
				continue
			}
			switch {
			case delta > 0 && resource.Amount < caps[key]:
				resource.Amount = min(resource.Amount+delta, caps[key])
			case delta < 0:
				resource.Amount = max(resource.Amount+delta, resource.MinAmount)
			}
			after.Resources[key] = resource
		}
		food := after.Resources[FoodResourceKey]
		food.Amount = max(food.Amount-Colonists(after.Roles)*FoodPerColonistPerDay, food.MinAmount)
		after.Resources[FoodResourceKey] = food
		if after.Schedule != nil && after.Schedule.NextDay == after.Day {
			arrivals = append(arrivals, after.Day)
			after.Schedule.NextDay += after.Schedule.IntervalDays
		}
		for index := range after.Projects {
			project := &after.Projects[index]
			finishedToday := false
			for lineIndex := range project.Lines {
				line := &project.Lines[lineIndex]
				if line.Crew == 0 || line.Progress == line.Effort {
					continue
				}
				line.Progress = min(line.Progress+line.Crew, line.Effort)
				if line.Progress == line.Effort {
					line.Crew = 0
					finishedToday = true
				}
			}
			if finishedToday && allDone(project.Lines) {
				standUp(&after, *project)
				applied, skipped := applyCompletion(&after, project.CompletionEffects)
				finishes = append(finishes, BuildingFinish{BuildingID: project.BuildingID, Day: after.Day, Applied: applied, Skipped: skipped, Lines: slices.Clone(project.Lines)})
			}
		}
		after.Projects = slices.DeleteFunc(after.Projects, func(project Project) bool { return allDone(project.Lines) })
	}
	return after, arrivals, finishes
}

func expandedArrivals(result AdvanceResult) []int64 {
	days := []int64{}
	for _, run := range result.Arrivals {
		for arrival := range run.Count {
			days = append(days, run.First+arrival*run.Every)
		}
	}
	return days
}

func TestSpanEngineMatchesTheDayByDayReference(t *testing.T) {
	for name, state := range equivalenceStates() {
		for _, days := range []int64{1, 2, 3, 6, 7, 8, 13, 30, 100} {
			result := mustAdvance(t, state, days)
			wantState, wantArrivals, wantFinishes := referenceAdvance(state, days)
			if !reflect.DeepEqual(result.After, wantState) {
				t.Fatalf("%s, advance %d: %+v, reference %+v", name, days, result.After, wantState)
			}
			if arrivals := expandedArrivals(result); !reflect.DeepEqual(arrivals, wantArrivals) {
				t.Fatalf("%s, advance %d: arrivals %v, reference %v", name, days, arrivals, wantArrivals)
			}
			if result.ArrivalCount != int64(len(wantArrivals)) {
				t.Fatalf("%s, advance %d: arrival count %d, want %d", name, days, result.ArrivalCount, len(wantArrivals))
			}
			if !reflect.DeepEqual(result.Finishes, wantFinishes) {
				t.Fatalf("%s, advance %d: finishes %+v, reference %+v", name, days, result.Finishes, wantFinishes)
			}
			// A finish's completion Effects are its own entry's deltas, not the advance's; output is the advance's.
			for key, resource := range wantState.Resources {
				wantDelta := resource.Amount - state.Resources[key].Amount
				for _, finish := range wantFinishes {
					wantDelta -= finish.Applied.Resources[key]
				}
				if result.Applied.Resources[key] != wantDelta {
					t.Fatalf("%s, advance %d: %s delta %d, want %d", name, days, key, result.Applied.Resources[key], wantDelta)
				}
			}
		}
	}
}

func TestAdvancingInPartsEqualsAdvancingOnce(t *testing.T) {
	for name, state := range equivalenceStates() {
		for _, parts := range [][2]int64{{3, 4}, {1, 6}, {7, 7}, {10, 20}, {2, 98}} {
			whole := mustAdvance(t, state, parts[0]+parts[1])
			first := mustAdvance(t, state, parts[0])
			second := mustAdvance(t, first.After, parts[1])
			if !reflect.DeepEqual(second.After, whole.After) {
				t.Fatalf("%s: advance %d then %d gives %+v, advance %d gives %+v", name, parts[0], parts[1],
					second.After, parts[0]+parts[1], whole.After)
			}
			if !reflect.DeepEqual(append(expandedArrivals(first), expandedArrivals(second)...), expandedArrivals(whole)) {
				t.Fatalf("%s: arrivals split %d+%d differ from the whole", name, parts[0], parts[1])
			}
			if !reflect.DeepEqual(append(slices.Clone(first.Finishes), second.Finishes...), whole.Finishes) {
				t.Fatalf("%s: finishes split %d+%d differ from the whole", name, parts[0], parts[1])
			}
		}
	}
}

func TestOneDayNTimesEqualsNDaysOnce(t *testing.T) {
	for name, state := range equivalenceStates() {
		const days = 15
		stepped := state
		for range days {
			stepped = mustAdvance(t, stepped, 1).After
		}
		if once := mustAdvance(t, state, days).After; !reflect.DeepEqual(stepped, once) {
			t.Fatalf("%s: one day %d times gives %+v, %d days once gives %+v", name, days, stepped, days, once)
		}
	}
}

// boundaryStep changes nothing but reports boundaries, so it exercises the walk without changing
// the answer: every day (a day-by-day loop), or on one fixed day.
type boundaryStep struct {
	fixedDay int64
}

func (step boundaryStep) Span(*State, int64, int64, *Log) {}

func (step boundaryStep) NextBoundary(_ State, after int64) (int64, bool) {
	if step.fixedDay == 0 {
		return after + 1, true
	}
	if step.fixedDay > after {
		return step.fixedDay, true
	}
	return 0, false
}

func TestBoundariesDoNotChangeTheResult(t *testing.T) {
	for name, state := range equivalenceStates() {
		for _, extra := range []boundaryStep{{fixedDay: 0}, {fixedDay: state.Day + 5}, {fixedDay: state.Day + 1}} {
			steps := append(append([]Step{}, DailySteps...), extra)
			walked, err := advance(state, 30, steps)
			if err != nil {
				t.Fatal(err)
			}
			single := mustAdvance(t, state, 30)
			if !reflect.DeepEqual(walked, single) {
				t.Fatalf("%s with boundary %+v: %+v, single span %+v", name, extra, walked, single)
			}
		}
	}
}

func TestAdvanceDoesNotChangeItsInput(t *testing.T) {
	state := colony(0, 100, exampleBRoles(), &Schedule{IntervalDays: 7, NextDay: 7})
	copied := state.Clone()
	mustAdvance(t, state, 10)
	if !reflect.DeepEqual(state, copied) {
		t.Fatalf("Advance changed its input: %+v", state)
	}
}

func TestTwelveArrivalsUnfoldAndThirteenFold(t *testing.T) {
	state := colony(0, 1000, unassignedOnly(1), &Schedule{IntervalDays: 1, NextDay: 1})
	twelve := mustAdvance(t, state, 12)
	if len(twelve.ArrivalEntries) != 12 || twelve.ArrivalEntries[11] != (ArrivalEntry{Day: 12, Run: ArrivalRun{First: 12, Every: 1, Count: 1}}) {
		t.Fatalf("12 arrivals: %+v", twelve.ArrivalEntries)
	}
	thirteen := mustAdvance(t, state, 13)
	want := []ArrivalEntry{{Day: 13, Folded: true, Run: ArrivalRun{First: 1, Every: 1, Count: 13}}}
	if !reflect.DeepEqual(thirteen.ArrivalEntries, want) {
		t.Fatalf("13 arrivals: %+v, want %+v", thirteen.ArrivalEntries, want)
	}
}

func TestAdvanceOverflowGuard(t *testing.T) {
	nearTheEnd := colony(MaxStored-5, 100, unassignedOnly(1), nil)
	if result := mustAdvance(t, nearTheEnd, 5); result.After.Day != MaxStored {
		t.Fatalf("day %d, want %d", result.After.Day, MaxStored)
	}
	_, err := Advance(nearTheEnd, 6)
	assertInvalid(t, err, "days", MessageTooManyDays)
	_, err = Advance(nearTheEnd, math.MaxInt64)
	assertInvalid(t, err, "days", MessageTooManyDays)

	// The day fits but the next shipment day would not.
	scheduled := colony(MaxStored-10, 100, unassignedOnly(1), &Schedule{IntervalDays: 7, NextDay: MaxStored - 5})
	_, err = Advance(scheduled, 5)
	assertInvalid(t, err, "days", MessageTooManyDays)
	if result := mustAdvance(t, scheduled, 4); result.ArrivalCount != 0 {
		t.Fatalf("advance 4 logged arrivals: %+v", result.Arrivals)
	}

	_, err = Advance(nearTheEnd, 0)
	assertInvalid(t, err, "days", MessageAdvanceAtLeast)

	// The largest possible N with the most colonists the columns allow does not overflow upkeep.
	crowded := colony(0, MaxStored, map[string]Role{UnassignedRoleKey: {UnnamedCount: MaxStored}, "FARMER": {UnnamedCount: MaxStored}}, nil)
	if result := mustAdvance(t, crowded, MaxStored); food(result.After) != 0 {
		t.Fatalf("food %d, want 0", food(result.After))
	}
}

func TestAdvanceRefusesAnImpossibleState(t *testing.T) {
	stalled := colony(14, 100, unassignedOnly(1), &Schedule{IntervalDays: 7, NextDay: 14})
	if _, err := Advance(stalled, 1); err == nil {
		t.Fatal("a next shipment day that is not after the day must be refused")
	}
	noFood := colony(0, 100, unassignedOnly(1), nil)
	delete(noFood.Resources, FoodResourceKey)
	if _, err := Advance(noFood, 1); err == nil {
		t.Fatal("advancing without a food resource must be refused")
	}
}
