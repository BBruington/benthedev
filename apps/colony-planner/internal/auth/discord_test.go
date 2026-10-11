package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

const (
	fakeClientID     = "fake-client-id"
	fakeClientSecret = "fake-client-secret"
)

// fakeDiscord answers the two calls sign-in makes: the code exchange and GET /users/@me.
type fakeDiscord struct {
	server *httptest.Server
	mutex  sync.Mutex
	// users maps an authorization code to the user Discord reports for it.
	users map[string]map[string]any
}

func newFakeDiscord(t *testing.T) *fakeDiscord {
	t.Helper()
	discord := &fakeDiscord{users: map[string]map[string]any{}}
	fakeDiscordRoutes := http.NewServeMux()
	fakeDiscordRoutes.HandleFunc("POST /api/oauth2/token", func(writer http.ResponseWriter, request *http.Request) {
		clientID, clientSecret, hasBasicAuth := request.BasicAuth()
		if !hasBasicAuth || clientID != fakeClientID || clientSecret != fakeClientSecret {
			http.Error(writer, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		if err := request.ParseForm(); err != nil || request.PostForm.Get("grant_type") != "authorization_code" ||
			request.PostForm.Get("redirect_uri") != "http://localhost:5173/auth/discord/callback" {
			http.Error(writer, `{"error":"invalid_request"}`, http.StatusBadRequest)
			return
		}
		code := request.PostForm.Get("code")
		discord.mutex.Lock()
		_, known := discord.users[code]
		discord.mutex.Unlock()
		if !known {
			http.Error(writer, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"access_token":"access-%s","token_type":"Bearer","expires_in":604800,"scope":"identify"}`, code)
	})
	fakeDiscordRoutes.HandleFunc("GET /api/v10/users/@me", func(writer http.ResponseWriter, request *http.Request) {
		code, hasPrefix := strings.CutPrefix(request.Header.Get("Authorization"), "Bearer access-")
		discord.mutex.Lock()
		user, known := discord.users[code]
		discord.mutex.Unlock()
		if !hasPrefix || !known {
			http.Error(writer, `{"message":"401: Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(user)
	})
	discord.server = httptest.NewServer(fakeDiscordRoutes)
	t.Cleanup(discord.server.Close)
	return discord
}

func (discord *fakeDiscord) addUser(code, userID, username string, globalName any, avatar any) {
	discord.mutex.Lock()
	defer discord.mutex.Unlock()
	discord.users[code] = map[string]any{"id": userID, "username": username, "global_name": globalName, "avatar": avatar}
}

func newDiscordHarness(t *testing.T, publicURL string) (*authHarness, *fakeDiscord) {
	t.Helper()
	discord := newFakeDiscord(t)
	harness := newAuthHarness(t, Options{
		PublicURL:           publicURL,
		DiscordClientID:     fakeClientID,
		DiscordClientSecret: fakeClientSecret,
		DiscordBaseURL:      discord.server.URL,
		HTTPClient:          discord.server.Client(),
	})
	return harness, discord
}

// startLogin follows GET /auth/discord/login and returns the state cookie and the state Discord echoes.
func (harness *authHarness) startLogin() (*http.Cookie, string) {
	harness.t.Helper()
	response := harness.serve(harness.auth.DiscordLogin(), httptest.NewRequest(http.MethodGet, "/auth/discord/login", nil))
	if response.Code != http.StatusFound {
		harness.t.Fatalf("login status = %d, want 302", response.Code)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		harness.t.Fatalf("login location: %v", err)
	}
	query := location.Query()
	for name, want := range map[string]string{
		"client_id":     fakeClientID,
		"scope":         "identify",
		"response_type": "code",
		"redirect_uri":  harness.auth.discordOAuth.RedirectURL,
		"prompt":        "none",
	} {
		if got := query.Get(name); got != want {
			harness.t.Errorf("authorize URL %s = %q, want %q", name, got, want)
		}
	}
	stateCookie := responseCookie(response, harness.auth.oauthStateCookieName)
	if stateCookie == nil || stateCookie.Value != query.Get("state") || query.Get("state") == "" {
		harness.t.Fatalf("the state cookie does not match the authorize URL's state")
	}
	if !stateCookie.HttpOnly || stateCookie.SameSite != http.SameSiteLaxMode || stateCookie.MaxAge != 600 || stateCookie.Path != "/" {
		harness.t.Errorf("state cookie attributes: %+v", stateCookie)
	}
	return stateCookie, query.Get("state")
}

func (harness *authHarness) callback(stateCookie *http.Cookie, query string) *httptest.ResponseRecorder {
	harness.t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/auth/discord/callback?"+query, nil)
	if stateCookie != nil {
		request.AddCookie(stateCookie)
	}
	return harness.serve(harness.auth.DiscordCallback(), request)
}

// signInWithDiscord runs the whole flow for code and returns the callback response.
func (harness *authHarness) signInWithDiscord(code string) *httptest.ResponseRecorder {
	harness.t.Helper()
	stateCookie, state := harness.startLogin()
	return harness.callback(stateCookie, url.Values{"code": {code}, "state": {state}}.Encode())
}

func expectRedirect(t *testing.T, response *httptest.ResponseRecorder, location string) {
	t.Helper()
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != location {
		t.Fatalf("callback answered %d → %q, want 303 → %q", response.Code, response.Header().Get("Location"), location)
	}
}

func TestDiscordSignInCreatesAPendingMemberAndPingsOnce(t *testing.T) {
	harness, discord := newDiscordHarness(t, "http://localhost:5173")
	discord.addUser("first-code", "300000000000000003", "brielle", "Brielle", "abc123")

	response := harness.signInWithDiscord("first-code")
	expectRedirect(t, response, "/")
	if cleared := responseCookie(response, "oauth_state"); cleared == nil || cleared.MaxAge >= 0 {
		t.Errorf("the state cookie was not cleared: %+v", cleared)
	}
	sessionCookie := responseCookie(response, "colony_session")
	if sessionCookie == nil {
		t.Fatal("no session cookie")
	}
	if !sessionCookie.HttpOnly || sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteLaxMode || sessionCookie.Path != "/" {
		t.Errorf("session cookie attributes over http: %+v", sessionCookie)
	}
	if sessionCookie.MaxAge < int(core.SessionLifetime.Seconds())-60 {
		t.Errorf("session cookie Max-Age = %d, want about 30 days", sessionCookie.MaxAge)
	}

	viewer, signedIn := harness.viewerFor(sessionCookie)
	if !signedIn || viewer.Status != store.MemberStatusPENDING || viewer.IsDM {
		t.Fatalf("viewer = %+v, signed in %v; want a pending non-DM", viewer, signedIn)
	}
	member, err := store.New(harness.database.Pool).GetMember(context.Background(), viewer.MemberID)
	if err != nil {
		t.Fatalf("load member: %v", err)
	}
	if member.DisplayName != "Brielle" || member.DiscordUsername != "brielle" || member.AvatarUrl == nil ||
		*member.AvatarUrl != "https://cdn.discordapp.com/avatars/300000000000000003/abc123.png?size=128" {
		t.Fatalf("member from Discord = %+v", member)
	}
	if harness.notifier.count() != 1 {
		t.Fatalf("pings = %d, want 1", harness.notifier.count())
	}

	// Signing in again keeps the status, refreshes the name, and pings nobody.
	discord.addUser("second-code", "300000000000000003", "brielle", nil, nil)
	expectRedirect(t, harness.signInWithDiscord("second-code"), "/")
	if harness.notifier.count() != 1 {
		t.Fatalf("signing in again pinged the DM: %d pings", harness.notifier.count())
	}
	member, _ = store.New(harness.database.Pool).GetMember(context.Background(), viewer.MemberID)
	if member.Status != store.MemberStatusPENDING || member.DisplayName != "brielle" || member.AvatarUrl != nil {
		t.Fatalf("returning member = %+v", member)
	}
	if harness.countMembers() != 1 {
		t.Fatalf("signing in again created another member")
	}
}

func TestDiscordSignInMakesTheDmActive(t *testing.T) {
	harness, discord := newDiscordHarness(t, "http://localhost:5173")
	discord.addUser("dm-code", testDmDiscordID, "thedm", "The DM", nil)

	response := harness.signInWithDiscord("dm-code")
	expectRedirect(t, response, "/")
	viewer, signedIn := harness.viewerFor(responseCookie(response, "colony_session"))
	if !signedIn || !viewer.IsDM || viewer.Status != store.MemberStatusACTIVE {
		t.Fatalf("the DM's viewer = %+v", viewer)
	}
	if harness.notifier.count() != 0 {
		t.Fatal("the DM signing in pinged the DM channel")
	}
}

func TestDiscordSignInKeepsRemovedMembersOut(t *testing.T) {
	harness, discord := newDiscordHarness(t, "http://localhost:5173")
	removed := harness.createMember("300000000000000004", "Gus", store.MemberStatusREMOVED)
	discord.addUser("removed-code", removed.DiscordUserID, "gus", "Gus", nil)

	response := harness.signInWithDiscord("removed-code")
	expectRedirect(t, response, "/")
	viewer, signedIn := harness.viewerFor(responseCookie(response, "colony_session"))
	if !signedIn || viewer.MemberID != removed.ID || viewer.Status != store.MemberStatusREMOVED {
		t.Fatalf("a removed member's viewer = %+v; want their own REMOVED member", viewer)
	}
	if harness.notifier.count() != 0 {
		t.Fatal("a removed member signing in again pinged the DM")
	}
}

func TestDiscordSignInStopsAtThePendingCap(t *testing.T) {
	harness, discord := newDiscordHarness(t, "http://localhost:5173")
	for index := range core.PendingMemberLimit {
		harness.createMember(fmt.Sprintf("40000000000000%04d", index), fmt.Sprintf("Waiting %d", index), store.MemberStatusPENDING)
	}
	discord.addUser("late-code", "300000000000000005", "late", "Late", nil)
	discord.addUser("dm-code", testDmDiscordID, "thedm", "The DM", nil)

	expectRedirect(t, harness.signInWithDiscord("late-code"), "/login?error=full")
	if harness.countMembers() != int64(core.PendingMemberLimit) {
		t.Fatal("a sign-in past the pending cap created a member")
	}
	if harness.notifier.count() != 0 {
		t.Fatal("a refused sign-in pinged the DM")
	}
	expectRedirect(t, harness.signInWithDiscord("dm-code"), "/")
}

func TestDiscordCallbackRefusals(t *testing.T) {
	harness, discord := newDiscordHarness(t, "http://localhost:5173")
	discord.addUser("good-code", "300000000000000006", "someone", nil, nil)

	stateCookie, state := harness.startLogin()
	for _, testCase := range []struct {
		name   string
		cookie *http.Cookie
		query  url.Values
	}{
		{"a state that doesn't match the cookie", stateCookie, url.Values{"code": {"good-code"}, "state": {"forged"}}},
		{"no state cookie", nil, url.Values{"code": {"good-code"}, "state": {state}}},
		{"no state at all", stateCookie, url.Values{"code": {"good-code"}}},
		{"the person declined on Discord", stateCookie, url.Values{"error": {"access_denied"}, "state": {state}}},
		{"a code Discord rejects", stateCookie, url.Values{"code": {"bad-code"}, "state": {state}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := harness.callback(testCase.cookie, testCase.query.Encode())
			expectRedirect(t, response, "/login?error=discord")
			if responseCookie(response, "colony_session") != nil {
				t.Fatal("a refused callback set a session cookie")
			}
		})
	}
	if harness.countMembers() != 0 {
		t.Fatal("a refused callback created a member")
	}
}

func TestDiscordSignInOverHTTPSUsesHostPrefixedSecureCookies(t *testing.T) {
	discord := newFakeDiscord(t)
	harness := newAuthHarness(t, Options{
		PublicURL:           "https://colony.example",
		DiscordClientID:     fakeClientID,
		DiscordClientSecret: fakeClientSecret,
		DiscordBaseURL:      discord.server.URL,
		HTTPClient:          discord.server.Client(),
	})
	if harness.auth.SessionCookieName() != "__Host-colony_session" || harness.auth.oauthStateCookieName != "__Host-oauth_state" {
		t.Fatalf("cookie names over https: %s, %s", harness.auth.SessionCookieName(), harness.auth.oauthStateCookieName)
	}
	response := harness.serve(harness.auth.DiscordLogin(), httptest.NewRequest(http.MethodGet, "/auth/discord/login", nil))
	stateCookie := responseCookie(response, "__Host-oauth_state")
	if stateCookie == nil || !stateCookie.Secure || stateCookie.Path != "/" || stateCookie.Domain != "" {
		t.Fatalf("state cookie over https: %+v", stateCookie)
	}

	member := harness.createMember("300000000000000007", "Ana", store.MemberStatusACTIVE)
	recorder := httptest.NewRecorder()
	session, err := harness.core.CreateSession(context.Background(), member.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	harness.auth.setSessionCookie(recorder, session.Token, session.ExpiresAt)
	sessionCookie := responseCookie(recorder, "__Host-colony_session")
	if sessionCookie == nil || !sessionCookie.Secure || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie over https: %+v", sessionCookie)
	}
}

func TestDiscordRoutesAreOffWithoutCredentials(t *testing.T) {
	harness := newAuthHarness(t, Options{})
	if harness.auth.DiscordEnabled() {
		t.Fatal("Discord sign-in is enabled without credentials")
	}
	for _, handler := range []http.Handler{harness.auth.DiscordLogin(), harness.auth.DiscordCallback()} {
		response := harness.serve(handler, httptest.NewRequest(http.MethodGet, "/auth/discord/callback?code=x&state=y", nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	}
}
