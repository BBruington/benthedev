package game

import (
	"maps"
	"math"
	"slices"

	"github.com/google/uuid"
)

// The output rules (docs/design/construction.md §4): every Active standing building adds its
// blueprint's daily output each day and its storage bonuses to the caps. A cap that falls never takes
// stock away (K13): an amount above its effective max stays, gains add nothing until it is under, and
// losses apply as usual.

// ResourceOutput is what a building does for one resource while it stands Active: DailyDelta each day
// (positive is production, negative is upkeep) and StorageBonus on the resource's max.
type ResourceOutput struct {
	DailyDelta   int64
	StorageBonus int64
}

// StandingBuilding is one finished building as the rules see it. Output is its blueprint's live
// output (K18), so a rebalanced blueprint changes every standing building from the next day.
type StandingBuilding struct {
	BuildingID uuid.UUID
	// Active is false while the DM has it Idle: no output and no storage bonus (K12).
	Active bool
	Output map[string]ResourceOutput
}

// StorageBonus is what every Active standing building adds to a resource's max, saturating at
// MaxStored.
func StorageBonus(state State, key string) int64 {
	var bonus int64
	for _, building := range state.Buildings {
		if building.Active {
			bonus = min(bonus+building.Output[key].StorageBonus, MaxStored)
		}
	}
	return bonus
}

// EffectiveMax is a resource's max as every rule that caps reads it: its own max plus every Active
// standing building's storage bonus, saturating at MaxStored. It is nil when the resource has no max
// of its own, or no such resource exists.
func EffectiveMax(state State, key string) *int64 {
	resource, found := state.Resources[key]
	if !found {
		return nil
	}
	return EffectiveMaxWith(state, key, resource.MaxAmount)
}

// EffectiveMaxWith is a resource's effective max were its own max ownMax, which is what /settings
// checks a new max against; nil when ownMax is.
func EffectiveMaxWith(state State, key string, ownMax *int64) *int64 {
	if ownMax == nil {
		return nil
	}
	effective := min(*ownMax+StorageBonus(state, key), MaxStored)
	return &effective
}

// ceiling is the highest amount a gain may reach: the effective max, or what the column stores.
func ceiling(state State, key string) int64 {
	if effective := EffectiveMax(state, key); effective != nil {
		return *effective
	}
	return MaxStored
}

// clampGain is amount + delta within [minimum, the ceiling], except that a gain never lowers an
// amount already above the ceiling (K13).
func clampGain(amount int64, delta int64, minimum int64, upperBound int64) int64 {
	return min(max(amount+delta, minimum), max(upperBound, amount))
}

// BuildingOutput is the daily step that runs first (K14): every resource takes the sum of every Active
// standing building's daily delta once a day, CLAMP with the never-lower rule. Food's output is applied
// by Upkeep instead, because food's output and its eating meet in one day: neither span may run ahead
// of the other.
type BuildingOutput struct{}

func (output BuildingOutput) Span(state *State, from int64, to int64, log *Log) {
	days := to - from
	if days <= 0 {
		return
	}
	outputs := dailyOutputs(*state)
	for _, key := range slices.Sorted(maps.Keys(outputs)) {
		if key == FoodResourceKey {
			continue
		}
		resource := state.Resources[key]
		log.RecordRequested(key, saturatingProduct(outputs[key], days))
		newAmount := amountAfterDays(resource.Amount, outputs[key], 0, resource.MinAmount, ceiling(*state, key), days)
		log.RecordResourceDelta(key, newAmount-resource.Amount)
		resource.Amount = newAmount
		state.Resources[key] = resource
	}
}

// Output changes only stock, which no other step reads but upkeep (food, which upkeep applies itself),
// and a finish's completion Effects, which run at a span's end.
func (output BuildingOutput) NextBoundary(State, int64) (int64, bool) {
	return 0, false
}

// dailyOutputs is each resource's net daily change from the Active standing buildings, for every
// unarchived resource it is non-zero for. No day's change can move an amount further than the
// storable range, so the sum saturates there.
func dailyOutputs(state State) map[string]int64 {
	outputs := map[string]int64{}
	for _, building := range state.Buildings {
		if !building.Active {
			continue
		}
		for key, output := range building.Output {
			if resource, found := state.Resources[key]; !found || resource.Archived || output.DailyDelta == 0 {
				continue
			}
			outputs[key] = min(max(outputs[key]+output.DailyDelta, MinStored-MaxStored), MaxStored-MinStored)
		}
	}
	maps.DeleteFunc(outputs, func(_ string, delta int64) bool { return delta == 0 })
	return outputs
}

