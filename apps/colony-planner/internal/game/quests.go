package game

// QuestResult is what completing or failing a quest changes: the colony, and its patron giver's favor
// when it has one.
type QuestResult struct {
	Effects EffectsResult
	// PatronBefore and PatronAfter are nil for a quest with no patron giver.
	PatronBefore *Patron
	PatronAfter  *Patron
}

// ResolveQuest applies a quest's reward when it is completed, or its failure cost when it fails (spec
// §5.6): effects in CLAMP mode, since neither can be refused, with colonists landing in Unassigned; and
// favorDelta to the patron giver in full. A reward adds favor; a failure's negative delta may take it
// below 0 (Open Question 14). A favor delta needs a patron giver. It changes nothing on error.
func ResolveQuest(state State, patron *Patron, effects Effects, favorDelta int64) (QuestResult, error) {
	if favorDelta != 0 && patron == nil {
		return QuestResult{}, invalid("patronId", "Favor needs a patron giver")
	}
	effectsResult, err := ApplyEffects(state, effects, Clamp)
	if err != nil {
		return QuestResult{}, err
	}
	result := QuestResult{Effects: effectsResult}
	if patron != nil {
		after, err := AddFavor(*patron, favorDelta)
		if err != nil {
			return QuestResult{}, err
		}
		before := *patron
		result.PatronBefore = &before
		result.PatronAfter = &after
	}
	return result, nil
}
