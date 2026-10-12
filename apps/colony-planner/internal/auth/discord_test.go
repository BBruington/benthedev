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

// A first Discord sign-in only creates the account (docs/design/multi-campaign.md §5.3): it joins no
// campaign and pings nobody. Joining is by invite link.
func TestDiscordSignInCreatesAnAccountAndPingsNobody(t *testing.T) {
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

	caller, signedIn := harness.callerFor(sessionCookie)
	if !signedIn || caller.IsSiteOwner || caller.DiscordUserID != "300000000000000003" {
		t.Fatalf("caller = %+v, signed in %v; want a non-owner account", caller, signedIn)
	}
	me, err := harness.core.AccountMe(context.Background(), caller)
	if err != nil {
		t.Fatalf("load account: %v", err)
	}
	if me.DisplayName != "Brielle" || me.DiscordUsername != "brielle" || me.AvatarURL == nil ||
		*me.AvatarURL != "https://cdn.discordapp.com/avatars/300000000000000003/abc123.png?size=128" {
		t.Fatalf("account from Discord = %+v", me)
	}
	if len(harness.memberships(caller)) != 0 || harness.notifier.count() != 0 {
		t.Fatalf("a first sign-in joined %v or pinged %d times; it only creates the account", harness.memberships(caller), harness.notifier.count())
	}

	// Signing in again refreshes the name and makes no second account.
	discord.addUser("second-code", "300000000000000003", "brielle", nil, nil)
	again := harness.signInWithDiscord("second-code")
	expectRedirect(t, again, "/")
	returning, _ := harness.callerFor(responseCookie(again, "colony_session"))
	me, _ = harness.core.AccountMe(context.Background(), returning)
	if returning.AccountID != caller.AccountID || me.DisplayName != "brielle" || me.AvatarURL != nil {
		t.Fatalf("returning account = %+v", me)
	}
	if harness.countAccounts() != 1 {
		t.Fatalf("signing in again created another account")
	}
}

func TestDiscordSignInRecognizesTheSiteOwner(t *testing.T) {
	harness, discord := newDiscordHarness(t, "http://localhost:5173")
	discord.addUser("owner-code", testOwnerDiscordID, "owner", "The Owner", nil)

	response := harness.signInWithDiscord("owner-code")
	expectRedirect(t, response, "/")
	caller, signedIn := harness.callerFor(responseCookie(response, "colony_session"))
	if !signedIn || !caller.IsSiteOwner {
		t.Fatalf("the owner's caller = %+v", caller)
	}
	if len(harness.memberships(caller)) != 0 {
		t.Fatal("being the site owner made a membership")
	}
}

// A membership's status is per campaign and signing in never changes it: a removed member signs in to
// their account and is still REMOVED there, with no ping.
func TestDiscordSignInKeepsRemovedMembersOut(t *testing.T) {
	harness, discord := newDiscordHarness(t, "http://localhost:5173")
	dm := harness.createAccount("300000000000000001", "Dee")
	campaignID, inviteCode := harness.createCampaign(dm, "Campaign", "campaign")
	removed := harness.createAccount("300000000000000004", "Gus")
	testContext := context.Background()
	joined, err := harness.core.JoinByInvite(testContext, removed, inviteCode)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	dmViewer, err := harness.core.EnterCampaign(testContext, dm, campaignID)
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	if _, err := harness.core.DenyMember(testContext, dmViewer, joined.MemberID); err != nil {
		t.Fatalf("deny: %v", err)
	}
	pings := harness.notifier.count()
	discord.addUser("removed-code", removed.DiscordUserID, "gus", "Gus", nil)

	response := harness.signInWithDiscord("removed-code")
	expectRedirect(t, response, "/")
	caller, signedIn := harness.callerFor(responseCookie(response, "colony_session"))
	if !signedIn || caller.AccountID != removed.AccountID {
		t.Fatalf("a removed member's caller = %+v; want their own account", caller)
	}
	memberships := harness.memberships(caller)
	if len(memberships) != 1 || memberships[0].Status != core.MemberStatusRemoved {
		t.Fatalf("after signing in again = %+v; want still REMOVED", memberships)
	}
	if harness.notifier.count() != pings {
		t.Fatal("a removed member signing in again pinged the DM")
	}
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
	if harness.countAccounts() != 0 {
		t.Fatal("a refused callback created an account")
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

	account := harness.createAccount("300000000000000007", "Ana")
	recorder := httptest.NewRecorder()
	session, err := harness.core.CreateSession(context.Background(), account.AccountID)
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
