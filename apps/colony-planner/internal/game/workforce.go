package game

import "strconv"

// The workforce rules (spec §5.5). The colonist total changes only when a named NPC joins as a new
// colonist or leaves for good; every other operation moves people between roles, or between a role's
// unnamed and named headcounts. Nothing happens on its own: the DM makes every change.

// Transfer moves count unnamed colonists from one role to another. The total is unchanged.
func Transfer(state State, fromKey string, toKey string, count int64) (State, error) {
	if count < 1 {
		return State{}, invalid("count", "Move at least 1 colonist")
	}
	if fromKey == toKey {
		return State{}, invalid("toRoleKey", "Pick a different role to move them to")
	}
	from, err := activeRole(state, fromKey, "fromRoleKey")
	if err != nil {
		return State{}, err
	}
	to, err := activeRole(state, toKey, "toRoleKey")
	if err != nil {
		return State{}, err
	}
	if from.UnnamedCount < count {
		return State{}, invalid("count", from.Label+" has only "+colonistsText(from.UnnamedCount, "unnamed colonist")+
			", so "+strconv.FormatInt(count, 10)+" can't move")
	}
	if to.UnnamedCount > MaxStored-count {
		return State{}, invalid("count", MessageTooManyColonists)
	}
	after := state.Clone()
	from.UnnamedCount -= count
	to.UnnamedCount += count
	after.Roles[fromKey] = from
	after.Roles[toKey] = to
	return after, nil
}

// SetUnnamedCount sets a role's unnamed headcount directly: a correction, and how deaths and departures
// are recorded.
func SetUnnamedCount(state State, roleKey string, count int64) (State, error) {
	if count < 0 {
		return State{}, invalid("count", MessageNegativeColonist)
	}
	if count > MaxStored {
		return State{}, invalid("count", MessageTooManyColonists)
	}
	role, err := activeRole(state, roleKey, "roleKey")
	if err != nil {
		return State{}, err
	}
	if role.UnnamedCount == count {
		return State{}, invalid("count", role.Label+" already has "+colonistsText(count, "unnamed colonist"))
	}
	after := state.Clone()
	role.UnnamedCount = count
	after.Roles[roleKey] = role
	return after, nil
}

// JoinColony makes a named NPC a colonist in a role: as a new colonist (the total rises by 1) or, with
// promote, by naming one of the role's unnamed colonists (the total is unchanged).
func JoinColony(state State, roleKey string, promote bool) (State, error) {
	role, err := activeRole(state, roleKey, "roleKey")
	if err != nil {
		return State{}, err
	}
	if promote {
		if role.UnnamedCount < 1 {
			return State{}, invalid("mode", role.Label+" has no unnamed colonists to name")
		}
		role.UnnamedCount--
	}
	if role.NamedCount >= MaxStored {
		return State{}, invalid("roleKey", MessageTooManyColonists)
	}
	role.NamedCount++
	after := state.Clone()
	after.Roles[roleKey] = role
	return after, nil
}

// ChangeRole moves a named colonist to another role. The total is unchanged.
func ChangeRole(state State, fromKey string, toKey string) (State, error) {
	if fromKey == toKey {
		return State{}, invalid("roleKey", "They are already in that role")
	}
	from, found := state.Roles[fromKey]
	if !found || from.NamedCount < 1 {
		return State{}, invalid("roleKey", "They aren't counted in their role")
	}
	to, err := activeRole(state, toKey, "roleKey")
	if err != nil {
		return State{}, err
	}
	if to.NamedCount >= MaxStored {
		return State{}, invalid("roleKey", MessageTooManyColonists)
	}
	after := state.Clone()
	from.NamedCount--
	to.NamedCount++
	after.Roles[fromKey] = from
	after.Roles[toKey] = to
	return after, nil
}

// LeaveColony takes a named colonist out of their role: removed (the total falls by 1) or, with
// returnToPool, kept as one of the role's unnamed colonists (the total is unchanged).
func LeaveColony(state State, roleKey string, returnToPool bool) (State, error) {
	role, found := state.Roles[roleKey]
	if !found || role.NamedCount < 1 {
		return State{}, invalid("mode", "They aren't counted in their role")
	}
	if returnToPool {
		if role.UnnamedCount >= MaxStored {
			return State{}, invalid("mode", MessageTooManyColonists)
		}
		role.UnnamedCount++
	}
	role.NamedCount--
	after := state.Clone()
	after.Roles[roleKey] = role
	return after, nil
}

// activeRole is a role a workforce change may put people into: it exists and isn't archived.
func activeRole(state State, roleKey string, field string) (Role, error) {
	role, found := state.Roles[roleKey]
	if !found {
		return Role{}, invalid(field, `There is no role "`+roleKey+`"`)
	}
	if role.Archived {
		return Role{}, invalid(field, role.Label+" is archived")
	}
	return role, nil
}

// colonistsText is "1 unnamed colonist" or "3 unnamed colonists".
func colonistsText(count int64, singular string) string {
	if count == 1 {
		return "1 " + singular
	}
	return strconv.FormatInt(count, 10) + " " + singular + "s"
}
