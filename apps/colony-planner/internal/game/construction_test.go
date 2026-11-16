package game

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func buildingID(number byte) uuid.UUID {
	return uuid.UUID{15: number}
}

// constructionExample is the brief's worked example: Day 40, food 12 (min 0), 5 colonists, 4 Builders,
// and a building needing 20 Builder-days (max crew 4) started on Day 40 with crew 4.
func constructionExample() State {
	state := colony(40, 12, map[string]Role{
		"BUILDER":         {Label: "Builder", UnnamedCount: 4},
		UnassignedRoleKey: {Label: "Unassigned", UnnamedCount: 1},
	}, nil)
	state.Projects = []Project{{
		BuildingID: buildingID(1),
		Lines:      []LaborLine{{RoleKey: "BUILDER", Effort: 20, MaxCrew: pointer(int64(4)), Crew: 4}},
	}}
	return state
}

func TestConstructionWorkedExample(t *testing.T) {
	state := constructionExample()
	result := mustAdvance(t, state, 7)
	wantFinishes := []BuildingFinish{{
		BuildingID: buildingID(1),
		Day:        45,
		Applied:    Effects{Resources: map[string]int64{}},
		Skipped:    []string{},
		Lines:      []LaborLine{{RoleKey: "BUILDER", Effort: 20, MaxCrew: pointer(int64(4)), Progress: 20}},
	}}
	if !reflect.DeepEqual(result.Finishes, wantFinishes) {
		t.Fatalf("finishes %+v, want %+v", result.Finishes, wantFinishes)
	}
	if len(result.After.Projects) != 0 || len(Crews(result.After.Projects)) != 0 {
		t.Fatalf("after the finish the Builders are still crewed: %+v", result.After.Projects)
	}
	if food(result.After) != 0 || result.Applied.Resources[FoodResourceKey] != -12 {
		t.Fatalf("food %d (applied %d), want 0 on Day 47 with no output in Phase 1", food(result.After), result.Applied.Resources[FoodResourceKey])
	}
	wantFood := map[int64]int64{41: 7, 42: 2, 43: 0, 45: 0, 47: 0}
	for day, want := range wantFood {
		if got := food(mustAdvance(t, state, day-state.Day).After); got != want {
			t.Errorf("food on Day %d = %d, want %d", day, got, want)
		}
	}
	fourDays := mustAdvance(t, state, 4).After
	if fourDays.Projects[0].Lines[0].Progress != 16 || fourDays.Projects[0].Lines[0].Crew != 4 {
		t.Fatalf("Day 44: %+v, want 16 of 20 with crew 4", fourDays.Projects[0].Lines[0])
	}
	if finish := ProjectedFinishDay(fourDays.Day, fourDays.Projects[0]); finish == nil || *finish != 45 {
		t.Fatalf("projected finish from Day 44 = %v, want Day 45", finish)
	}
}

func TestEachLineFreesItsCrewOnItsOwnFinishDay(t *testing.T) {
	state := projectsThatFeedUpkeep()
	// The Mage line (7 at 2 a day) finishes on Day 7, the Builder line (13 at 3 a day) on Day 8.
	day7 := mustAdvance(t, state, 4)
	if len(day7.Finishes) != 0 {
		t.Fatalf("nothing finishes by Day 7: %+v", day7.Finishes)
	}
	lines := day7.After.Projects[0].Lines
	if lines[1].Progress != 7 || lines[1].Crew != 0 || lines[0].Progress != 12 || lines[0].Crew != 3 {
		t.Fatalf("Day 7 lines = %+v", lines)
	}
	day8 := mustAdvance(t, state, 5)
	if len(day8.Finishes) != 1 || day8.Finishes[0].Day != 8 || day8.Finishes[0].BuildingID != buildingID(1) {
		t.Fatalf("Day 8 finishes = %+v", day8.Finishes)
	}
	finish := day8.Finishes[0]
	if !reflect.DeepEqual(finish.Applied, Effects{Resources: map[string]int64{"morale": -10}, Colonists: 4}) {
		t.Fatalf("completion applied %+v", finish.Applied)
	}
	// The four colonists land in Unassigned after Day 8's upkeep, so Day 9 feeds 14, not 10.
	day9 := mustAdvance(t, state, 6)
	if food(day8.After) != 10 || food(day9.After) != 0 || day9.After.Roles[UnassignedRoleKey].UnnamedCount != 7 {
		t.Fatalf("food %d on Day 8 and %d on Day 9, Unassigned %d", food(day8.After), food(day9.After), day9.After.Roles[UnassignedRoleKey].UnnamedCount)
	}
	stalled := day9.After.Projects[len(day9.After.Projects)-1]
	if stalled.BuildingID != buildingID(3) || ProjectedFinishDay(day9.After.Day, stalled) != nil {
		t.Fatalf("a project with an uncrewed line has no finish day: %+v", stalled)
	}
}

