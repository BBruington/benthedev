package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// jsonNumbers collects every number in a JSON document, so a test can look for a planted number
// without matching digits inside an ID or a timestamp.
func jsonNumbers(value any, into map[float64]bool) {
	switch typed := value.(type) {
	case float64:
		into[typed] = true
	case map[string]any:
		for _, nested := range typed {
			jsonNumbers(nested, into)
		}
	case []any:
		for _, nested := range typed {
			jsonNumbers(nested, into)
		}
	}
}

// TestPlayersSeeOnlyVisibleQuestsPatronsAndRequests is the M3 sweep's targeted half, run as every kind
// of player of each campaign: a hidden quest, patron or request reads exactly like a missing one; the
// revealed quest's failure cost reaches no player, as a field or as a number; archived patrons and
// actions are in no player list or picker; and a decision note reaches the requester and the DM only,
// never another player or the DM viewing as player.
func TestPlayersSeeOnlyVisibleQuestsPatronsAndRequests(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	playersOf := map[*campaignFixture][]sweepCaller{
		campaignA: {
			{name: "ana", who: &testHarness.ana},
			{name: "otherA", who: &testHarness.otherA},
			{name: "dmB (DM of B)", who: &testHarness.dmB},
			{name: "dmA viewing as player", who: &testHarness.dmA, viewAsPlayer: true},
		},
		campaignB: {
			{name: "ana", who: &testHarness.ana},
			{name: "bOnly", who: &testHarness.bOnly},
			{name: "dmA (DM of A)", who: &testHarness.dmA},
			{name: "dmB viewing as player", who: &testHarness.dmB, viewAsPlayer: true},
		},
	}
	for _, side := range testHarness.sides() {
		campaign := side.campaign
		forbidden := joinValues(testHarness.dmOnlyValues(campaignA), testHarness.dmOnlyValues(campaignB))
		if campaign == campaignA {
			forbidden = append(forbidden, canaryB)
		}
		read := func(label, token, suffix string) *httptest.ResponseRecorder {
			t.Helper()
			path := campaignPathOf(campaign, suffix)
			response := testHarness.do(http.MethodGet, path, requestOptions{token: token})
			checkResponse(t, label+": GET "+suffix, path, forbidden, response)
			return response
		}
		decode := func(label string, response *httptest.ResponseRecorder, into any) {
			t.Helper()
			testHarness.expectStatus(response, http.StatusOK)
			if err := json.Unmarshal(response.Body.Bytes(), into); err != nil {
				t.Fatalf("%s: decode: %v", label, err)
			}
		}
		requestWithNote := func(requests []core.Request) *core.Request {
			index := slices.IndexFunc(requests, func(request core.Request) bool { return request.ID == campaign.declinedRequestID })
			if index < 0 {
				return nil
			}
			return &requests[index]
		}

		// Controls: the DM reads the failure cost and the decline note, and the requester reads the note.
		dmToken := testHarness.signIn(side.dm)
		var dmQuest core.Quest
		decode("the DM's quest", testHarness.do(http.MethodGet, campaignPathOf(campaign, "/quests/"+campaign.pathIDs["questId"]), requestOptions{token: dmToken}), &dmQuest)
		if dmQuest.Dm == nil || dmQuest.Dm.FailureFavor != side.failureFavor || dmQuest.Dm.FailureEffects.Resources["gold"] != -side.failureGold {
			t.Errorf("the DM of %s reads no failure cost: %+v", campaign.name, dmQuest.Dm)
		}
		for _, reader := range []struct {
			name  string
			token string
		}{{"the DM", dmToken}, {"the requester", testHarness.signIn(campaign.requester)}} {
			var declined core.Request
			decode(reader.name+"'s request", testHarness.do(http.MethodGet, campaignPathOf(campaign, "/requests/"+campaign.declinedRequestID.String()), requestOptions{token: reader.token}), &declined)
			if declined.DecisionNote == nil || !strings.Contains(*declined.DecisionNote, side.dmSecret+" decline note") {
				t.Errorf("%s of %s doesn't read the decline note: %+v", reader.name, campaign.name, declined.DecisionNote)
			}
			var list core.RequestList
			decode(reader.name+"'s requests", testHarness.do(http.MethodGet, campaignPathOf(campaign, "/requests"), requestOptions{token: reader.token}), &list)
			if listed := requestWithNote(list.Requests); listed == nil || listed.DecisionNote == nil {
				t.Errorf("%s of %s doesn't read the decline note in the list", reader.name, campaign.name)
			}
		}
		missingQuest := testHarness.do(http.MethodGet, campaignPathOf(campaign, "/quests/"+uuid.NewString()), requestOptions{token: dmToken})
		missingPatron := testHarness.do(http.MethodGet, campaignPathOf(campaign, "/patrons/"+uuid.NewString()), requestOptions{token: dmToken})
		missingRequest := testHarness.do(http.MethodGet, campaignPathOf(campaign, "/requests/"+uuid.NewString()), requestOptions{token: dmToken})

		for _, caller := range playersOf[campaign] {
			token := testHarness.tokenFor(caller, campaign)
			label := caller.name + " in " + campaign.name

			numbers := map[float64]bool{}
			for _, suffix := range []string{
				"/quests", "/quests/" + campaign.pathIDs["questId"], "/patrons", "/patrons/" + campaign.pathIDs["patronId"],
				"/patron-actions", "/requests", "/requests?mine=true", "/timeline/recent", "/timeline?fromDay=0&toDay=60",
			} {
				response := read(label, token, suffix)
				testHarness.expectStatus(response, http.StatusOK)
				for _, field := range []string{`"failureEffects"`, `"failureFavor"`, `"dmNotes"`} {
					if strings.Contains(response.Body.String(), field) {
						t.Errorf("%s: GET %s carries %s: %s", label, suffix, field, response.Body.String())
					}
				}
				var document any
				if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
					t.Fatalf("%s: decode %s: %v", label, suffix, err)
				}
				jsonNumbers(document, numbers)
			}
			for _, cost := range campaign.failureCost {
				if numbers[float64(cost)] || numbers[float64(-cost)] {
					t.Errorf("%s: a response holds the failure cost %d", label, cost)
				}
			}

			for _, hiddenRow := range []struct {
				suffix  string
				id      string
				missing *httptest.ResponseRecorder
			}{
				{"/quests/" + campaign.hiddenQuestID.String(), campaign.hiddenQuestID.String(), missingQuest},
				{"/patrons/" + campaign.hiddenPatronID.String(), campaign.hiddenPatronID.String(), missingPatron},
				{"/requests/" + campaign.hiddenRequestID.String(), campaign.hiddenRequestID.String(), missingRequest},
			} {
				path := campaignPathOf(campaign, hiddenRow.suffix)
				response := testHarness.do(http.MethodGet, path, requestOptions{token: token})
				checkResponse(t, label+": GET "+hiddenRow.suffix, path, forbidden, response, hiddenRow.id)
				if response.Code != http.StatusNotFound || response.Body.String() != hiddenRow.missing.Body.String() {
					t.Errorf("%s: a hidden row answered %d %s; a missing one answers %d %s", label, response.Code, response.Body.String(), hiddenRow.missing.Code, hiddenRow.missing.Body.String())
				}
			}

			var patrons core.PatronList
			decode(label, read(label, token, "/patrons"), &patrons)
			var actions PatronActionList
			decode(label, read(label, token, "/patron-actions"), &actions)
			var page core.PatronPage
			decode(label, read(label, token, "/patrons/"+campaign.pathIDs["patronId"]), &page)
			listed := []string{}
			for _, patron := range patrons.Patrons {
				listed = append(listed, patron.ID.String())
			}
			for _, action := range append(actions.Actions, page.Actions...) {
				listed = append(listed, action.ID.String())
			}
			var market core.MarketPage
			decode(label, read(label, token, "/market"), &market)
			for _, item := range market.Items {
				listed = append(listed, item.ID.String())
			}
			if !slices.Contains(listed, campaign.pathIDs["patronId"]) || !slices.Contains(listed, campaign.pathIDs["actionId"]) {
				t.Errorf("%s: the revealed patron or action is missing from %v", label, listed)
			}
			for _, archivedID := range campaign.archivedIDs {
				if slices.Contains(listed, archivedID) {
					t.Errorf("%s: an archived patron, action or market item %s is listed", label, archivedID)
				}
			}

			var requests core.RequestList
			decode(label, read(label, token, "/requests"), &requests)
			listedRequest := requestWithNote(requests.Requests)
			if listedRequest == nil || listedRequest.Status != core.RequestStatusDeclined {
				t.Errorf("%s: the party's history lacks the declined request: %+v", label, requests.Requests)
			} else if listedRequest.DecisionNote != nil {
				t.Errorf("%s: reads another member's decision note %q", label, *listedRequest.DecisionNote)
			}
		}
	}
}

