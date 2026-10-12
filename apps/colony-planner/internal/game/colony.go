package game

import (
	"fmt"
	"maps"
	"math"
)

// MaxStored is the largest value a Postgres integer column holds. Days, next shipment days, amounts,
// headcounts and favor are stored as integer, so every rule does its arithmetic in int64 and refuses a
// result past this bound instead of letting the database reject it (spec §5.2, the overflow guard).
const MaxStored = math.MaxInt32

// MinStored is the smallest value a Postgres integer column holds.
const MinStored = math.MinInt32

const (
	// FoodResourceKey names the system resource that upkeep consumes.
	FoodResourceKey = "food"
	// UnassignedRoleKey names the system role where every new colonist lands (spec §5.5).
	UnassignedRoleKey = "UNASSIGNED"
	// FoodPerColonistPerDay is the old app's upkeep rule, kept as is (spec §5.2).
	FoodPerColonistPerDay = 1
)

// State is everything the M1 rules read and change. Handlers load it from the colony, resource and
// role rows, call a rule and save what comes back; no rule mutates the State it is given.
type State struct {
	Day int64
	// Schedule is nil when no shipment schedule is set.
	Schedule *Schedule
	// Resources is keyed by resource key. Archived resources are included so upkeep and history
	// stay correct; effects refuse to name them.
	Resources map[string]Resource
	// Roles is keyed by role key and must include UNASSIGNED.
	Roles map[string]Role
}

// Schedule is the shipment cadence (spec §5.3): interval I ≥ 1 and next arrival day D > Day.
type Schedule struct {
	IntervalDays int64
	NextDay      int64
}

// Resource is one stockpile row. MaxAmount is nil when the resource has no upper bound.
type Resource struct {
	// Label names the resource in shortfall messages ("Not enough food: …"); it is lowercased there.
	Label     string
	Amount    int64
	MinAmount int64
	MaxAmount *int64
	Archived  bool
}

// Role is one row of the role table with its headcounts.
type Role struct {
	// Label names the role in the workforce rules' messages ("Farmer has only 2 unnamed colonists").
	Label        string
	UnnamedCount int64
	// NamedCount is the number of named NPC colonists in this role; the handler fills it from
	// `npc.colony_role_key`.
	NamedCount int64
	Archived   bool
}

// Colonists is the colony's population, which is never stored: the sum over every role, UNASSIGNED
// included, of its unnamed headcount plus its named NPC colonists (spec §5.5). Archived roles hold 0.
func Colonists(roles map[string]Role) int64 {
	var total int64
	for _, role := range roles {
		total += role.UnnamedCount + role.NamedCount
	}
	return total
}

// Clone returns a deep copy, so a rule can change it without touching its caller's maps.
func (state State) Clone() State {
	clone := state
	if state.Schedule != nil {
		schedule := *state.Schedule
		clone.Schedule = &schedule
	}
	clone.Resources = make(map[string]Resource, len(state.Resources))
	for key, resource := range state.Resources {
		if resource.MaxAmount != nil {
			maxAmount := *resource.MaxAmount
			resource.MaxAmount = &maxAmount
		}
		clone.Resources[key] = resource
	}
	clone.Roles = maps.Clone(state.Roles)
	if clone.Roles == nil {
		clone.Roles = map[string]Role{}
	}
	return clone
}

// checkStoredState reports a State the database could never hold. It guards the closed forms, which
// would otherwise count arrivals that are already in the past.
func checkStoredState(state State) error {
	if state.Day < 0 || state.Day > MaxStored {
		return fmt.Errorf("game: colony day %d is out of range", state.Day)
	}
	if state.Schedule != nil {
		if state.Schedule.IntervalDays < 1 {
			return fmt.Errorf("game: shipment interval %d is below 1", state.Schedule.IntervalDays)
		}
		if state.Schedule.NextDay <= state.Day {
			return fmt.Errorf("game: next shipment day %d is not after day %d", state.Schedule.NextDay, state.Day)
		}
	}
	for key, resource := range state.Resources {
		if resource.Amount < resource.MinAmount || (resource.MaxAmount != nil && resource.Amount > *resource.MaxAmount) {
			return fmt.Errorf("game: resource %q amount %d is outside its bounds", key, resource.Amount)
		}
	}
	for key, role := range state.Roles {
		if role.UnnamedCount < 0 || role.NamedCount < 0 {
			return fmt.Errorf("game: role %q has a negative headcount", key)
		}
	}
	return nil
}
