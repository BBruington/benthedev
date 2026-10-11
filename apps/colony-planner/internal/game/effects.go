package game

import (
	"maps"
	"slices"
	"strings"
)

// Effects is the one shape for every change to colony numbers (spec §4.2): quest rewards, action
// effects, cargo, adjustments and the applied deltas a timeline entry records.
type Effects struct {
	// Resources maps resource keys to deltas. Keys must name existing, unarchived resources.
	Resources map[string]int64 `json:"resources,omitempty"`
	// Colonists is ≥ 0 and always lands in UNASSIGNED. Removals go through the workforce (spec §5.5).
	Colonists int64 `json:"colonists,omitempty"`
}

// Mode says how resource deltas meet their bounds (spec §5.4).
type Mode int

const (
	// Clamp is for quest rewards, DM adjustments and cargo: each resource becomes
	// clamp(old + Δ, min, max), and only the applied delta is logged.
	Clamp Mode = iota + 1
	// Strict is for what a player's request spends: a negative delta that would take a resource below
	// its minimum rejects the whole operation with the shortfall named. Positive deltas still cap.
	Strict
)

// ResourceChange is one resource an Effects named, for the preview ("Morale 98 → 100 (capped)").
type ResourceChange struct {
	Key       string
	Before    int64
	After     int64
	Requested int64
	// Clamped is true when a bound cut the requested delta.
	Clamped bool
}

// Applied is the delta that took effect.
func (change ResourceChange) Applied() int64 {
	return change.After - change.Before
}

// EffectsResult is the State to save after applying Effects, with what to log.
type EffectsResult struct {
	Before State
	After  State
	// Changes lists every resource the Effects named with a non-zero delta, sorted by key.
	Changes []ResourceChange
	// Applied is what the timeline entry's deltas record: applied resource deltas only (zero ones
	// omitted) and the colonists added to UNASSIGNED.
	Applied Effects
}

// ApplyEffects applies effects to the colony in the given mode. It changes nothing on error.
func ApplyEffects(state State, effects Effects, mode Mode) (EffectsResult, error) {
	if err := checkStoredState(state); err != nil {
		return EffectsResult{}, err
	}
	if effects.Colonists < 0 {
		return EffectsResult{}, invalid("colonists", MessageNegativeColonist)
	}
	after := state.Clone()
	result := EffectsResult{
		Changes: []ResourceChange{},
		Applied: Effects{Resources: map[string]int64{}, Colonists: effects.Colonists},
	}
	var shortfalls []Shortfall
	for _, key := range slices.Sorted(maps.Keys(effects.Resources)) {
		delta := effects.Resources[key]
		resource, found := state.Resources[key]
		if !found {
			return EffectsResult{}, invalid("resources", `There is no resource "`+key+`"`)
		}
		if resource.Archived {
			return EffectsResult{}, invalid("resources", resourceName(key, resource, true)+" is archived")
		}
		if delta == 0 {
			continue
		}
		// No delta between two storable amounts is larger than this, so the sums below fit in int64.
		if delta < MinStored-MaxStored || delta > MaxStored-MinStored {
			return EffectsResult{}, invalid("resources", "That much "+resourceName(key, resource, false)+" can't be stored")
		}
		upperBound := int64(MaxStored)
		if resource.MaxAmount != nil {
			upperBound = *resource.MaxAmount
		} else if delta > MaxStored-resource.Amount {
			return EffectsResult{}, invalid("resources", "That much "+resourceName(key, resource, false)+" can't be stored")
		}
		newAmount := resource.Amount + delta
		switch {
		case delta < resource.MinAmount-resource.Amount:
			if mode == Strict {
				shortfalls = append(shortfalls, Shortfall{
					ResourceKey: key,
					Holder:      "the colony",
					What:        resourceName(key, resource, false),
					Has:         resource.Amount,
					Needs:       resource.MinAmount - delta,
				})
				continue
			}
			newAmount = resource.MinAmount
		case delta > upperBound-resource.Amount:
			newAmount = upperBound
		}
		result.Changes = append(result.Changes, ResourceChange{
			Key:       key,
			Before:    resource.Amount,
			After:     newAmount,
			Requested: delta,
			Clamped:   newAmount-resource.Amount != delta,
		})
		if newAmount != resource.Amount {
			result.Applied.Resources[key] = newAmount - resource.Amount
		}
		resource.Amount = newAmount
		after.Resources[key] = resource
	}
	if len(shortfalls) > 0 {
		return EffectsResult{}, &ShortfallError{Shortfalls: shortfalls}
	}
	if effects.Colonists > 0 {
		unassigned, found := after.Roles[UnassignedRoleKey]
		if !found {
			return EffectsResult{}, invalid("colonists", "The colony has no "+UnassignedRoleKey+" role")
		}
		if effects.Colonists > MaxStored-unassigned.UnnamedCount {
			return EffectsResult{}, invalid("colonists", MessageTooManyColonists)
		}
		unassigned.UnnamedCount += effects.Colonists
		after.Roles[UnassignedRoleKey] = unassigned
	}
	result.Before = state.Clone()
	result.After = after
	return result, nil
}

// AdjustmentFromTargets turns the DM's "set to" values into deltas against the locked State, so the
// adjustment applies as Effects in CLAMP mode (spec §5.4).
func AdjustmentFromTargets(state State, targets map[string]int64) (Effects, error) {
	effects := Effects{Resources: map[string]int64{}}
	for _, key := range slices.Sorted(maps.Keys(targets)) {
		resource, found := state.Resources[key]
		if !found {
			return Effects{}, invalid("resources", `There is no resource "`+key+`"`)
		}
		target := targets[key]
		if target < MinStored || target > MaxStored {
			return Effects{}, invalid("resources", "That much "+resourceName(key, resource, false)+" can't be stored")
		}
		if target != resource.Amount {
			effects.Resources[key] = target - resource.Amount
		}
	}
	return effects, nil
}

// resourceName is the resource's label, or its key when it has none, lowercased inside a sentence.
func resourceName(key string, resource Resource, sentenceStart bool) string {
	name := resource.Label
	if name == "" {
		name = key
	}
	if sentenceStart {
		return name
	}
	return strings.ToLower(name)
}