// An edit form opened before the DM hid or archived a quest, patron or action still holds the old
// switches. The PUT bodies have no such fields, so a body that carries one is refused (422) and the row
// stays as it was; only the dedicated switches change it.
func TestEditsCannotRevealOrUnarchiveQuestsAndPatrons(t *testing.T) {
	testHarness := newHarness(t)
	campaign := testHarness.campaignA
	dmViewer := testHarness.enter(testHarness.dmA, campaign.id)
	dmToken, playerToken := testHarness.signIn(testHarness.dmA), testHarness.signIn(testHarness.ana)

	questRecord := core.QuestInput{Title: "Burn the Archive", Kind: core.QuestKindColony, Loot: []core.LootItem{}}
	quest, err := testHarness.core.CreateQuest(testHarness.context, dmViewer, core.NewQuestInput{QuestInput: questRecord})
	if err != nil {
		t.Fatal(err)
	}
	patronRecord := core.PatronInput{Name: "Shadow Broker"}
	patron, err := testHarness.core.CreatePatron(testHarness.context, dmViewer, core.NewPatronInput{PatronInput: patronRecord})
	if err != nil {
		t.Fatal(err)
	}
	actionRecord := core.PatronActionInput{Name: "Night Run", FavorCost: 1}
	action, err := testHarness.core.CreatePatronAction(testHarness.context, dmViewer, core.NewPatronActionInput{PatronID: patron.Patron.ID, PatronActionInput: actionRecord})
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range []struct {
		suffix      string
		record      any
		staleFields []string
	}{
		{"/quests/" + quest.ID.String(), questRecord, []string{"revealed", "status"}},
		{"/patrons/" + patron.Patron.ID.String(), patronRecord, []string{"revealed", "archived", "favor"}},
		{"/patron-actions/" + action.ID.String(), actionRecord, []string{"revealed", "archived"}},
	} {
		path := campaignPathOf(campaign, row.suffix)
		edit, err := json.Marshal(row.record)
		if err != nil {
			t.Fatal(err)
		}
		for _, staleField := range row.staleFields {
			var staleForm map[string]any
			if err := json.Unmarshal(edit, &staleForm); err != nil {
				t.Fatal(err)
			}
			staleForm[staleField] = true
			staleEdit, err := json.Marshal(staleForm)
			if err != nil {
				t.Fatal(err)
			}
			refused := testHarness.do(http.MethodPut, path, requestOptions{token: dmToken, body: string(staleEdit)})
			testHarness.expectStatus(refused, http.StatusUnprocessableEntity)
			if !strings.Contains(refused.Body.String(), staleField) {
				t.Errorf("PUT %s with %s was refused for another reason: %s", row.suffix, staleField, refused.Body.String())
			}
		}
		testHarness.expectStatus(testHarness.do(http.MethodPut, path, requestOptions{token: dmToken, body: string(edit)}), http.StatusOK)
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaign, "/quests/"+quest.ID.String()), requestOptions{token: playerToken}), http.StatusNotFound)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaign, "/patrons/"+patron.Patron.ID.String()), requestOptions{token: playerToken}), http.StatusNotFound)

	reveal := testHarness.do(http.MethodPut, campaignPathOf(campaign, "/quests/"+quest.ID.String()+"/revealed"), requestOptions{token: dmToken, body: `{"revealed":true,"postToTimeline":false}`})
	testHarness.expectStatus(reveal, http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaign, "/quests/"+quest.ID.String()), requestOptions{token: playerToken}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodPut, campaignPathOf(campaign, "/patrons/"+patron.Patron.ID.String()+"/revealed"), requestOptions{token: dmToken, body: `{"revealed":true}`}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaign, "/patrons/"+patron.Patron.ID.String()), requestOptions{token: playerToken}), http.StatusOK)
}

