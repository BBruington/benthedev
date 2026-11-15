package game

import (
	"reflect"
	"testing"
)

func TestDepositAddsTheWholeAmount(t *testing.T) {
	state := marketColony()
	state.Resources["gold"] = Resource{Label: "Gold", Amount: 460, MaxAmount: pointer(int64(500))}
	result, err := Deposit(state, 40)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.After.Resources["gold"].Amount; got != 500 {
		t.Fatalf("gold after %d, want 500", got)
	}
	if want := map[string]int64{"gold": 40}; !reflect.DeepEqual(result.Applied.Resources, want) {
		t.Fatalf("applied %+v, want %+v", result.Applied.Resources, want)
	}
	if state.Resources["gold"].Amount != 460 || result.Before.Resources["gold"].Amount != 460 {
		t.Fatal("Deposit changed its input")
	}
}

func TestDepositRefusesRatherThanCaps(t *testing.T) {
	withGold := func(gold Resource) State {
		state := marketColony()
		state.Resources["gold"] = gold
		return state
	}
	withoutGold := marketColony()
	delete(withoutGold.Resources, "gold")
	for _, testCase := range []struct {
		name    string
		state   State
		gold    int64
		message string
	}{
		{
			name:    "one past the max",
			state:   withGold(Resource{Label: "Gold", Amount: 460, MaxAmount: pointer(int64(500))}),
			gold:    41,
			message: "Colony gold would pass its max of 500. Raise the max on Settings, or dismiss the deposit to return the coins",
		},
		{
			name:    "past what can be stored",
			state:   withGold(Resource{Label: "Gold", Amount: MaxStored - 5}),
			gold:    6,
			message: "Colony gold can't store that much. Dismiss the deposit to return the coins",
		},
		{name: "no gold", state: withoutGold, gold: 5, message: "This colony has no gold to deposit into"},
		{name: "archived", state: withGold(Resource{Label: "Gold", Amount: 10, Archived: true}), gold: 5, message: "Gold is archived"},
		{name: "nothing", state: marketColony(), gold: 0, message: "Deposit at least 1 gold"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Deposit(testCase.state, testCase.gold)
			assertInvalid(t, err, "gold", testCase.message)
		})
	}
}

func TestDepositUpToStoredLimitWithoutAMax(t *testing.T) {
	state := marketColony()
	state.Resources["gold"] = Resource{Label: "Gold", Amount: MaxStored - 5}
	result, err := Deposit(state, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.After.Resources["gold"].Amount; got != MaxStored {
		t.Fatalf("gold after %d, want %d", got, int64(MaxStored))
	}
}
