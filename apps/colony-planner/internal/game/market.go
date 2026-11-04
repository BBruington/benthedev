package game

import (
	"maps"
	"slices"
)

// MessageOrderTooCostly refuses an order whose total no gold amount can hold.
const MessageOrderTooCostly = "That order costs more gold than the colony can hold"

// PurchaseLine is one line of a market order as it was snapshotted when the order was placed.
type PurchaseLine struct {
	UnitPrice int64
	Quantity  int64
	// Resources maps resource keys to the positive amount one unit grants.
	Resources map[string]int64
	// Colonists is how many colonists one unit brings, landing in RoleKey.
	Colonists int64
	// RoleKey is empty for Unassigned. Bought colonists keep their role, the market's one exception
	// to every new colonist landing in Unassigned.
	RoleKey string
}

// UnloadResult is the State to save after a docked order is paid for, with what to log.
type UnloadResult struct {
	Before State
	After  State
	// Changes lists gold and every granted resource that changed, sorted by key.
	Changes []ResourceChange
	// Applied holds the applied resource deltas only; bought colonists are in Colonists.
	Applied Effects
	// Colonists maps each role key to the colonists the order added to it.
	Colonists map[string]int64
	Total     int64
}

// OrderTotal is the gold an order costs: the sum of price × quantity.
func OrderTotal(lines []PurchaseLine) (int64, error) {
	var total int64
	for _, line := range lines {
		if line.UnitPrice < 0 || line.Quantity < 0 {
			return 0, invalid("cart", "Prices and quantities can't be negative")
		}
		if line.Quantity > 0 && line.UnitPrice > (MaxStored-total)/line.Quantity {
			return 0, invalid("cart", MessageOrderTooCostly)
		}
		total += line.UnitPrice * line.Quantity
	}
	return total, nil
}

// ArrivesDay is the day an order placed on day docks after a voyage of voyageDays.
func ArrivesDay(day int64, voyageDays int64) (int64, error) {
	if voyageDays < 1 {
		return 0, invalid("voyageDays", "A voyage takes at least 1 day")
	}
	if day < 0 {
		return 0, invalid("day", MessageNegativeDay)
	}
	if voyageDays > MaxStored-day {
		return 0, invalid("voyageDays", MessageTooManyDays)
	}
	return day + voyageDays, nil
}

// Docked says whether an order arriving on arrivesDay is at the docks on day.
func Docked(day int64, arrivesDay int64) bool {
	return day >= arrivesDay
}

// DocksDuring says whether an advance from fromDay to toDay passes the order's arrival.
func DocksDuring(fromDay int64, toDay int64, arrivesDay int64) bool {
	return fromDay < arrivesDay && arrivesDay <= toDay
}

// Unload pays for an order in STRICT gold and lands its goods: grants cap at their maximum, and
// colonists join their roles. It refuses rather than drop any part of the order.
func Unload(state State, lines []PurchaseLine) (UnloadResult, error) {
	total, err := OrderTotal(lines)
	if err != nil {
		return UnloadResult{}, err
	}
	deltas := map[string]int64{GoldResourceKey: -total}
	colonists := map[string]int64{}
	for _, line := range lines {
		for _, key := range slices.Sorted(maps.Keys(line.Resources)) {
			grant := line.Resources[key]
			if grant < 0 {
				return UnloadResult{}, invalid("resources", "A purchase can't take resources away")
			}
			if line.Quantity > 0 && grant > (MaxStored-MinStored-deltas[key])/line.Quantity {
				return UnloadResult{}, invalid("resources", "That much "+resourceName(key, state.Resources[key], false)+" can't be stored")
			}
			deltas[key] += grant * line.Quantity
		}
		if line.Colonists < 0 {
			return UnloadResult{}, invalid("colonists", MessageNegativeColonist)
		}
		roleKey := line.RoleKey
		if roleKey == "" {
			roleKey = UnassignedRoleKey
		}
		if line.Quantity > 0 && line.Colonists > (MaxStored-colonists[roleKey])/line.Quantity {
			return UnloadResult{}, invalid("colonists", MessageTooManyColonists)
		}
		if added := line.Colonists * line.Quantity; added > 0 {
			colonists[roleKey] += added
		}
	}
	effects, err := ApplyEffects(state, Effects{Resources: deltas}, Strict)
	if err != nil {
		return UnloadResult{}, err
	}
	after := effects.After
	for _, roleKey := range slices.Sorted(maps.Keys(colonists)) {
		role, err := activeRole(after, roleKey, "roleKey")
		if err != nil {
			return UnloadResult{}, err
		}
		if role.UnnamedCount > MaxStored-colonists[roleKey] {
			return UnloadResult{}, invalid("roleKey", MessageTooManyColonists)
		}
		role.UnnamedCount += colonists[roleKey]
		after.Roles[roleKey] = role
	}
	return UnloadResult{
		Before:    effects.Before,
		After:     after,
		Changes:   effects.Changes,
		Applied:   Effects{Resources: effects.Applied.Resources},
		Colonists: colonists,
		Total:     total,
	}, nil
}
