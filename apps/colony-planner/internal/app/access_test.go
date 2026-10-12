package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	authService, err := auth.New(auth.Options{AppEnvironment: "test", PublicURL: "http://localhost:5173"})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	server, err := New(Options{Core: core.New(core.Options{}), Auth: authService, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	return server
}

func TestEveryOperationDeclaresAnAccessLevel(t *testing.T) {
	server := newTestServer(t)
	for path, pathItem := range server.OpenAPI().Paths {
		for method, operation := range pathOperations(pathItem) {
			if !operationAccess(operation).valid() {
				t.Errorf("%s %s has no valid %s", method, path, accessExtension)
			}
		}
	}
}

func TestAccessCheckRejectsUndeclaredRoutes(t *testing.T) {
	api := humago.New(http.NewServeMux(), humaConfig())
	huma.Register(api, huma.Operation{OperationID: "sneaky", Method: http.MethodGet, Path: "/api/sneaky"},
		func(context.Context, *struct{}) (*struct{}, error) { return &struct{}{}, nil })
	routes := []Route{{Pattern: "GET /img/{id}", Handler: http.NotFoundHandler()}}

	err := checkAccessDeclarations(api, routes)
	if err == nil {
		t.Fatal("expected the access check to fail")
	}
	for _, expected := range []string{"GET /api/sneaky", "GET /img/{id}"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not name %q", err, expected)
		}
	}
}

// TestAccessDecision runs the access rules against real Viewers, which only core can build.
func TestAccessDecision(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	viewerOf := func(who account) core.Viewer { return testHarness.enter(who, campaignA.id) }
	dmViewingAsPlayer := testHarness.dmA
	dmViewingAsPlayer.caller.ViewAsPlayer = true
	signedIn := accessRequest{caller: testHarness.outsider.caller, signedIn: true}
	member := func(who account) accessRequest {
		return accessRequest{caller: who.caller, signedIn: true, viewer: viewerOf(who), isMember: true}
	}

	for _, testCase := range []struct {
		name    string
		level   AccessLevel
		request accessRequest
		status  int
	}{
		{"public admits visitors", AccessPublic, accessRequest{}, 0},
		{"token admits visitors", AccessToken, accessRequest{}, 0},
		{"account refuses visitors", AccessAccount, accessRequest{}, http.StatusUnauthorized},
		{"account admits any account", AccessAccount, signedIn, 0},
		{"owner refuses visitors", AccessOwner, accessRequest{}, http.StatusUnauthorized},
		{"owner refuses other accounts", AccessOwner, member(testHarness.dmA), http.StatusForbidden},
		{"owner admits the site owner", AccessOwner, accessRequest{caller: testHarness.owner.caller, signedIn: true}, 0},
		{"pending refuses visitors", AccessPending, accessRequest{}, http.StatusUnauthorized},
		{"pending refuses non-members with 404", AccessPending, signedIn, http.StatusNotFound},
		{"pending admits pending", AccessPending, member(testHarness.pendingA), 0},
		{"pending admits removed", AccessPending, member(testHarness.removedA), 0},
		{"member refuses visitors", AccessMember, accessRequest{}, http.StatusUnauthorized},
		{"member refuses non-members with 404", AccessMember, signedIn, http.StatusNotFound},
		{"member refuses pending", AccessMember, member(testHarness.pendingA), http.StatusForbidden},
		{"member refuses removed", AccessMember, member(testHarness.removedA), http.StatusForbidden},
		{"member admits players", AccessMember, member(testHarness.ana), 0},
		{"member admits a DM elsewhere as a player", AccessMember, member(testHarness.dmB), 0},
		{"member admits the DM viewing as player", AccessMember, member(dmViewingAsPlayer), 0},
		{"dm refuses visitors", AccessDm, accessRequest{}, http.StatusUnauthorized},
		{"dm refuses non-members with 404", AccessDm, signedIn, http.StatusNotFound},
		{"dm refuses players", AccessDm, member(testHarness.ana), http.StatusForbidden},
		{"dm refuses a DM elsewhere", AccessDm, member(testHarness.dmB), http.StatusForbidden},
		{"dm refuses the DM viewing as player", AccessDm, member(dmViewingAsPlayer), http.StatusForbidden},
		{"dm admits the DM", AccessDm, member(testHarness.dmA), 0},
		{"a missing level fails closed", AccessLevel(""), member(testHarness.dmA), http.StatusInternalServerError},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := testCase.request
			request.accessedAs = testCase.level
			status, _ := accessDecision(request)
			if status != testCase.status {
				t.Fatalf("status = %d, want %d", status, testCase.status)
			}
		})
	}
}

