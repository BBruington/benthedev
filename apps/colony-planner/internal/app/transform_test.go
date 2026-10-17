package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// The Transformer keeps `dm` objects only for a campaign Viewer that sees DM fields: never for a
// visitor, an account or owner route (no Viewer), a player, a DM elsewhere, or the DM viewing as player.
func TestRedactionTransformer(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	member := core.Member{
		ID:          uuid.New(),
		DisplayName: "Ana",
		Dm:          &core.MemberDm{DiscordUserID: "100000000000000002", Status: core.MemberStatusActive},
	}
	operation := &huma.Operation{Method: http.MethodGet, Path: "/api/test"}
	dmViewingAsPlayer := testHarness.dmA
	dmViewingAsPlayer.caller.ViewAsPlayer = true

	for _, testCase := range []struct {
		name    string
		caller  *account
		inside  bool
		keepsDm bool
	}{
		{name: "visitor"},
		{name: "the site owner on an owner route", caller: &testHarness.owner},
		{name: "the DM on an account route", caller: &testHarness.dmA},
		{name: "player", caller: &testHarness.ana, inside: true},
		{name: "a DM elsewhere", caller: &testHarness.dmB, inside: true},
		{name: "DM viewing as player", caller: &dmViewingAsPlayer, inside: true},
		{name: "DM", caller: &testHarness.dmA, inside: true, keepsDm: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			if testCase.caller != nil {
				requestContext := auth.WithCaller(request.Context(), testCase.caller.caller)
				if testCase.inside {
					requestContext = withCampaignViewer(requestContext, testHarness.enter(*testCase.caller, campaignA.id))
				}
				request = request.WithContext(requestContext)
			}
			humaContext := humatest.NewContext(operation, request, httptest.NewRecorder())

			transformed, err := redactDmFields(humaContext, "200", member)
			if err != nil {
				t.Fatalf("transform: %v", err)
			}
			result, ok := transformed.(core.Member)
			if !ok {
				t.Fatalf("transform returned %T, want core.Member", transformed)
			}
			if (result.Dm != nil) != testCase.keepsDm {
				t.Fatalf("dm present = %v, want %v", result.Dm != nil, testCase.keepsDm)
			}
			if result.DisplayName != "Ana" {
				t.Fatalf("visible field lost: %+v", result)
			}
		})
	}
	if member.Dm == nil {
		t.Fatal("the transformer modified the handler's value")
	}
}
