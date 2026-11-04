package game

import (
	"errors"
	"reflect"
	"testing"
)

func marketColony() State {
	state := colony(10, 20, map[string]Role{
		UnassignedRoleKey: {Label: "Unassigned", UnnamedCount: 2},
		"FARMER":          {Label: "Farmer", UnnamedCount: 3},
		"MAGE":            {Label: "Mage", Archived: true},
	}, nil)
	state.Resources["gold"] = Resource{Label: "Gold", Amount: 100}
	return state
}

func TestOrderTotalAtOneGoldForTwoFood(t *testing.T) {
	food := PurchaseLine{UnitPrice: 1, Quantity: 10, Resources: map[string]int64{"food": 2}}
	farmer := PurchaseLine{UnitPrice: 30, Quantity: 1, Colonists: 1, RoleKey: "FARMER"}
	total, err := OrderTotal([]PurchaseLine{food, farmer})
	if err != nil || total != 40 {
		t.Fatalf("total %d, %v; want 40", total, err)
	}
	_, err = OrderTotal([]PurchaseLine{{UnitPrice: MaxStored, Quantity: 2}})
	assertInvalid(t, err, "cart", MessageOrderTooCostly)
	_, err = OrderTotal([]PurchaseLine{{UnitPrice: MaxStored, Quantity: 1}, {UnitPrice: 1, Quantity: 1}})
	assertInvalid(t, err, "cart", MessageOrderTooCostly)
}

func TestArrivesDay(t *testing.T) {
	if day, err := ArrivesDay(22, 14); err != nil || day != 36 {
		t.Fatalf("arrives %d, %v; want 36", day, err)
	}
	_, err := ArrivesDay(MaxStored-3, 4)
	assertInvalid(t, err, "voyageDays", MessageTooManyDays)
	_, err = ArrivesDay(5, 0)
	assertInvalid(t, err, "voyageDays", "A voyage takes at least 1 day")
}

func TestDocksDuringBoundaries(t *testing.T) {
	for _, testCase := range []struct {
		from, to, arrives int64
		want              bool
	}{
		{from: 30, to: 36, arrives: 36, want: true},
		{from: 36, to: 40, arrives: 36, want: false},
		{from: 30, to: 35, arrives: 36, want: false},
		{from: 30, to: 40, arrives: 31, want: true},
	} {
		if got := DocksDuring(testCase.from, testCase.to, testCase.arrives); got != testCase.want {
			t.Errorf("DocksDuring(%d, %d, %d) = %v, want %v", testCase.from, testCase.to, testCase.arrives, got, testCase.want)
		}
	}
	if Docked(35, 36) || !Docked(36, 36) {
		t.Fatal("an order docks on its arrival day")
	}
}

func TestUnloadPaysAndLandsColonistsInTheirRoles(t *testing.T) {
	state := marketColony()
	state.Resources["morale"] = Resource{Label: "Morale", Amount: 99, MaxAmount: pointer(int64(100))}
	result, err := Unload(state, []PurchaseLine{
		{UnitPrice: 1, Quantity: 10, Resources: map[string]int64{"food": 2}},
		{UnitPrice: 30, Quantity: 2, Colonists: 1, RoleKey: "FARMER"},
		{UnitPrice: 5, Quantity: 1, Colonists: 3},
		{UnitPrice: 10, Quantity: 1, Resources: map[string]int64{"morale": 5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := result.After
	if after.Resources["gold"].Amount != 15 || food(after) != 40 || after.Resources["morale"].Amount != 100 {
		t.Fatalf("gold %d, food %d, morale %d; want 15, 40, 100", after.Resources["gold"].Amount, food(after), after.Resources["morale"].Amount)
	}
	if after.Roles["FARMER"].UnnamedCount != 5 || after.Roles[UnassignedRoleKey].UnnamedCount != 5 {
		t.Fatalf("roles after: %+v", after.Roles)
	}
	if want := map[string]int64{"FARMER": 2, UnassignedRoleKey: 3}; !reflect.DeepEqual(result.Colonists, want) {
		t.Fatalf("colonists %+v, want %+v", result.Colonists, want)
	}
	if want := map[string]int64{"gold": -85, "food": 20, "morale": 1}; !reflect.DeepEqual(result.Applied.Resources, want) {
		t.Fatalf("applied %+v, want %+v", result.Applied.Resources, want)
	}
	if state.Resources["gold"].Amount != 100 || state.Roles["FARMER"].UnnamedCount != 3 {
		t.Fatal("Unload changed its input")
	}
}

func TestUnloadRefuses(t *testing.T) {
	state := marketColony()
	_, err := Unload(state, []PurchaseLine{{UnitPrice: 65, Quantity: 2}})
	var shortfallError *ShortfallError
	if !errors.As(err, &shortfallError) || shortfallError.Error() != "Not enough gold: the colony has 100, needs 130" {
		t.Fatalf("shortfall: %v", err)
	}
	_, err = Unload(state, []PurchaseLine{{UnitPrice: 1, Quantity: 1, Colonists: 1, RoleKey: "MAGE"}})
	assertInvalid(t, err, "roleKey", "Mage is archived")
	_, err = Unload(state, []PurchaseLine{{UnitPrice: 1, Quantity: 1, Resources: map[string]int64{"spice": 1}}})
	assertInvalid(t, err, "resources", `There is no resource "spice"`)
	full := marketColony()
	full.Roles["FARMER"] = Role{Label: "Farmer", UnnamedCount: MaxStored}
	_, err = Unload(full, []PurchaseLine{{UnitPrice: 1, Quantity: 1, Colonists: 1, RoleKey: "FARMER"}})
	assertInvalid(t, err, "roleKey", MessageTooManyColonists)
	noGold := marketColony()
	delete(noGold.Resources, "gold")
	_, err = Unload(noGold, []PurchaseLine{{UnitPrice: 0, Quantity: 1}})
	assertInvalid(t, err, "resources", `There is no resource "gold"`)
}
