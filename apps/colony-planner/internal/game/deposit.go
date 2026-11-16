package game

import "strconv"

// DepositResult is the State to save after a Foundry actor's coins join colony gold, with what to log.
type DepositResult struct {
	Before State
	After  State
	// Applied holds the gold delta, which is always the whole deposit.
	Applied Effects
}

// Deposit adds a deposit from Foundry to colony gold, 1 gp to 1 gold. Unlike a STRICT grant it refuses
// rather than caps: the coins already left the actor that paid, so capping would lose the rest
// (docs/design/foundry-bridge.md §14.5).
func Deposit(state State, gold int64) (DepositResult, error) {
	if err := checkStoredState(state); err != nil {
		return DepositResult{}, err
	}
	if gold < 1 {
		return DepositResult{}, invalid("gold", "Deposit at least 1 gold")
	}
	resource, found := state.Resources[GoldResourceKey]
	if !found {
		return DepositResult{}, invalid("gold", "This colony has no gold to deposit into")
	}
	if resource.Archived {
		return DepositResult{}, invalid("gold", resourceName(GoldResourceKey, resource, true)+" is archived")
	}
	if effective := EffectiveMax(state, GoldResourceKey); effective != nil && gold > *effective-resource.Amount {
		return DepositResult{}, invalid("gold", "Colony "+resourceName(GoldResourceKey, resource, false)+" would pass its max of "+
			strconv.FormatInt(*effective, 10)+". Raise the max on Settings, or dismiss the deposit to return the coins")
	}
	if gold > MaxStored-resource.Amount {
		return DepositResult{}, invalid("gold", "Colony "+resourceName(GoldResourceKey, resource, false)+
			" can't store that much. Dismiss the deposit to return the coins")
	}
	after := state.Clone()
	resource.Amount += gold
	after.Resources[GoldResourceKey] = resource
	return DepositResult{
		Before:  state.Clone(),
		After:   after,
		Applied: Effects{Resources: map[string]int64{GoldResourceKey: gold}},
	}, nil
}
