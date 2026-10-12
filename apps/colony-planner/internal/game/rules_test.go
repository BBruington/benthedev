package game

import (
	"errors"
	"reflect"
	"testing"
)

func TestColonistsCountUnnamedAndNamedInEveryRole(t *testing.T) {
	roles := map[string]Role{
		"FARMER":          {UnnamedCount: 3, NamedCount: 1},
		"SCHOLAR":         {Archived: true},
		UnassignedRoleKey: {UnnamedCount: 5},
	}
	if colonists := Colonists(roles); colonists != 9 {
		t.Fatalf("colonists %d, want 9", colonists)
	}
	if colonists := Colonists(nil); colonists != 0 {
		t.Fatalf("no roles: %d colonists", colonists)
	}
}

func TestScheduleValidation(t *testing.T) {
	state := colony(10, 100, unassignedOnly(1), nil)
	_, err := SetSchedule(state, ScheduleInput{IntervalDays: 0})
	assertInvalid(t, err, "intervalDays", MessageIntervalTooSmall)
	_, err = SetSchedule(state, ScheduleInput{IntervalDays: 7, NextDay: pointer(int64(MaxStored) + 1)})
	assertInvalid(t, err, "nextShipmentDay", MessageTooManyDays)
	_, err = SetSchedule(colony(MaxStored-3, 100, unassignedOnly(1), nil), ScheduleInput{IntervalDays: 7})
	assertInvalid(t, err, "nextShipmentDay", MessageTooManyDays)

	// Setting a schedule leaves the input alone.
	if _, err := SetSchedule(state, ScheduleInput{IntervalDays: 7}); err != nil || state.Schedule != nil {
		t.Fatalf("SetSchedule changed its input or failed: %v", err)
	}
}

func TestCorrectDay(t *testing.T) {
	state := colony(10, 100, unassignedOnly(1), &Schedule{IntervalDays: 7, NextDay: 14})

	backward, err := CorrectDay(state, 4)
	if err != nil || backward.State.Schedule.NextDay != 14 || backward.SkippedShipments != 0 {
		t.Fatalf("backward: %+v, %v", backward, err)
	}
	exactlyOnArrival, err := CorrectDay(state, 14)
	if err != nil || exactlyOnArrival.State.Schedule.NextDay != 21 || exactlyOnArrival.SkippedShipments != 1 {
		t.Fatalf("onto the arrival day: %+v, %v", exactlyOnArrival, err)
	}
	beforeArrival, err := CorrectDay(state, 13)
	if err != nil || beforeArrival.State.Schedule.NextDay != 14 || beforeArrival.SkippedShipments != 0 {
		t.Fatalf("short of the arrival day: %+v, %v", beforeArrival, err)
	}

	_, err = CorrectDay(state, -1)
	assertInvalid(t, err, "day", MessageNegativeDay)
	_, err = CorrectDay(state, int64(MaxStored)+1)
	assertInvalid(t, err, "day", MessageTooManyDays)
	_, err = CorrectDay(colony(10, 100, unassignedOnly(1), &Schedule{IntervalDays: 10, NextDay: 14}), MaxStored-1)
	assertInvalid(t, err, "day", MessageTooManyDays)
}

func TestClampFloorsAtTheMinimum(t *testing.T) {
	result, err := ApplyEffects(colony(0, 4, unassignedOnly(1), nil), Effects{Resources: map[string]int64{"food": -10, "gold": 0}}, Clamp)
	if err != nil {
		t.Fatal(err)
	}
	if food(result.After) != 0 || !reflect.DeepEqual(result.Applied, Effects{Resources: map[string]int64{"food": -4}}) {
		t.Fatalf("food %d, applied %+v", food(result.After), result.Applied)
	}
	if want := []ResourceChange{{Key: "food", Before: 4, After: 0, Requested: -10, Clamped: true}}; !reflect.DeepEqual(result.Changes, want) {
		t.Fatalf("changes %+v, want %+v", result.Changes, want)
	}
}

func TestClampFullyCappedDeltaIsNotLogged(t *testing.T) {
	result, err := ApplyEffects(colony(0, 4, unassignedOnly(1), nil), Effects{Resources: map[string]int64{"morale": 5}}, Clamp)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied.Resources) != 0 || !result.Changes[0].Clamped || result.Changes[0].Applied() != 0 {
		t.Fatalf("applied %+v, changes %+v", result.Applied, result.Changes)
	}
}

func TestStrictShortfallUsesTheMinimumAndNamesEveryShortfall(t *testing.T) {
	state := colony(0, 12, unassignedOnly(1), nil)
	state.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 12, MinAmount: 5}
	_, err := Spend(state, Patron{Name: "Lady Vex", Favor: 1}, 3, Effects{Resources: map[string]int64{"food": -10, "gold": -60}})
	var shortfallError *ShortfallError
	if !errors.As(err, &shortfallError) {
		t.Fatalf("want a ShortfallError, got %v", err)
	}
	want := "Not enough favor: Lady Vex has 1, needs 3. Not enough food: the colony has 12, needs 15. Not enough gold: the colony has 50, needs 60"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}