// TestSiteOwnerChangesNoCampaignDecision: for every campaign level and every kind of membership,
// being the site owner gives the same answer as not being it. The owner who DMs A is a stranger to B.
func TestSiteOwnerChangesNoCampaignDecision(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	dmViewingAsPlayer := testHarness.dmA
	dmViewingAsPlayer.caller.ViewAsPlayer = true
	for _, who := range []account{testHarness.dmA, dmViewingAsPlayer, testHarness.ana, testHarness.dmB, testHarness.pendingA, testHarness.removedA, testHarness.outsider, testHarness.owner} {
		viewer, err := testHarness.core.EnterCampaign(testHarness.context, who.caller, campaignA.id)
		isMember := err == nil
		for _, level := range []AccessLevel{AccessPending, AccessMember, AccessDm} {
			answers := map[bool]int{}
			for _, isSiteOwner := range []bool{false, true} {
				caller := who.caller
				caller.IsSiteOwner = isSiteOwner
				status, _ := accessDecision(accessRequest{caller: caller, signedIn: true, viewer: viewer, isMember: isMember, accessedAs: level})
				answers[isSiteOwner] = status
			}
			if answers[false] != answers[true] {
				t.Errorf("%s at %s: %d as a non-owner, %d as the owner", who.name, level, answers[false], answers[true])
			}
		}
	}

	// Over HTTP: a site owner who DMs campaign A gets 404 from campaign B.
	ownerService := core.New(core.Options{Pool: testHarness.database.Pool, SiteOwnerDiscordID: testHarness.dmA.caller.DiscordUserID})
	ownerAuth, err := auth.New(auth.Options{Core: ownerService, PublicURL: testPublicURL, AppEnvironment: "test"})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	ownerServer, err := New(Options{Core: ownerService, Auth: ownerAuth, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	soloDm := testHarness.signUp("soloDm", "400000000000000001", "solo", "Solo")
	campaignC := testHarness.createCampaign(soloDm, "Third", "third-campaign")
	token := testHarness.signIn(testHarness.dmA)
	for _, path := range []string{campaignPathOf(campaignA, "/members"), campaignPathOf(campaignC, "/colony"), campaignPathOf(campaignC, "/me"), "/api/admin/campaigns"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Host = testHost
		request.AddCookie(&http.Cookie{Name: ownerAuth.SessionCookieName(), Value: token})
		recorder := httptest.NewRecorder()
		ownerServer.Handler().ServeHTTP(recorder, request)
		want := http.StatusOK
		if strings.Contains(path, campaignC.id.String()) {
			want = http.StatusNotFound
		}
		if recorder.Code != want {
			t.Errorf("the owner who DMs A: GET %s = %d, want %d; body: %s", path, recorder.Code, want, recorder.Body.String())
		}
	}
}

// The startup check refuses a campaign level on a path without {campaignId}, a {campaignId} route
// outside /api/admin/ at any other level, an owner route outside /api/admin/, and a non-owner admin
// route; an owner route under /api/admin/ may carry {campaignId}.
func TestStartupCheckMatchesLevelsToPaths(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		path  string
		level AccessLevel
		fails bool
	}{
		{"a campaign route at member", "/api/campaigns/{campaignId}/things", AccessMember, false},
		{"an admin campaign route at owner", "/api/admin/campaigns/{campaignId}/things", AccessOwner, false},
		{"an account route", "/api/things", AccessAccount, false},
		{"the docs at owner", "/api/docs", AccessOwner, false},
		{"a campaign route at account", "/api/campaigns/{campaignId}/things", AccessAccount, true},
		{"a campaign route at public", "/api/campaigns/{campaignId}/things", AccessPublic, true},
		{"a campaign route at owner outside admin", "/api/campaigns/{campaignId}/things", AccessOwner, true},
		{"member without a campaign", "/api/things", AccessMember, true},
		{"dm without a campaign", "/api/things", AccessDm, true},
		{"pending under admin", "/api/admin/campaigns/{campaignId}/things", AccessPending, true},
		{"an admin route at account", "/api/admin/things", AccessAccount, true},
		{"owner outside admin", "/api/things", AccessOwner, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			api := humago.New(http.NewServeMux(), humaConfig())
			Register(api, testCase.level, huma.Operation{OperationID: "probe", Method: http.MethodGet, Path: testCase.path},
				func(context.Context, *struct{}) (*struct{}, error) { return &struct{}{}, nil })
			err := checkAccessDeclarations(api, nil)
			if (err != nil) != testCase.fails {
				t.Fatalf("check = %v, want failure %v", err, testCase.fails)
			}
			routeError := checkAccessDeclarations(humago.New(http.NewServeMux(), humaConfig()), []Route{{Pattern: "GET " + testCase.path, Access: testCase.level, Handler: http.NotFoundHandler()}})
			if (routeError != nil) != testCase.fails {
				t.Fatalf("route-table check = %v, want failure %v", routeError, testCase.fails)
			}
		})
	}
}

