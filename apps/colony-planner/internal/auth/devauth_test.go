//go:build devauth

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BBruington/colony-planner/internal/core"
)

func TestDevAuthRefusesProduction(t *testing.T) {
	if _, err := New(Options{AppEnvironment: "production", PublicURL: "https://colony.example"}); err == nil {
		t.Fatal("a devauth build started with APP_ENV=production")
	}
	if _, err := New(Options{AppEnvironment: "development", PublicURL: "http://localhost:5173"}); err != nil {
		t.Fatalf("a devauth build refused development: %v", err)
	}
	if EffectiveSiteOwnerDiscordID("") != core.SeedDmDiscordID {
		t.Fatal("with SITE_OWNER_DISCORD_ID unset, a devauth build should treat the seeded DM as the site owner")
	}
	if EffectiveSiteOwnerDiscordID("100000000000000001") != "100000000000000001" {
		t.Fatal("a set SITE_OWNER_DISCORD_ID must win")
	}
}

func (harness *authHarness) devSignIn(query string, previous *http.Cookie) *httptest.ResponseRecorder {
	harness.t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/auth/dev?"+query, nil)
	if previous != nil {
		request.AddCookie(previous)
	}
	return harness.serve(harness.auth.DevSignIn(), request)
}

func (harness *authHarness) devSignInAs(query string) core.Caller {
	harness.t.Helper()
	response := harness.devSignIn(query, nil)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		harness.t.Fatalf("dev sign-in %s answered %d → %q; body: %s", query, response.Code, response.Header().Get("Location"), response.Body.String())
	}
	caller, signedIn := harness.callerFor(responseCookie(response, "colony_session"))
	if !signedIn {
		harness.t.Fatalf("dev sign-in %s did not sign in", query)
	}
	return caller
}

