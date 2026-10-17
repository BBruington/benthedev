package game

import "testing"

func workforce() State {
	return colony(10, 100, map[string]Role{
		UnassignedRoleKey: {Label: "Unassigned", UnnamedCount: 5},
		"FARMER":          {Label: "Farmer", UnnamedCount: 12, NamedCount: 1},
		"HEALER":          {Label: "Healer", UnnamedCount: 0},
		"MAGE":            {Label: "Mage", Archived: true},
	}, nil)
}

func TestWorkforceRulesKeepTheTotalUnlessSomeoneJoinsOrLeaves(t *testing.T) {
	state := workforce()
	before := Colonists(state.Roles)
	cases := map[string]struct {
		change      func(State) (State, error)
		totalChange int64
	}{
		"move 3 from Unassigned to Farmer": {func(state State) (State, error) { return Transfer(state, UnassignedRoleKey, "FARMER", 3) }, 0},
		"set Healer to 4":                  {func(state State) (State, error) { return SetUnnamedCount(state, "HEALER", 4) }, 4},
		"join as a new colonist":           {func(state State) (State, error) { return JoinColony(state, "HEALER", false) }, 1},
		"promote an existing Farmer":       {func(state State) (State, error) { return JoinColony(state, "FARMER", true) }, 0},
		"change role":                      {func(state State) (State, error) { return ChangeRole(state, "FARMER", "HEALER") }, 0},
		"leave the colony":                 {func(state State) (State, error) { return LeaveColony(state, "FARMER", false) }, -1},
		"return to the unnamed pool":       {func(state State) (State, error) { return LeaveColony(state, "FARMER", true) }, 0},
	}
	for name, testCase := range cases {
		after, err := testCase.change(state)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := Colonists(after.Roles) - before; got != testCase.totalChange {
			t.Errorf("%s: total changed by %d, want %d", name, got, testCase.totalChange)
		}
		if Colonists(state.Roles) != before {
			t.Fatalf("%s changed its input", name)
		}
	}
	moved, _ := Transfer(state, UnassignedRoleKey, "FARMER", 3)
	if moved.Roles[UnassignedRoleKey].UnnamedCount != 2 || moved.Roles["FARMER"].UnnamedCount != 15 {
		t.Fatalf("after the transfer: %+v", moved.Roles)
	}
	promoted, _ := JoinColony(state, "FARMER", true)
	if farmer := promoted.Roles["FARMER"]; farmer.UnnamedCount != 11 || farmer.NamedCount != 2 {
		t.Fatalf("after the promotion: %+v", farmer)
	}
}

func TestWorkforceRulesRefuse(t *testing.T) {
	state := workforce()
	_, err := Transfer(state, UnassignedRoleKey, "FARMER", 6)
	assertInvalid(t, err, "count", "Unassigned has only 5 unnamed colonists, so 6 can't move")
	_, err = Transfer(state, UnassignedRoleKey, "FARMER", 0)
	assertInvalid(t, err, "count", "Move at least 1 colonist")
	_, err = Transfer(state, "FARMER", "FARMER", 1)
	assertInvalid(t, err, "toRoleKey", "Pick a different role to move them to")
	_, err = Transfer(state, UnassignedRoleKey, "MAGE", 1)
	assertInvalid(t, err, "toRoleKey", "Mage is archived")
	_, err = Transfer(state, "SAILOR", "FARMER", 1)
	assertInvalid(t, err, "fromRoleKey", `There is no role "SAILOR"`)
	_, err = SetUnnamedCount(state, "FARMER", 12)
	assertInvalid(t, err, "count", "Farmer already has 12 unnamed colonists")
	_, err = SetUnnamedCount(state, "FARMER", -1)
	assertInvalid(t, err, "count", MessageNegativeColonist)
	_, err = SetUnnamedCount(state, "FARMER", MaxStored+1)
	assertInvalid(t, err, "count", MessageTooManyColonists)
	_, err = JoinColony(state, "HEALER", true)
	assertInvalid(t, err, "mode", "Healer has no unnamed colonists to name")
	_, err = JoinColony(state, "MAGE", false)
	assertInvalid(t, err, "roleKey", "Mage is archived")
	_, err = ChangeRole(state, "FARMER", "FARMER")
	assertInvalid(t, err, "roleKey", "They are already in that role")
	_, err = ChangeRole(state, "HEALER", "FARMER")
	assertInvalid(t, err, "roleKey", "They aren't counted in their role")
	_, err = LeaveColony(state, "HEALER", false)
	assertInvalid(t, err, "mode", "They aren't counted in their role")
}
