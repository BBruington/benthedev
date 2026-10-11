//go:build devauth

package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

func TestDevAuthRefusesProduction(t *testing.T) {
	if _, err := New(Options{AppEnvironment: "production", PublicURL: "https://colony.example"}); err == nil {
		t.Fatal("a devauth build started with APP_ENV=production")
	}
	if _, err := New(Options{AppEnvironment: "development", PublicURL: "http://localhost:5173"}); err != nil {
		t.Fatalf("a devauth build refused development: %v", err)
	}
	if EffectiveDmDiscordID("") != core.SeedDmDiscordID {
		t.Fatal("with DM_DISCORD_ID unset, a devauth build should treat the seeded DM as the DM")
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

func (harness *authHarness) devSignInAs(query string) core.Viewer {
	harness.t.Helper()
	response := harness.devSignIn(query, nil)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		harness.t.Fatalf("dev sign-in %s answered %d → %q; body: %s", query, response.Code, response.Header().Get("Location"), response.Body.String())
	}
	viewer, signedIn := harness.viewerFor(responseCookie(response, "colony_session"))
	if !signedIn {
		harness.t.Fatalf("dev sign-in %s did not sign in", query)
	}
	return viewer
}

func TestDevSignIn(t *testing.T) {
	harness := newAuthHarness(t, Options{})

	// A fresh database has no DM; signing in as the DM creates them.
	dm := harness.devSignInAs("as=dm")
	if !dm.IsDM || dm.Status != store.MemberStatusACTIVE {
		t.Fatalf("as=dm gave %+v", dm)
	}
	if again := harness.devSignInAs("as=dm"); again.MemberID != dm.MemberID {
		t.Fatal("as=dm created a second DM")
	}
	if response := harness.devSignIn("as=player", nil); response.Code != http.StatusNotFound {
		t.Fatalf("as=player with no players = %d, want 404", response.Code)
	}

	player := harness.createMember("seed:ana", "Ana", store.MemberStatusACTIVE)
	pending := harness.createMember("seed:emil", "Emil", store.MemberStatusPENDING)
	removed := harness.createMember("seed:gus", "Gus", store.MemberStatusREMOVED)
	for _, testCase := range []struct {
		query  string
		member store.Member
		status store.MemberStatus
	}{
		{"as=player", player, store.MemberStatusACTIVE},
		{"as=pending", pending, store.MemberStatusPENDING},
		{"as=" + removed.ID.String(), removed, store.MemberStatusREMOVED},
	} {
		viewer := harness.devSignInAs(testCase.query)
		if viewer.MemberID != testCase.member.ID || viewer.Status != testCase.status || viewer.IsDM {
			t.Errorf("%s gave %+v", testCase.query, viewer)
		}
	}

	brandNew := harness.devSignInAs("as=new&name=Brielle%20Vane")
	if brandNew.Status != store.MemberStatusPENDING {
		t.Fatalf("as=new gave %+v; want a pending member", brandNew)
	}
	member, err := store.New(harness.database.Pool).GetMember(context.Background(), brandNew.MemberID)
	if err != nil || member.DisplayName != "Brielle Vane" {
		t.Fatalf("as=new member = %+v, %v", member, err)
	}
	if harness.notifier.count() != 1 {
		t.Fatalf("a brand-new dev sign-in sent %d pings, want 1 (the same path as Discord)", harness.notifier.count())
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
	harness.createMember("seed:ana", "Ana", store.MemberStatusACTIVE)
	harness.createMember("seed:emil", "Emil", store.MemberStatusPENDING)

	first := responseCookie(harness.devSignIn("as=player", nil), "colony_session")
	switched := harness.devSignIn("as=pending", first)
	if switched.Code != http.StatusSeeOther {
		t.Fatalf("switching members = %d", switched.Code)
	}
	if _, signedIn := harness.viewerFor(first); signedIn {
		t.Fatal("the previous session still works after signing in as someone else")
	}
}

func TestDevSignInRespectsThePendingCap(t *testing.T) {
	harness := newAuthHarness(t, Options{})
	for index := range core.PendingMemberLimit {
		harness.createMember(fmt.Sprintf("seed:waiting-%d", index), fmt.Sprintf("Waiting %d", index), store.MemberStatusPENDING)
	}
	response := harness.devSignIn("as=new&name=Late", nil)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login?error=full" {
		t.Fatalf("as=new past the cap = %d → %q", response.Code, response.Header().Get("Location"))
	}
}

// Dev sign-in is a GET that changes state, which CrossOriginProtection doesn't cover; another site's
// link or script must not sign this browser in as someone else (or add members and ping Beny Bot).
func TestDevSignInRefusesOtherSites(t *testing.T) {
	harness := newAuthHarness(t, Options{})
	harness.createMember("seed:ana", "Ana", store.MemberStatusACTIVE)
	current := responseCookie(harness.devSignIn("as=player", nil), "colony_session")
	membersBefore := harness.countMembers()

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
	if harness.countMembers() != membersBefore || harness.notifier.count() != 0 {
		t.Fatalf("refused sign-ins added members (%d → %d) or pinged Beny Bot (%d)", membersBefore, harness.countMembers(), harness.notifier.count())
	}
	if _, signedIn := harness.viewerFor(current); !signedIn {
		t.Fatal("a refused sign-in ended the browser's session")
	}

	for name, headers := range map[string]map[string]string{
		"this site's own page": {"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "navigate"},
		"the address bar":      {"Sec-Fetch-Site": "none", "Sec-Fetch-Mode": "navigate"},
		"curl":                 {},
		"a same-origin fetch":  {"Sec-Fetch-Site": "same-origin", "Origin": "http://example.com"},
	} {
		if response := signIn("as=player", headers); response.Code != http.StatusSeeOther {
			t.Errorf("%s: dev sign-in = %d, want 303", name, response.Code)
		}
	}
}