func TestStrictStillCapsPositiveDeltas(t *testing.T) {
	state := colony(0, 20, unassignedOnly(1), nil)
	state.Resources["morale"] = Resource{Label: "Morale", Amount: 98, MaxAmount: pointer(int64(100))}
	result, err := ApplyEffects(state, Effects{Resources: map[string]int64{"morale": 5, "food": -20}}, Strict)
	if err != nil {
		t.Fatal(err)
	}
	if result.After.Resources["morale"].Amount != 100 || food(result.After) != 0 {
		t.Fatalf("morale %d, food %d", result.After.Resources["morale"].Amount, food(result.After))
	}
}

func TestEffectsValidation(t *testing.T) {
	state := colony(0, 20, unassignedOnly(1), nil)
	state.Resources["timber"] = Resource{Label: "Timber", Archived: true}

	_, err := ApplyEffects(state, Effects{Colonists: -1}, Clamp)
	assertInvalid(t, err, "colonists", MessageNegativeColonist)
	_, err = ApplyEffects(state, Effects{Resources: map[string]int64{"spice": 1}}, Clamp)
	assertInvalid(t, err, "resources", `There is no resource "spice"`)
	_, err = ApplyEffects(state, Effects{Resources: map[string]int64{"timber": 1}}, Clamp)
	assertInvalid(t, err, "resources", "Timber is archived")
	_, err = ApplyEffects(state, Effects{Resources: map[string]int64{"gold": MaxStored}}, Clamp)
	assertInvalid(t, err, "resources", "That much gold can't be stored")
	_, err = ApplyEffects(state, Effects{Resources: map[string]int64{"food": -1 << 62}}, Clamp)
	assertInvalid(t, err, "resources", "That much food can't be stored")
	full := colony(0, 20, map[string]Role{UnassignedRoleKey: {UnnamedCount: MaxStored}}, nil)
	_, err = ApplyEffects(full, Effects{Colonists: 1}, Clamp)
	assertInvalid(t, err, "colonists", MessageTooManyColonists)
}

func TestApplyEffectsDoesNotChangeItsInput(t *testing.T) {
	state := colony(0, 20, unassignedOnly(1), nil)
	copied := state.Clone()
	result, err := ApplyEffects(state, Effects{Resources: map[string]int64{"food": 5}, Colonists: 2}, Clamp)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state, copied) || !reflect.DeepEqual(result.Before, copied) {
		t.Fatalf("ApplyEffects changed its input: %+v", state)
	}
}

func TestAdjustmentFromTargets(t *testing.T) {
	state := colony(0, 20, unassignedOnly(1), nil)
	effects, err := AdjustmentFromTargets(state, map[string]int64{"food": 35, "gold": 50, "morale": 120})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Effects{Resources: map[string]int64{"food": 15, "morale": 20}}); !reflect.DeepEqual(effects, want) {
		t.Fatalf("effects %+v, want %+v", effects, want)
	}
	result, err := ApplyEffects(state, effects, Clamp)
	if err != nil || result.After.Resources["morale"].Amount != 100 {
		t.Fatalf("set morale to 120 should cap at 100: %+v, %v", result.After.Resources["morale"], err)
	}
	_, err = AdjustmentFromTargets(state, map[string]int64{"spice": 1})
	assertInvalid(t, err, "resources", `There is no resource "spice"`)
}

func TestFavor(t *testing.T) {
	below, err := AddFavor(Patron{Name: "Lady Vex", Favor: 1}, -4)
	if err != nil || below.Favor != -3 {
		t.Fatalf("a DM adjustment may take favor below 0: %+v, %v", below, err)
	}
	_, err = AddFavor(Patron{Name: "Lady Vex", Favor: MaxStored}, 1)
	assertInvalid(t, err, "favor", "That much favor can't be stored")

	free, err := Spend(colony(0, 20, unassignedOnly(1), nil), below, 0, Effects{})
	if err != nil || free.PatronAfter.Favor != -3 {
		t.Fatalf("a free action needs no favor: %+v, %v", free, err)
	}
	_, err = Spend(colony(0, 20, unassignedOnly(1), nil), below, -1, Effects{})
	assertInvalid(t, err, "favorCost", "A favor cost can't be negative")
}

func TestResolveQuestFailureCost(t *testing.T) {
	state := colony(30, 5, unassignedOnly(2), nil)
	vex := Patron{Name: "Lady Vex", Favor: 1}
	failed, err := ResolveQuest(state, &vex, Effects{Resources: map[string]int64{"food": -10}}, -3)
	if err != nil {
		t.Fatal(err)
	}
	if food(failed.Effects.After) != 0 || failed.Effects.Applied.Resources["food"] != -5 || !failed.Effects.Changes[0].Clamped {
		t.Fatalf("a failure cost clamps at the minimum: %+v", failed.Effects.Changes)
	}
	if failed.PatronAfter.Favor != -2 || vex.Favor != 1 {
		t.Fatalf("the favor loss applies in full, below 0 too: %d (input now %d)", failed.PatronAfter.Favor, vex.Favor)
	}

	nothing, err := ResolveQuest(state, nil, Effects{}, 0)
	if err != nil || nothing.PatronAfter != nil || len(nothing.Effects.Changes) != 0 {
		t.Fatalf("a quest with no cost and no patron: %+v, %v", nothing, err)
	}
	_, err = ResolveQuest(state, nil, Effects{}, -1)
	assertInvalid(t, err, "patronId", "Favor needs a patron giver")
}