const sampleCampaignID = "6f1c7a2e-0000-4000-8000-000000000000"

func TestServerRoutes(t *testing.T) {
	handler := newTestServer(t).Handler()

	for _, testCase := range []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		headers     map[string]string
		status      int
		contains    string
		notContains string
	}{
		{name: "health check is public", method: http.MethodGet, path: "/healthz", status: http.StatusOK, contains: "ok"},
		{name: "me needs a session", method: http.MethodGet, path: "/api/me", status: http.StatusUnauthorized},
		{name: "members need a session", method: http.MethodGet, path: "/api/campaigns/" + sampleCampaignID + "/members", status: http.StatusUnauthorized},
		{name: "a malformed campaign needs a session first", method: http.MethodGet, path: "/api/campaigns/not-a-uuid/colony", status: http.StatusUnauthorized},
		{name: "the admin routes need a session", method: http.MethodGet, path: "/api/admin/campaigns", status: http.StatusUnauthorized},
		{name: "the OpenAPI document is owner-only", method: http.MethodGet, path: "/api/openapi.json", status: http.StatusUnauthorized},
		{name: "the docs page is owner-only", method: http.MethodGet, path: "/api/docs", status: http.StatusUnauthorized},
		{name: "huma's default docs route is off", method: http.MethodGet, path: "/docs", status: http.StatusOK, notContains: "elements-api"},
		{name: "huma's default OpenAPI route is off", method: http.MethodGet, path: "/openapi.json", status: http.StatusOK, notContains: `"openapi"`},
		{name: "the stream needs a session", method: http.MethodGet, path: "/api/campaigns/" + sampleCampaignID + "/stream", status: http.StatusUnauthorized},
		{name: "unknown API paths are 404", method: http.MethodGet, path: "/api/nothing-here", status: http.StatusNotFound},
		{name: "the old unscoped stream is gone", method: http.MethodGet, path: "/api/stream", status: http.StatusNotFound},
		{name: "client-side routes get the SPA shell", method: http.MethodGet, path: "/players", status: http.StatusOK, contains: "<html"},
		{name: "logout needs a session", method: http.MethodPost, path: "/auth/logout", status: http.StatusUnauthorized},
		{
			name: "bodies must be JSON", method: http.MethodPut, path: "/api/campaigns/" + sampleCampaignID + "/me/profile",
			contentType: "text/plain", body: "characterName=Thorin", status: http.StatusUnsupportedMediaType,
		},
		{
			name: "cross-origin mutations are refused", method: http.MethodPost, path: "/auth/logout",
			headers: map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"},
			status:  http.StatusForbidden,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			request.Host = testHost
			if testCase.contentType != "" {
				request.Header.Set("Content-Type", testCase.contentType)
			}
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body: %s", recorder.Code, testCase.status, recorder.Body.String())
			}
			if testCase.contains != "" && !strings.Contains(recorder.Body.String(), testCase.contains) {
				t.Fatalf("body does not contain %q: %s", testCase.contains, recorder.Body.String())
			}
			if testCase.notContains != "" && strings.Contains(recorder.Body.String(), testCase.notContains) {
				t.Fatalf("body contains %q: %s", testCase.notContains, recorder.Body.String())
			}
		})
	}
}
