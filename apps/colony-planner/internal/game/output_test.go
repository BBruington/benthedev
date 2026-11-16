package game

import (
	"reflect"
	"testing"
)

// dailyMap is one day of a resource done the slow way: output (a gain never lowering an amount above
// the ceiling), then eating, floored at the minimum.
func dailyMap(amount int64, output int64, eaten int64, minimum int64, upperBound int64) int64 {
	switch {
	case output > 0 && amount < upperBound:
		amount = min(amount+output, upperBound)
	case output < 0:
		amount = max(amount+output, minimum)
	}
	return max(amount-eaten, minimum)
}

func TestAmountAfterDaysMatchesTheDailyMap(t *testing.T) {
	for _, minimum := range []int64{0, 3} {
		for _, upperBound := range []int64{minimum, minimum + 1, 12, 20} {
			for amount := minimum; amount <= upperBound+15; amount++ {
				for output := int64(-4); output <= 9; output++ {
					for eaten := int64(0); eaten <= 6; eaten++ {
						want := amount
						for days := int64(1); days <= 25; days++ {
							want = dailyMap(want, output, eaten, minimum, upperBound)
							if got := amountAfterDays(amount, output, eaten, minimum, upperBound, days); got != want {
								t.Fatalf("amount %d, output %d, eaten %d, bounds [%d, %d], %d days: %d, want %d",
									amount, output, eaten, minimum, upperBound, days, got, want)
							}
						}
					}
				}
			}
		}
	}
}

func TestAmountAfterDaysNeverOverflows(t *testing.T) {
	cases := []struct {
		name                                       string
		amount, output, eaten, minimum, upperBound int64
		want                                       int64
	}{
		{"growth to the storable maximum", 0, MaxStored - MinStored, 0, 0, MaxStored, MaxStored},
		{"slow growth over every day", MinStored, 1, 0, MinStored, MaxStored, MinStored + MaxStored},
		{"a crowd eating everything", MaxStored, 1, 2 * MaxStored, MinStored, MaxStored, MinStored},
		{"upkeep past the floor", MaxStored, MinStored - MaxStored, 0, MinStored, MaxStored, MinStored},
	}
	for _, testCase := range cases {
		if got := amountAfterDays(testCase.amount, testCase.output, testCase.eaten, testCase.minimum, testCase.upperBound, MaxStored); got != testCase.want {
			t.Fatalf("%s: %d, want %d", testCase.name, got, testCase.want)
		}
	}
}

// millExample is design §4.2: Day 40, food 12 (min 0, max 100), 5 colonists, 4 Builders, and a Mill
// (20 Builder-days, max crew 4, food +10 a day) started on Day 40 with crew 4.
func millExample() State {
	state := constructionExample()
	state.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 12, MaxAmount: pointer(int64(100))}
	state.Projects[0].Output = map[string]ResourceOutput{FoodResourceKey: {DailyDelta: 10}}
	return state
}

func TestMillWorkedExample(t *testing.T) {
	state := millExample()
	result := mustAdvance(t, state, 7)
	if len(result.Finishes) != 1 || result.Finishes[0].Day != 45 {
		t.Fatalf("finishes %+v, want the Mill on Day 45", result.Finishes)
	}
	wantFood := map[int64]int64{41: 7, 42: 2, 43: 0, 44: 0, 45: 0, 46: 5, 47: 10}
	for day, want := range wantFood {
		if got := food(mustAdvance(t, state, day-state.Day).After); got != want {
			t.Errorf("food on Day %d = %d, want %d", day, got, want)
		}
	}
	if result.Applied.Resources[FoodResourceKey] != -2 {
		t.Fatalf("DAYS_ADVANCED food delta %d, want -2 (12 → 10)", result.Applied.Resources[FoodResourceKey])
	}
	// Days 46 and 47 make 20 and the 7 days eat 35, so the floor cut the change.
	if result.Requested[FoodResourceKey] != 20-35 {
		t.Fatalf("requested food %d, want -15", result.Requested[FoodResourceKey])
	}
	want := []StandingBuilding{{BuildingID: buildingID(1), Active: true, Output: map[string]ResourceOutput{FoodResourceKey: {DailyDelta: 10}}}}
	if !reflect.DeepEqual(result.After.Buildings, want) || len(result.After.Projects) != 0 {
		t.Fatalf("standing %+v, projects %+v", result.After.Buildings, result.After.Projects)
	}
}

