package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/BBruington/colony-planner/internal/core"
)

// An edit form opened before the DM hid an NPC or place still holds the old switch. The PUT body has
// no revealed field, so a body that carries one is refused (422) and the row stays hidden; only the
// visibility switch reveals it.
func TestEditsCannotRevealAHiddenRow(t *testing.T) {
	testHarness := newHarness(t)
	campaign := testHarness.campaignA
	dmViewer := testHarness.enter(testHarness.dmA, campaign.id)
	dmToken, playerToken := testHarness.signIn(testHarness.dmA), testHarness.signIn(testHarness.ana)
	npcRecord := core.NpcInput{
		Name: "Mother Corrow", Aliases: []string{}, Labels: []string{},
		Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationEnemy,
	}
	npc, err := testHarness.core.CreateNpc(testHarness.context, dmViewer, core.NewNpcInput{NpcInput: npcRecord})
	if err != nil {
		t.Fatal(err)
	}
	placeRecord := core.PlaceInput{Name: "Drowned Chapel"}
	place, err := testHarness.core.CreatePlace(testHarness.context, dmViewer, core.NewPlaceInput{PlaceInput: placeRecord})
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range []struct {
		suffix string
		record any
	}{
		{"/npcs/" + npc.ID.String(), npcRecord},
		{"/places/" + place.ID.String(), placeRecord},
	} {
		path := campaignPathOf(campaign, row.suffix)
		edit, err := json.Marshal(row.record)
		if err != nil {
			t.Fatal(err)
		}
		var staleForm map[string]any
		if err := json.Unmarshal(edit, &staleForm); err != nil {
			t.Fatal(err)
		}
		staleForm["revealed"] = true
		staleEdit, err := json.Marshal(staleForm)
		if err != nil {
			t.Fatal(err)
		}

		refused := testHarness.do(http.MethodPut, path, requestOptions{token: dmToken, body: string(staleEdit)})
		testHarness.expectStatus(refused, http.StatusUnprocessableEntity)
		if !strings.Contains(refused.Body.String(), "revealed") {
			t.Errorf("PUT %s with revealed was refused for another reason: %s", row.suffix, refused.Body.String())
		}
		testHarness.expectStatus(testHarness.do(http.MethodPut, path, requestOptions{token: dmToken, body: string(edit)}), http.StatusOK)
		testHarness.expectStatus(testHarness.do(http.MethodGet, path, requestOptions{token: playerToken}), http.StatusNotFound)

		testHarness.expectStatus(testHarness.do(http.MethodPut, path+"/revealed", requestOptions{token: dmToken, body: `{"revealed":true}`}), http.StatusOK)
		testHarness.expectStatus(testHarness.do(http.MethodGet, path, requestOptions{token: playerToken}), http.StatusOK)
	}
}