// amountAfterDays runs one resource's day `days` times in closed form: the day's output (CLAMP, never
// lowering an amount above the ceiling), then `eaten` taken away, floored at the minimum. Each part
// is monotone, so the amount moves in at most three phases: down while at or above the ceiling, then
// up or down by the day's net until the ceiling or the floor engages, then a fixed point. amount
// starts at or above minimum, and ceiling is never below minimum.
func amountAfterDays(amount int64, output int64, eaten int64, minimum int64, upperBound int64, days int64) int64 {
	if days <= 0 {
		return amount
	}
	if output <= 0 {
		return declined(amount, eaten-output, minimum, days)
	}
	if amount >= upperBound {
		if eaten == 0 {
			return amount
		}
		// The first day the amount is under the ceiling again, before the floor.
		daysAtCeiling := (amount-upperBound)/eaten + 1
		if minimum >= upperBound || days <= daysAtCeiling {
			return declined(amount, eaten, minimum, days)
		}
		amount = max(amount-daysAtCeiling*eaten, minimum)
		days -= daysAtCeiling
	}
	// Under the ceiling: a day whose output reaches it ends at plateau, and plateau's next day does too
	// unless the colony eats more than its output.
	plateau := max(upperBound-eaten, minimum)
	net := output - eaten
	switch {
	case amount+output >= upperBound:
		if days == 1 || net >= 0 {
			return plateau
		}
		return declined(plateau, -net, minimum, days-1)
	case net > 0:
		growingDays := (upperBound - output - amount + net - 1) / net
		if days <= growingDays {
			return amount + days*net
		}
		return plateau
	case net == 0:
		return amount
	default:
		return declined(amount, -net, minimum, days)
	}
}

// declined is amount after losing drop a day for days days, floored at minimum. Dividing first keeps
// drop × days from overflowing int64 for any N.
func declined(amount int64, drop int64, minimum int64, days int64) int64 {
	spare := amount - minimum
	if drop <= 0 || spare <= 0 {
		return amount
	}
	if days <= spare/drop {
		return amount - days*drop
	}
	return minimum
}

// saturatingProduct is perDay × days, saturating rather than overflowing for a huge N.
func saturatingProduct(perDay int64, days int64) int64 {
	switch {
	case perDay > 0 && days > math.MaxInt64/perDay:
		return math.MaxInt64
	case perDay < 0 && days > math.MaxInt64/-perDay:
		return -math.MaxInt64
	}
	return perDay * days
}

// standUp makes a finished project a standing building, Active, with its blueprint's output.
func standUp(state *State, project Project) {
	state.Buildings = append(state.Buildings, StandingBuilding{BuildingID: project.BuildingID, Active: true, Output: maps.Clone(project.Output)})
	slices.SortFunc(state.Buildings, func(left, right StandingBuilding) int { return compareUUIDs(left.BuildingID, right.BuildingID) })
}

// PlaceBuilding is the State with a building the DM added standing at once, Active (K19): its storage
// bonus counts now and its output from the next day. It costs nothing and applies no completion Effects.
func PlaceBuilding(state State, buildingID uuid.UUID, output map[string]ResourceOutput) State {
	after := state.Clone()
	standUp(&after, Project{BuildingID: buildingID, Output: output})
	return after
}

// WithoutBuilding is the State once a building is gone, cancelled or demolished: its project or its
// standing building leaves, and with it its output and storage bonus, so a refund caps without them.
func WithoutBuilding(state State, buildingID uuid.UUID) State {
	after := state.Clone()
	after.Projects = slices.DeleteFunc(after.Projects, func(project Project) bool { return project.BuildingID == buildingID })
	after.Buildings = slices.DeleteFunc(after.Buildings, func(building StandingBuilding) bool { return building.BuildingID == buildingID })
	return after
}

// SetBuildingActive switches a standing building Idle or Active (K12). Stock is untouched: an amount
// above a cap that fell stays (K13).
func SetBuildingActive(state State, buildingID uuid.UUID, active bool) (State, error) {
	index := slices.IndexFunc(state.Buildings, func(building StandingBuilding) bool { return building.BuildingID == buildingID })
	if index < 0 {
		return State{}, invalid("buildingId", "That building isn't standing")
	}
	if state.Buildings[index].Active == active {
		if active {
			return State{}, invalid("active", "It is already Active")
		}
		return State{}, invalid("active", "It is already Idle")
	}
	after := state.Clone()
	after.Buildings[index].Active = active
	return after, nil
}

func cloneBuildings(buildings []StandingBuilding) []StandingBuilding {
	if buildings == nil {
		return nil
	}
	clones := make([]StandingBuilding, len(buildings))
	for index, building := range buildings {
		clones[index] = StandingBuilding{BuildingID: building.BuildingID, Active: building.Active, Output: maps.Clone(building.Output)}
	}
	return clones
}

// checkStoredOutputs reports outputs the database could never hold.
func checkStoredOutputs(outputs map[string]ResourceOutput) bool {
	for _, output := range outputs {
		if output.DailyDelta < MinStored || output.DailyDelta > MaxStored || output.StorageBonus < 0 || output.StorageBonus > MaxStored {
			return false
		}
	}
	return true
}