func TestCompletionEffectsClampAndSkipArchivedResources(t *testing.T) {
	state := constructionExample()
	state.Resources["timber"] = Resource{Label: "Timber", Archived: true}
	state.Projects[0].CompletionEffects = Effects{Resources: map[string]int64{"morale": 5, "gold": MaxStored, "timber": 10}}
	result := mustAdvance(t, state, 5)
	finish := result.Finishes[0]
	if !reflect.DeepEqual(finish.Applied, Effects{Resources: map[string]int64{"gold": MaxStored - 50}}) || !reflect.DeepEqual(finish.Skipped, []string{"timber"}) {
		t.Fatalf("applied %+v, skipped %v; want morale capped at 100, gold at the storable maximum, timber skipped", finish.Applied, finish.Skipped)
	}
	if _, found := result.Applied.Resources["gold"]; found {
		t.Fatalf("completion Effects leaked into the advance's own deltas: %+v", result.Applied)
	}
}

func TestStartProjectTakesTheCostStrict(t *testing.T) {
	state := constructionExample()
	state.Projects = nil
	project := Project{BuildingID: buildingID(9), Lines: []LaborLine{{RoleKey: "BUILDER", Effort: 40, MaxCrew: pointer(int64(4))}}}

	_, err := StartProject(state, project, Effects{Resources: map[string]int64{"materials": -60, "gold": -20}}, map[string]int64{"BUILDER": 2})
	var shortfallError *ShortfallError
	if !errors.As(err, &shortfallError) || err.Error() != "Not enough materials: the colony has 30, needs 60" {
		t.Fatalf("a short cost: %v", err)
	}
	result, err := StartProject(state, project, Effects{Resources: map[string]int64{"materials": -30, "gold": -20}}, map[string]int64{"BUILDER": 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.After.Resources["materials"].Amount != 0 || result.After.Resources["gold"].Amount != 30 ||
		len(result.After.Projects) != 1 || result.After.Projects[0].Lines[0].Crew != 2 {
		t.Fatalf("after Start: %+v", result.After)
	}
	if !reflect.DeepEqual(result.Applied, Effects{Resources: map[string]int64{"materials": -30, "gold": -20}}) {
		t.Fatalf("applied %+v", result.Applied)
	}

	_, err = StartProject(state, project, Effects{}, map[string]int64{"BUILDER": 5})
	assertInvalid(t, err, "crews", "At most 4 Builders can work on this")
	state.Roles["MAGE"] = Role{Label: "Mage", Archived: true}
	_, err = StartProject(state, Project{BuildingID: buildingID(9), Lines: []LaborLine{{RoleKey: "MAGE", Effort: 10}}}, Effects{}, nil)
	assertInvalid(t, err, "blueprintId", "Mage is archived")
}

func TestSetCrewsGuards(t *testing.T) {
	state := projectsThatFeedUpkeep()
	cases := []struct {
		name    string
		project byte
		crews   map[string]int64
		message string
	}{
		{"past the free Builders", 3, map[string]int64{"BUILDER": 1}, "No Builders are free"},
		{"past the free Mages", 3, map[string]int64{"MAGE": 1}, "No Mages are free"},
		{"past the max crew", 1, map[string]int64{"MAGE": 3}, "At most 2 Mages can work on this"},
		{"a role the project doesn't need", 2, map[string]int64{"MAGE": 1}, "This project needs no Mage work"},
		{"a negative crew", 1, map[string]int64{"BUILDER": -1}, "A crew can't be negative"},
	}
	for _, testCase := range cases {
		_, err := SetCrews(state, buildingID(testCase.project), testCase.crews)
		assertInvalid(t, err, "crews", testCase.message)
	}
	_, err := SetCrews(state, buildingID(7), map[string]int64{})
	assertInvalid(t, err, "crews", "That project isn't under construction")

	// Taking Builders off the first project frees them for the third; the map is the whole record.
	freed, err := SetCrews(state, buildingID(1), map[string]int64{"MAGE": 2, "BUILDER": 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = SetCrews(freed, buildingID(3), map[string]int64{"BUILDER": 3})
	assertInvalid(t, err, "crews", "Only 2 Builders are free")
	moved, err := SetCrews(freed, buildingID(3), map[string]int64{"BUILDER": 2})
	if err != nil {
		t.Fatal(err)
	}
	if crews := Crews(moved.Projects); crews["BUILDER"] != 5 || crews["MAGE"] != 2 {
		t.Fatalf("crews %v", crews)
	}
	cleared, err := SetCrews(moved, buildingID(1), map[string]int64{})
	if err != nil || cleared.Projects[0].Lines[0].Crew != 0 || cleared.Projects[0].Lines[1].Crew != 0 {
		t.Fatalf("a line the map leaves out gets no crew: %+v, %v", cleared.Projects[0], err)
	}
	if state.Projects[0].Lines[0].Crew != 3 {
		t.Fatal("SetCrews changed its input")
	}

	// Once the Mage line is done it takes no crew.
	day7 := mustAdvance(t, state, 4).After
	_, err = SetCrews(day7, buildingID(1), map[string]int64{"MAGE": 1, "BUILDER": 3})
	assertInvalid(t, err, "crews", "Mage work on this project is done")
}

func TestWorkforceChangesNeverStrandCrews(t *testing.T) {
	state := constructionExample()
	state.Roles["BUILDER"] = Role{Label: "Builder", UnnamedCount: 4, NamedCount: 1}
	state.Roles["FARMER"] = Role{Label: "Farmer", UnnamedCount: 2}
	state.Projects[0].Lines[0].MaxCrew = nil
	state.Projects[0].Lines[0].Crew = 5

	_, err := Transfer(state, "BUILDER", "FARMER", 2)
	assertInvalid(t, err, "count", "5 Builders are on projects; take 2 off a project first")
	_, err = SetUnnamedCount(state, "BUILDER", 3)
	assertInvalid(t, err, "count", "5 Builders are on projects; take 1 off a project first")
	_, err = ChangeRole(state, "BUILDER", "FARMER")
	assertInvalid(t, err, "roleKey", "5 Builders are on projects; take 1 off a project first")
	_, err = LeaveColony(state, "BUILDER", false)
	assertInvalid(t, err, "mode", "5 Builders are on projects; take 1 off a project first")

	// Returning to the pool keeps the headcount, and Builders off the project may move.
	if _, err := LeaveColony(state, "BUILDER", true); err != nil {
		t.Fatalf("return to the pool: %v", err)
	}
	state.Projects[0].Lines[0].Crew = 1
	if _, err := Transfer(state, "BUILDER", "FARMER", 4); err != nil {
		t.Fatalf("moving the idle Builders: %v", err)
	}
	single := state.Clone()
	single.Roles["BUILDER"] = Role{Label: "Builder", UnnamedCount: 1}
	_, err = SetUnnamedCount(single, "BUILDER", 0)
	assertInvalid(t, err, "count", "1 Builder is on a project; take 1 off a project first")
}

func TestAdvanceRefusesImpossibleLabor(t *testing.T) {
	state := constructionExample()
	state.Projects[0].Lines[0].Progress = 21
	if _, err := Advance(state, 1); err == nil {
		t.Fatal("progress past the effort must be refused")
	}
}

func TestSetCrewsRefusesACrewPastWhatCanBeStored(t *testing.T) {
	state := colony(1, 0, map[string]Role{
		"BUILDER":         {Label: "Builder", UnnamedCount: MaxStored, NamedCount: 1},
		UnassignedRoleKey: {Label: "Unassigned"},
	}, nil)
	state.Projects = []Project{{BuildingID: buildingID(1), Lines: []LaborLine{{RoleKey: "BUILDER", Effort: 10}}}}
	_, err := SetCrews(state, buildingID(1), map[string]int64{"BUILDER": MaxStored + 1})
	assertInvalid(t, err, "crews", "That crew can't be stored")
}

func TestRequestedCountsUpkeepAfterAFinishAndItsCompletionEffects(t *testing.T) {
	state := constructionExample()
	state.Projects[0].CompletionEffects = Effects{Resources: map[string]int64{"food": 10, "morale": 5}, Colonists: 2}
	result := mustAdvance(t, state, 7)
	// Days 41 to 45 feed 5 colonists, Days 46 and 47 feed 7, and the finish on Day 45 brings 10 food.
	want := map[string]int64{"food": -5*5 - 7*2 + 10, "morale": 5}
	if !reflect.DeepEqual(result.Requested, want) {
		t.Fatalf("requested %v, want %v", result.Requested, want)
	}
}