// A member withdraws only their own pending request; the DM decides, and a second decision is a
// conflict. Another member's withdraw reads like a request that doesn't exist.
func TestRequestsOverHTTP(t *testing.T) {
	testHarness := newHarness(t)
	campaign := testHarness.campaignA
	dmViewer := testHarness.enter(testHarness.dmA, campaign.id)
	patron, err := testHarness.core.CreatePatron(testHarness.context, dmViewer, core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{Name: "Lady Vex"}})
	if err != nil {
		t.Fatal(err)
	}
	board, err := testHarness.core.GetColonyBoard(testHarness.context, dmViewer)
	if err != nil {
		t.Fatal(err)
	}
	favor := int64(5)
	if _, err := testHarness.core.AdjustFavor(testHarness.context, dmViewer, patron.Patron.ID, core.FavorAdjustInput{ExpectedVersion: board.Version, SetTo: &favor}); err != nil {
		t.Fatal(err)
	}
	action, err := testHarness.core.CreatePatronAction(testHarness.context, dmViewer, core.NewPatronActionInput{
		PatronID: patron.Patron.ID, Revealed: true,
		PatronActionInput: core.PatronActionInput{Name: "Grain Barge", FavorCost: 3, Effects: core.Effects{Resources: map[string]int64{"food": 7}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	anaToken, otherToken, dmToken := testHarness.signIn(testHarness.ana), testHarness.signIn(testHarness.otherA), testHarness.signIn(testHarness.dmA)
	ask := func(token string) core.Request {
		t.Helper()
		response := testHarness.do(http.MethodPost, campaignPathOf(campaign, "/requests"), requestOptions{token: token, body: `{"actionId":"` + action.ID.String() + `","note":"For the winter"}`})
		testHarness.expectStatus(response, http.StatusCreated)
		var request core.Request
		if err := json.Unmarshal(response.Body.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		return request
	}
	requestPathOf := func(request core.Request, suffix string) string {
		return campaignPathOf(campaign, "/requests/"+request.ID.String()+suffix)
	}

	first := ask(anaToken)
	if !first.Mine || first.Status != core.RequestStatusPending {
		t.Errorf("a new request = %+v, want the requester's own, PENDING", first)
	}
	testHarness.expectStatus(testHarness.do(http.MethodPost, requestPathOf(first, "/withdraw"), requestOptions{token: otherToken}), http.StatusNotFound)
	testHarness.expectStatus(testHarness.do(http.MethodPost, requestPathOf(first, "/approve"), requestOptions{token: anaToken, body: `{"note":""}`}), http.StatusForbidden)
	testHarness.expectStatus(testHarness.do(http.MethodPost, requestPathOf(first, "/withdraw"), requestOptions{token: anaToken}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodPost, requestPathOf(first, "/withdraw"), requestOptions{token: anaToken}), http.StatusConflict)

	second, third := ask(anaToken), ask(otherToken)
	preview := testHarness.do(http.MethodPost, requestPathOf(second, "/approve-preview"), requestOptions{token: dmToken, body: `{"note":""}`})
	testHarness.expectStatus(preview, http.StatusOK)
	if !strings.Contains(preview.Body.String(), `"affordable":true`) {
		t.Errorf("the approval preview: %s", preview.Body.String())
	}
	testHarness.expectStatus(testHarness.do(http.MethodPost, requestPathOf(second, "/approve"), requestOptions{token: dmToken, body: `{"note":"Enjoy"}`}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodPost, requestPathOf(second, "/approve"), requestOptions{token: dmToken, body: `{"note":""}`}), http.StatusConflict)
	short := testHarness.do(http.MethodPost, requestPathOf(third, "/approve"), requestOptions{token: dmToken, body: `{"note":""}`})
	testHarness.expectStatus(short, http.StatusConflict)
	if !strings.Contains(short.Body.String(), "Not enough favor: Lady Vex has 2, needs 3") {
		t.Errorf("the second grant's refusal doesn't name the shortfall: %s", short.Body.String())
	}
	testHarness.expectStatus(testHarness.do(http.MethodPost, requestPathOf(third, "/decline"), requestOptions{token: dmToken, body: `{"note":"Vex is busy this week."}`}), http.StatusOK)
	unaffordable := testHarness.do(http.MethodPost, campaignPathOf(campaign, "/requests"), requestOptions{token: anaToken, body: `{"actionId":"` + action.ID.String() + `","note":""}`})
	testHarness.expectStatus(unaffordable, http.StatusUnprocessableEntity)
}