// granaryExample has a Mill (food +10) standing and a Granary (food storage +200) finishing on Day 1,
// with food at its cap of 100 and 5 colonists eating.
func granaryExample() State {
	state := colony(0, 100, map[string]Role{
		"BUILDER":         {Label: "Builder", UnnamedCount: 1},
		UnassignedRoleKey: {Label: "Unassigned", UnnamedCount: 4},
	}, nil)
	state.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 100, MaxAmount: pointer(int64(100))}
	state.Buildings = []StandingBuilding{{BuildingID: buildingID(9), Active: true, Output: map[string]ResourceOutput{FoodResourceKey: {DailyDelta: 10}}}}
	state.Projects = []Project{{
		BuildingID: buildingID(1),
		Lines:      []LaborLine{{RoleKey: "BUILDER", Effort: 1, Crew: 1}},
		Output:     map[string]ResourceOutput{FoodResourceKey: {StorageBonus: 200}},
	}}
	return state
}

func TestGranaryStorageExample(t *testing.T) {
	state := granaryExample()
	// Day 1 is capped at 100 before the Granary stands (100 + 10 → 100, then 5 eaten), then +5 a day.
	finished := mustAdvance(t, state, 1).After
	if food(finished) != 95 || *EffectiveMax(finished, FoodResourceKey) != 300 {
		t.Fatalf("Day 1: food %d, cap %v; want 95 under a cap of 300", food(finished), *EffectiveMax(finished, FoodResourceKey))
	}
	full := mustAdvance(t, finished, 31).After
	if food(full) != 250 {
		t.Fatalf("Day 32: food %d, want 250", food(full))
	}

	idle, err := SetBuildingActive(full, buildingID(1), false)
	if err != nil {
		t.Fatal(err)
	}
	if food(idle) != 250 || *EffectiveMax(idle, FoodResourceKey) != 100 {
		t.Fatalf("Idle: food %d, cap %d; want 250 kept above a cap of 100", food(idle), *EffectiveMax(idle, FoodResourceKey))
	}
	// The Mill's +10 adds nothing while food is at or above 100, so it falls by 5 a day to 100, then
	// sits at 95, where each day's output reaches the cap again.
	for days, want := range map[int64]int64{1: 245, 30: 100, 31: 95, 200: 95} {
		if got := food(mustAdvance(t, idle, days).After); got != want {
			t.Errorf("%d days after the Idle: food %d, want %d", days, got, want)
		}
	}
	_, err = SetBuildingActive(idle, buildingID(1), false)
	assertInvalid(t, err, "active", "It is already Idle")
	_, err = SetBuildingActive(idle, buildingID(7), true)
	assertInvalid(t, err, "buildingId", "That building isn't standing")
	active, err := SetBuildingActive(idle, buildingID(1), true)
	if err != nil || *EffectiveMax(active, FoodResourceKey) != 300 {
		t.Fatalf("Active again: %v, %v", EffectiveMax(active, FoodResourceKey), err)
	}
}

func TestCompletionEffectsFillTheirOwnStorage(t *testing.T) {
	state := granaryExample()
	state.Projects[0].CompletionEffects = Effects{Resources: map[string]int64{FoodResourceKey: 150}}
	result := mustAdvance(t, state, 1)
	if food(result.After) != 245 || result.Finishes[0].Applied.Resources[FoodResourceKey] != 150 {
		t.Fatalf("food %d, completion applied %+v; want 95 + 150 under the Granary's own 300", food(result.After), result.Finishes[0].Applied)
	}
}

func TestEffectiveMaxAddsActiveBonusesAndSaturates(t *testing.T) {
	state := colony(0, 10, unassignedOnly(1), nil)
	state.Buildings = []StandingBuilding{
		{BuildingID: buildingID(1), Active: true, Output: map[string]ResourceOutput{"morale": {StorageBonus: MaxStored}, "gold": {StorageBonus: 5}}},
		{BuildingID: buildingID(2), Active: true, Output: map[string]ResourceOutput{"morale": {StorageBonus: MaxStored}}},
		{BuildingID: buildingID(3), Output: map[string]ResourceOutput{"materials": {StorageBonus: 5}}},
	}
	if got := EffectiveMax(state, "morale"); got == nil || *got != MaxStored {
		t.Fatalf("morale cap %v, want the storable maximum", got)
	}
	if EffectiveMax(state, "gold") != nil || EffectiveMax(state, "missing") != nil {
		t.Fatal("a resource with no max of its own has no effective max")
	}
	if StorageBonus(state, "gold") != 5 || StorageBonus(state, "materials") != 0 {
		t.Fatalf("bonuses: gold %d, materials %d (Idle)", StorageBonus(state, "gold"), StorageBonus(state, "materials"))
	}
}