func TestDevSignIn(t *testing.T) {
	harness := newAuthHarness(t, Options{SiteOwnerDiscordID: core.SeedDmDiscordID})

	// A fresh database has no one; as=owner creates the site owner, and as=dm finds no Sample Colony.
	owner := harness.devSignInAs("as=owner")
	if !owner.IsSiteOwner || owner.DiscordUserID != core.SeedDmDiscordID {
		t.Fatalf("as=owner gave %+v", owner)
	}
	if again := harness.devSignInAs("as=owner"); again.AccountID != owner.AccountID {
		t.Fatal("as=owner created a second owner")
	}
	for _, query := range []string{"as=dm", "as=player", "as=pending"} {
		if response := harness.devSignIn(query, nil); response.Code != http.StatusNotFound {
			t.Fatalf("%s without Sample Colony = %d, want 404", query, response.Code)
		}
	}

	// Sample Colony, DM'd by the owner (whose own request is approved on submit), with a player and a
	// waiting joiner; and another campaign the player DMs, so as=dm must pick Sample Colony's DM.
	_, inviteCode := harness.createCampaign(owner, "Sample Colony", "sample-colony")
	player := harness.createAccount("seed:ana", "Ana")
	pending := harness.createAccount("seed:emil", "Emil")
	testContext := context.Background()
	ownerViewer := func() core.Viewer {
		campaigns := harness.memberships(owner)
		viewer, err := harness.core.EnterCampaign(testContext, owner, campaigns[0].Campaign.ID)
		if err != nil {
			t.Fatalf("enter: %v", err)
		}
		return viewer
	}
	joined, err := harness.core.JoinByInvite(testContext, player, inviteCode)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := harness.core.ApproveMember(testContext, ownerViewer(), joined.MemberID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := harness.core.JoinByInvite(testContext, pending, inviteCode); err != nil {
		t.Fatalf("join: %v", err)
	}
	harness.createCampaign(player, "Ana's Table", "anas-table")

	for query, want := range map[string]core.Caller{"as=dm": owner, "as=player": player, "as=pending": pending, "as=" + pending.AccountID.String(): pending} {
		if caller := harness.devSignInAs(query); caller.AccountID != want.AccountID {
			t.Errorf("%s signed in as %s, want %s", query, caller.AccountID, want.AccountID)
		}
	}

	// as=new is a first Discord sign-in: an account in no campaign, and no ping.
	brandNew := harness.devSignInAs("as=new&name=Brielle%20Vane")
	if len(harness.memberships(brandNew)) != 0 || brandNew.IsSiteOwner {
		t.Fatalf("as=new gave %+v with memberships %+v", brandNew, harness.memberships(brandNew))
	}
	me, err := harness.core.AccountMe(testContext, brandNew)
	if err != nil || me.DisplayName != "Brielle Vane" {
		t.Fatalf("as=new account = %+v, %v", me, err)
	}
	pings := harness.notifier.count()
	harness.devSignInAs("as=new&name=Another")
	if harness.notifier.count() != pings {
		t.Fatal("a brand-new sign-in pinged someone; only asking to join does")
	}

	for query, status := range map[string]int{
		"as=new":    http.StatusBadRequest,
		"as=wizard": http.StatusBadRequest,
		"as=6f1c7a2e-0000-4000-8000-000000000000": http.StatusNotFound,
	} {
		if response := harness.devSignIn(query, nil); response.Code != status {
			t.Errorf("%s = %d, want %d", query, response.Code, status)
		}
	}
}

func TestDevSignInReplacesTheCurrentSession(t *testing.T) {
	harness := newAuthHarness(t, Options{})
	first := responseCookie(harness.devSignIn("as=owner", nil), "colony_session")
	switched := harness.devSignIn("as=new&name=Someone", first)
	if switched.Code != http.StatusSeeOther {
		t.Fatalf("switching accounts = %d", switched.Code)
	}
	if _, signedIn := harness.callerFor(first); signedIn {
		t.Fatal("the previous session still works after signing in as someone else")
	}
}

// Dev sign-in is a GET that changes state, which CrossOriginProtection doesn't cover; another site's
// link or script must not sign this browser in as someone else (or add accounts).
func TestDevSignInRefusesOtherSites(t *testing.T) {
	harness := newAuthHarness(t, Options{})
	current := responseCookie(harness.devSignIn("as=owner", nil), "colony_session")
	accountsBefore := harness.countAccounts()

	signIn := func(query string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/auth/dev?"+query, nil)
		request.AddCookie(current)
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		return harness.serve(harness.auth.DevSignIn(), request)
	}

	for name, headers := range map[string]map[string]string{
		"a link on another site":              {"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate"},
		"another port on this machine":        {"Sec-Fetch-Site": "same-site", "Sec-Fetch-Mode": "navigate"},
		"an older browser's cross-site fetch": {"Origin": "https://evil.example"},
		"a sandboxed frame":                   {"Origin": "null"},
	} {
		response := signIn("as=new&name=Spam", headers)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s: dev sign-in = %d, want 403", name, response.Code)
		}
		if cookie := responseCookie(response, "colony_session"); cookie != nil {
			t.Errorf("%s: dev sign-in set a session cookie", name)
		}
	}
	if harness.countAccounts() != accountsBefore {
		t.Fatalf("refused sign-ins added accounts (%d → %d)", accountsBefore, harness.countAccounts())
	}
	if _, signedIn := harness.callerFor(current); !signedIn {
		t.Fatal("a refused sign-in ended the browser's session")
	}

	for name, headers := range map[string]map[string]string{
		"this site's own page": {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate"},
		"the address bar":      {"Sec-Fetch-Site": "none", "Sec-Fetch-Mode": "navigate"},
		"curl":                 {},
		"a same-origin fetch":  {"Sec-Fetch-Site": "same-origin", "Origin": "http://example.com"},
	} {
		if response := signIn("as=owner", headers); response.Code != http.StatusSeeOther {
			t.Errorf("%s: dev sign-in = %d, want 303", name, response.Code)
		}
	}
}
