package game

import (
	"math"
	"reflect"
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
	}
}

// referenceAdvance is the naive day-by-day loop the closed forms must match.
func referenceAdvance(state State, days int64) (State, []int64) {
	after := state.Clone()
	arrivals := []int64{}
	for range days {
		after.Day++
		food := after.Resources[FoodResourceKey]
		food.Amount = max(food.Amount-Colonists(after.Roles)*FoodPerColonistPerDay, food.MinAmount)
		after.Resources[FoodResourceKey] = food
		if after.Schedule != nil && after.Schedule.NextDay == after.Day {
			arrivals = append(arrivals, after.Day)
			after.Schedule.NextDay += after.Schedule.IntervalDays
		}
	}
	return after, arrivals
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
			wantState, wantArrivals := referenceAdvance(state, days)
			if !reflect.DeepEqual(result.After, wantState) {
				t.Fatalf("%s, advance %d: %+v, reference %+v", name, days, result.After, wantState)
			}
			if arrivals := expandedArrivals(result); !reflect.DeepEqual(arrivals, wantArrivals) {
				t.Fatalf("%s, advance %d: arrivals %v, reference %v", name, days, arrivals, wantArrivals)
			}
			if result.ArrivalCount != int64(len(wantArrivals)) {
				t.Fatalf("%s, advance %d: arrival count %d, want %d", name, days, result.ArrivalCount, len(wantArrivals))
			}
			wantFoodDelta := food(wantState) - food(state)
			if result.Applied.Resources[FoodResourceKey] != wantFoodDelta {
				t.Fatalf("%s, advance %d: food delta %d, want %d", name, days, result.Applied.Resources[FoodResourceKey], wantFoodDelta)
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
