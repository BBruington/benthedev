package game

import "errors"

// Patron is the part of a patron row the favor rules read.
type Patron struct {
	Name  string
	Favor int64
}

// AddFavor pays a quest's reward_favor or applies a DM adjustment. Neither is bounded below (Open
// Question 12's default lets the DM push favor under 0); only the integer column bounds it.
func AddFavor(patron Patron, delta int64) (Patron, error) {
	newFavor := patron.Favor + delta
	if delta > MaxStored-MinStored || delta < MinStored-MaxStored || newFavor > MaxStored || newFavor < MinStored {
		return Patron{}, invalid("favor", "That much favor can't be stored")
	}
	patron.Favor = newFavor
	return patron, nil
}

// SpendResult is what a granted request changes: the patron's favor and the colony.
type SpendResult struct {
	PatronBefore Patron
	PatronAfter  Patron
	Effects      EffectsResult
}

// Spend pays a request's favor cost and applies its effects, all in STRICT mode (spec §5.4, §5.7):
// spending never takes favor below 0, and a negative resource delta never takes a resource below its
// minimum. If anything falls short, nothing changes and the ShortfallError names every shortfall,
// e.g. "Not enough favor: Lady Vex has 2, needs 3". Requesting checks it (422), approving re-checks it (409).
func Spend(state State, patron Patron, favorCost int64, effects Effects) (SpendResult, error) {
	if favorCost < 0 {
		return SpendResult{}, invalid("favorCost", "A favor cost can't be negative")
	}
	var shortfalls []Shortfall
	if favorCost > 0 && patron.Favor < favorCost {
		shortfalls = append(shortfalls, Shortfall{Holder: patron.Name, What: "favor", Has: patron.Favor, Needs: favorCost})
	}
	effectsResult, err := ApplyEffects(state, effects, Strict)
	var shortfallError *ShortfallError
	if errors.As(err, &shortfallError) {
		shortfalls = append(shortfalls, shortfallError.Shortfalls...)
	} else if err != nil {
		return SpendResult{}, err
	}
	if len(shortfalls) > 0 {
		return SpendResult{}, &ShortfallError{Shortfalls: shortfalls}
	}
	patronAfter := patron
	patronAfter.Favor -= favorCost
	return SpendResult{PatronBefore: patron, PatronAfter: patronAfter, Effects: effectsResult}, nil
}