func TestGainsNeverLowerAnAmountAboveItsCap(t *testing.T) {
	state := colony(0, 10, unassignedOnly(1), nil)
	state.Resources["morale"] = Resource{Label: "Morale", Amount: 140, MaxAmount: pointer(int64(100))}
	for _, mode := range []Mode{Clamp, Strict} {
		result, err := ApplyEffects(state, Effects{Resources: map[string]int64{"morale": 10}}, mode)
		if err != nil {
			t.Fatal(err)
		}
		want := []ResourceChange{{Key: "morale", Before: 140, After: 140, Requested: 10, Clamped: true}}
		if !reflect.DeepEqual(result.Changes, want) || len(result.Applied.Resources) != 0 {
			t.Fatalf("mode %d: changes %+v, applied %+v", mode, result.Changes, result.Applied)
		}
	}
	lowered, err := ApplyEffects(state, Effects{Resources: map[string]int64{"morale": -20}}, Clamp)
	if err != nil || lowered.After.Resources["morale"].Amount != 120 {
		t.Fatalf("a loss above the cap applies in full: %+v, %v", lowered.After.Resources["morale"], err)
	}
	state.Buildings = []StandingBuilding{{BuildingID: buildingID(1), Active: true, Output: map[string]ResourceOutput{"morale": {StorageBonus: 50}}}}
	raised, err := ApplyEffects(state, Effects{Resources: map[string]int64{"morale": 30}}, Clamp)
	if err != nil || raised.After.Resources["morale"].Amount != 150 {
		t.Fatalf("a gain caps at the effective max of 150: %+v, %v", raised.After.Resources["morale"], err)
	}
}

func TestMarketGrantsAndDepositsUseTheEffectiveMax(t *testing.T) {
	state := colony(0, 10, unassignedOnly(1), nil)
	state.Resources[GoldResourceKey] = Resource{Label: "Gold", Amount: 90, MaxAmount: pointer(int64(100))}
	state.Buildings = []StandingBuilding{{BuildingID: buildingID(1), Active: true, Output: map[string]ResourceOutput{
		GoldResourceKey: {StorageBonus: 20}, FoodResourceKey: {StorageBonus: 5},
	}}}
	state.Resources[FoodResourceKey] = Resource{Label: "Food", Amount: 10, MaxAmount: pointer(int64(10))}
	unloaded, err := Unload(state, []PurchaseLine{{UnitPrice: 5, Quantity: 2, Resources: map[string]int64{FoodResourceKey: 4}}})
	if err != nil || unloaded.After.Resources[FoodResourceKey].Amount != 15 || unloaded.After.Resources[GoldResourceKey].Amount != 80 {
		t.Fatalf("unload: %+v, %v", unloaded.After.Resources, err)
	}
	if _, err := Deposit(state, 30); err != nil {
		t.Fatalf("a deposit within the effective max of 120: %v", err)
	}
	_, err = Deposit(state, 31)
	assertInvalid(t, err, "gold", "Colony gold would pass its max of 120. Raise the max on Settings, or dismiss the deposit to return the coins")
}

func TestAdvanceRefusesImpossibleOutput(t *testing.T) {
	state := colony(0, 10, unassignedOnly(1), nil)
	state.Buildings = []StandingBuilding{{BuildingID: buildingID(1), Active: true, Output: map[string]ResourceOutput{"gold": {StorageBonus: -1}}}}
	if _, err := Advance(state, 1); err == nil {
		t.Fatal("a negative storage bonus must be refused")
	}
	aboveTheColumn := colony(0, 10, unassignedOnly(1), nil)
	aboveTheColumn.Resources["gold"] = Resource{Label: "Gold", Amount: MaxStored + 1}
	if _, err := Advance(aboveTheColumn, 1); err == nil {
		t.Fatal("an amount past what the column stores must be refused")
	}
}

func TestPlacingAndRemovingBuildingsMovesTheCap(t *testing.T) {
	state := granaryExample()
	placed := PlaceBuilding(state, buildingID(5), map[string]ResourceOutput{FoodResourceKey: {StorageBonus: 50, DailyDelta: 1}})
	if *EffectiveMax(placed, FoodResourceKey) != 150 || food(placed) != 100 || len(state.Buildings) != 1 {
		t.Fatalf("placed: cap %d, food %d; the input has %d buildings", *EffectiveMax(placed, FoodResourceKey), food(placed), len(state.Buildings))
	}
	if got := EffectiveMaxWith(placed, FoodResourceKey, pointer(int64(20))); got == nil || *got != 70 {
		t.Fatalf("a proposed own max of 20 = %v, want 70", got)
	}
	if EffectiveMaxWith(placed, FoodResourceKey, nil) != nil {
		t.Fatal("no own max means no effective max")
	}
	removed := WithoutBuilding(WithoutBuilding(placed, buildingID(5)), buildingID(1))
	if *EffectiveMax(removed, FoodResourceKey) != 100 || len(removed.Projects) != 0 || len(removed.Buildings) != 1 {
		t.Fatalf("removed: %+v", removed)
	}
}
