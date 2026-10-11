package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

func decodeMe(t *testing.T, body string) core.Me {
	t.Helper()
	var me core.Me
	if err := json.Unmarshal([]byte(body), &me); err != nil {
		t.Fatalf("decode me: %v; body: %s", err, body)
	}
	return me
}

func memberPathFor(member store.Member, action string) string {
	return "/api/members/" + member.ID.String() + "/" + action
}

func TestRemovalLocksOutAtOnceAndClosesStreams(t *testing.T) {
	testHarness := newHarness(t)
	dmToken := testHarness.signIn(testHarness.dm.ID)
	playerToken := testHarness.signIn(testHarness.player.ID)
	otherDeviceToken := testHarness.signIn(testHarness.player.ID)

	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: playerToken}), http.StatusOK)
	stream := testHarness.openStream(playerToken, "")
	if _, received := stream.nextMessage(2 * time.Second); !received {
		t.Fatal("no catch-up message on connect")
	}

	testHarness.expectStatus(testHarness.do(http.MethodPost, memberPathFor(testHarness.player, "remove"), requestOptions{token: dmToken}), http.StatusOK)

	if !stream.waitForEnd(2 * time.Second) {
		t.Fatal("the removed member's stream stayed open")
	}
	for _, token := range []string{playerToken, otherDeviceToken} {
		response := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: token})
		testHarness.expectStatus(response, http.StatusUnauthorized)
		if !strings.Contains(response.Header().Get("Set-Cookie"), "Max-Age=0") {
			t.Errorf("a dead session's cookie is not cleared: %q", response.Header().Get("Set-Cookie"))
		}
	}

	// Signing in again keeps them REMOVED, gives only pending-level access, and pings nobody.
	memberID, err := testHarness.core.SignIn(context.Background(), core.DiscordIdentity{
		UserID:   testHarness.player.DiscordUserID,
		Username: testHarness.player.DiscordUsername,
	})
	if err != nil {
		t.Fatalf("sign in again: %v", err)
	}
	if memberID != testHarness.player.ID {
		t.Fatalf("signing in again made a new member")
	}
	if len(testHarness.notifier.waiting) != 0 {
		t.Fatalf("a removed member signing in again pinged the DM: %v", testHarness.notifier.waiting)
	}
	returningToken := testHarness.signIn(memberID)
	me := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: returningToken})
	testHarness.expectStatus(me, http.StatusOK)
	if status := decodeMe(t, me.Body.String()).Status; status != store.MemberStatusREMOVED {
		t.Fatalf("status after signing in again = %s, want REMOVED", status)
	}
	testHarness.expectStatus(testHarness.do(http.MethodPut, "/api/me/profile", requestOptions{token: returningToken, body: `{"characterName":"Back"}`}), http.StatusForbidden)
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/auth/logout-everywhere", requestOptions{token: returningToken}), http.StatusForbidden)

	// Re-admitting lets the same session in at once: the cache entry was evicted.
	testHarness.expectStatus(testHarness.do(http.MethodPost, memberPathFor(testHarness.player, "readmit"), requestOptions{token: dmToken}), http.StatusOK)
	me = testHarness.do(http.MethodGet, "/api/me", requestOptions{token: returningToken})
	if status := decodeMe(t, me.Body.String()).Status; status != store.MemberStatusACTIVE {
		t.Fatalf("status after re-admission = %s, want ACTIVE", status)
	}
	testHarness.expectStatus(testHarness.do(http.MethodPut, "/api/me/profile", requestOptions{token: returningToken, body: `{"characterName":"Back"}`}), http.StatusOK)
}

func TestPendingPageOpensLiveOnApproval(t *testing.T) {
	testHarness := newHarness(t)
	dmToken := testHarness.signIn(testHarness.dm.ID)
	pendingToken := testHarness.signIn(testHarness.pending.ID)

	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: pendingToken}), http.StatusOK)
	stream := testHarness.openStream(pendingToken, "")
	catchUp, received := stream.nextMessage(2 * time.Second)
	if !received || catchUp.data != `{"topics":["me"]}` {
		t.Fatalf("pending catch-up = %+v, want only the me topic", catchUp)
	}

	settingsBody := `{"campaignName":"Renamed","colonyName":"Port Eldari","timezone":"UTC","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null}`
	testHarness.expectStatus(testHarness.do(http.MethodPut, "/api/settings", requestOptions{token: dmToken, body: settingsBody}), http.StatusOK)
	if message, received := stream.nextMessage(300 * time.Millisecond); received {
		t.Fatalf("a pending stream heard a broadcast: %+v", message)
	}

	testHarness.expectStatus(testHarness.do(http.MethodPost, memberPathFor(testHarness.pending, "approve"), requestOptions{token: dmToken}), http.StatusOK)
	message, received := stream.nextMessage(2 * time.Second)
	if !received || message.data != `{"topics":["me"]}` {
		t.Fatalf("approval sent %+v, want the me topic", message)
	}
	me := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: pendingToken})
	if status := decodeMe(t, me.Body.String()).Status; status != store.MemberStatusACTIVE {
		t.Fatalf("status after approval = %s, want ACTIVE", status)
	}

	// The same stream now hears everything an active member hears, starting with the members list.
	message, received = stream.nextMessage(2 * time.Second)
	if !received || message.data != `{"topics":["members"]}` {
		t.Fatalf("after approval the stream got %+v, want the members topic", message)
	}
	testHarness.expectStatus(testHarness.do(http.MethodPut, "/api/settings", requestOptions{token: dmToken, body: settingsBody}), http.StatusOK)
	message, received = stream.nextMessage(2 * time.Second)
	// The board carries the campaign and colony names, so a settings change also invalidates it.
	if !received || message.data != `{"topics":["settings","colony"]}` {
		t.Fatalf("after approval the stream got %+v, want the settings and colony topics", message)
	}
}

func TestDenyShowsNoAccessLive(t *testing.T) {
	testHarness := newHarness(t)
	dmToken := testHarness.signIn(testHarness.dm.ID)
	pendingToken := testHarness.signIn(testHarness.pending.ID)
	stream := testHarness.openStream(pendingToken, "")
	stream.nextMessage(2 * time.Second)

	testHarness.expectStatus(testHarness.do(http.MethodPost, memberPathFor(testHarness.pending, "deny"), requestOptions{token: dmToken}), http.StatusOK)
	if message, received := stream.nextMessage(2 * time.Second); !received || message.data != `{"topics":["me"]}` {
		t.Fatalf("deny sent %+v, want the me topic", message)
	}
	me := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: pendingToken})
	testHarness.expectStatus(me, http.StatusOK)
	if status := decodeMe(t, me.Body.String()).Status; status != store.MemberStatusREMOVED {
		t.Fatalf("status after deny = %s, want REMOVED", status)
	}
}

func TestMemberDecisions(t *testing.T) {
	testHarness := newHarness(t)
	dmToken := testHarness.signIn(testHarness.dm.ID)
	dmOptions := requestOptions{token: dmToken}

	list := testHarness.do(http.MethodGet, "/api/members", dmOptions)
	testHarness.expectStatus(list, http.StatusOK)
	var groups core.MemberGroups
	if err := json.Unmarshal(list.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(groups.Active) != 3 || len(groups.Pending) != 1 || len(groups.Removed) != 1 {
		t.Fatalf("groups = %d active, %d pending, %d removed; want 3, 1, 1", len(groups.Active), len(groups.Pending), len(groups.Removed))
	}
	if groups.Active[0].Dm == nil || !groups.Active[0].Dm.IsDM {
		t.Fatalf("the DM should be listed first among the active, flagged: %+v", groups.Active[0])
	}

	for _, testCase := range []struct {
		name   string
		member store.Member
		action string
		status int
	}{
		{"approving a removed member asks for re-admission", testHarness.removed, "approve", http.StatusConflict},
		{"denying an active member asks for removal", testHarness.player, "deny", http.StatusConflict},
		{"re-admitting a pending member asks for approval", testHarness.pending, "readmit", http.StatusConflict},
		{"the DM can't be removed", testHarness.dm, "remove", http.StatusForbidden},
		{"approving twice is harmless", testHarness.player, "approve", http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			testHarness.expectStatus(testHarness.do(http.MethodPost, memberPathFor(testCase.member, testCase.action), dmOptions), testCase.status)
		})
	}
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/members/6f1c7a2e-0000-4000-8000-000000000000/approve", dmOptions), http.StatusNotFound)

	rename := testHarness.do(http.MethodPut, memberPathFor(testHarness.otherPlayer, "character-name"), requestOptions{token: dmToken, body: `{"characterName":"  Sela Windmere  "}`})
	testHarness.expectStatus(rename, http.StatusOK)
	if !strings.Contains(rename.Body.String(), `"characterName":"Sela Windmere"`) {
		t.Fatalf("character name not trimmed: %s", rename.Body.String())
	}
	tooLong := testHarness.do(http.MethodPut, memberPathFor(testHarness.otherPlayer, "character-name"), requestOptions{token: dmToken, body: `{"characterName":"` + strings.Repeat("x", 61) + `"}`})
	testHarness.expectStatus(tooLong, http.StatusUnprocessableEntity)
	if !strings.Contains(tooLong.Body.String(), "body.characterName") {
		t.Fatalf("422 lacks the field location: %s", tooLong.Body.String())
	}

	playerToken := testHarness.signIn(testHarness.player.ID)
	profile := testHarness.do(http.MethodPut, "/api/me/profile", requestOptions{token: playerToken, body: `{"characterName":null}`})
	testHarness.expectStatus(profile, http.StatusOK)
	me := decodeMe(t, profile.Body.String())
	if me.Member.CharacterName != nil || me.Member.Dm != nil {
		t.Fatalf("profile reply = %+v; want a cleared name and no dm object", me)
	}
}

func TestViewAsPlayer(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	dmToken := testHarness.signIn(testHarness.dm.ID)
	otherDeviceToken := testHarness.signIn(testHarness.dm.ID)
	dmOptions := requestOptions{token: dmToken}

	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/members", dmOptions), http.StatusOK)
	// The switch ran under the DM projection, so it answers with no body at all (spec §2.1: while view
	// as player is on, every answer is the player projection); the client reads /api/me again.
	enabled := testHarness.do(http.MethodPost, "/api/me/view-as-player", requestOptions{token: dmToken, body: `{"enabled":true}`})
	testHarness.expectStatus(enabled, http.StatusNoContent)
	if enabled.Body.Len() != 0 {
		t.Fatalf("turning view as player on answered %q; want no body", enabled.Body.String())
	}

	me := testHarness.do(http.MethodGet, "/api/me", dmOptions)
	testHarness.expectStatus(me, http.StatusOK)
	decoded := decodeMe(t, me.Body.String())
	if !decoded.IsDM || !decoded.ViewAsPlayer || decoded.Member.Dm != nil {
		t.Fatalf("me while viewing as player = %+v; want isDm, viewAsPlayer and no dm object", decoded)
	}
	settingsBody := `{"campaignName":"Renamed","colonyName":"Port Eldari","timezone":"UTC","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null}`
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/members", ""},
		{http.MethodGet, "/api/settings", ""},
		{http.MethodPut, "/api/settings", settingsBody},
		{http.MethodPost, memberPathFor(testHarness.pending, "approve"), ""},
		{http.MethodGet, "/api/openapi.json", ""},
	} {
		response := testHarness.do(request.method, request.path, requestOptions{token: dmToken, body: request.body})
		if response.Code != http.StatusForbidden {
			t.Errorf("%s %s while viewing as player = %d, want 403", request.method, request.path, response.Code)
		}
		if strings.Contains(response.Body.String(), numericCanary) {
			t.Errorf("%s %s leaked settings while viewing as player", request.method, request.path)
		}
	}

	// View as player belongs to one browser session; the DM's other device is unaffected.
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/members", requestOptions{token: otherDeviceToken}), http.StatusOK)

	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/me/view-as-player", requestOptions{token: dmToken, body: `{"enabled":false}`}), http.StatusNoContent)
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/members", dmOptions), http.StatusOK)

	playerToken := testHarness.signIn(testHarness.player.ID)
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/me/view-as-player", requestOptions{token: playerToken, body: `{"enabled":true}`}), http.StatusForbidden)
}

func TestSessionCacheAvoidsQueriesAndIsEvicted(t *testing.T) {
	testHarness := newHarness(t)
	playerToken := testHarness.signIn(testHarness.player.ID)
	queries := testHarness.database.Queries

	testHarness.expectStatus(testHarness.do(http.MethodGet, "/healthz", requestOptions{token: playerToken}), http.StatusOK)
	queries.Reset()
	for range 3 {
		testHarness.expectStatus(testHarness.do(http.MethodGet, "/healthz", requestOptions{token: playerToken}), http.StatusOK)
	}
	if count := queries.Count(); count != 0 {
		t.Fatalf("repeat requests ran %d queries; a cached session should run none", count)
	}

	// A reconnect with an up-to-date ID sends nothing and runs no query.
	first := testHarness.openStream(playerToken, "")
	catchUp, received := first.nextMessage(2 * time.Second)
	if !received {
		t.Fatal("no catch-up message on the first connect")
	}
	first.cancel()
	queries.Reset()
	reconnected := testHarness.openStream(playerToken, catchUp.id)
	if message, received := reconnected.nextMessage(300 * time.Millisecond); received {
		t.Fatalf("an up-to-date reconnect got %+v; want nothing", message)
	}
	if count := queries.Count(); count != 0 {
		t.Fatalf("a reconnect ran %d queries; want none", count)
	}
	reconnected.cancel()

	// A change made behind core's back stays invisible until an operation evicts the entry.
	if _, err := testHarness.database.Pool.Exec(context.Background(), "UPDATE member SET status = 'PENDING' WHERE id = $1", testHarness.player.ID); err != nil {
		t.Fatalf("change status directly: %v", err)
	}
	me := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: playerToken})
	if status := decodeMe(t, me.Body.String()).Status; status != store.MemberStatusACTIVE {
		t.Fatalf("status = %s; the cached session should still say ACTIVE", status)
	}
	dmToken := testHarness.signIn(testHarness.dm.ID)
	testHarness.expectStatus(testHarness.do(http.MethodPost, memberPathFor(testHarness.player, "deny"), requestOptions{token: dmToken}), http.StatusOK)
	me = testHarness.do(http.MethodGet, "/api/me", requestOptions{token: playerToken})
	if status := decodeMe(t, me.Body.String()).Status; status != store.MemberStatusREMOVED {
		t.Fatalf("status after deny = %s; deny should have evicted the cached session", status)
	}
}

// `mise run reset` under a running server deletes every member and session the cache still holds. The
// first request that notices signs everyone out, instead of leaving a stale session able to act or a
// browser stuck on an error screen.
func TestSessionsEndWhenTheDatabaseIsReset(t *testing.T) {
	testHarness := newHarness(t)
	dmToken := testHarness.signIn(testHarness.dm.ID)
	otherDeviceToken := testHarness.signIn(testHarness.dm.ID)
	playerToken := testHarness.signIn(testHarness.player.ID)
	for _, token := range []string{dmToken, otherDeviceToken, playerToken} {
		testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: token}), http.StatusOK)
	}

	if _, err := testHarness.database.Pool.Exec(context.Background(), "DELETE FROM member"); err != nil {
		t.Fatalf("simulate a reset: %v", err)
	}
	me := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: dmToken})
	testHarness.expectStatus(me, http.StatusUnauthorized)

	settingsBody := `{"campaignName":"Stale session wrote this","colonyName":"Port Eldari","timezone":"UTC","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null}`
	testHarness.expectStatus(testHarness.do(http.MethodPut, "/api/settings", requestOptions{token: otherDeviceToken, body: settingsBody}), http.StatusUnauthorized)
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/members", requestOptions{token: dmToken}), http.StatusUnauthorized)
	cleared := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: playerToken})
	testHarness.expectStatus(cleared, http.StatusUnauthorized)
	if !strings.Contains(cleared.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("the stale cookie was not cleared: %q", cleared.Header().Get("Set-Cookie"))
	}
	settings, err := store.New(testHarness.database.Pool).GetSettings(context.Background())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if settings.CampaignName == "Stale session wrote this" {
		t.Fatal("a session from before the reset wrote settings")
	}
}

func TestSignOut(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.signIn(testHarness.player.ID)
	otherDeviceToken := testHarness.signIn(testHarness.player.ID)
	stream := testHarness.openStream(token, "")
	stream.nextMessage(2 * time.Second)

	response := testHarness.do(http.MethodPost, "/auth/logout", requestOptions{token: token})
	testHarness.expectStatus(response, http.StatusNoContent)
	if !strings.Contains(response.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("sign out did not clear the cookie: %q", response.Header().Get("Set-Cookie"))
	}
	if !stream.waitForEnd(2 * time.Second) {
		t.Fatal("sign out left the session's stream open")
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: token}), http.StatusUnauthorized)
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: otherDeviceToken}), http.StatusOK)

	// A pending member may sign out too.
	pendingToken := testHarness.signIn(testHarness.pending.ID)
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/auth/logout", requestOptions{token: pendingToken}), http.StatusNoContent)
}

func TestSignOutEverywhere(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.signIn(testHarness.player.ID)
	otherDeviceToken := testHarness.signIn(testHarness.player.ID)
	stream := testHarness.openStream(otherDeviceToken, "")
	stream.nextMessage(2 * time.Second)

	testHarness.expectStatus(testHarness.do(http.MethodPost, "/auth/logout-everywhere", requestOptions{token: token}), http.StatusNoContent)
	if !stream.waitForEnd(2 * time.Second) {
		t.Fatal("sign out everywhere left another device's stream open")
	}
	for _, signedOut := range []string{token, otherDeviceToken} {
		testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: signedOut}), http.StatusUnauthorized)
	}
	otherPlayerToken := testHarness.signIn(testHarness.otherPlayer.ID)
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: otherPlayerToken}), http.StatusOK)
}

func TestCrossSiteRequestsAreRefused(t *testing.T) {
	testHarness := newHarness(t)
	dmToken := testHarness.signIn(testHarness.dm.ID)
	enable := `{"enabled":true}`

	for _, testCase := range []struct {
		name    string
		headers map[string]string
		body    string
		status  int
	}{
		{"a cross-site fetch", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, enable, http.StatusForbidden},
		{"an old browser with only an Origin", map[string]string{"Origin": "https://evil.example"}, enable, http.StatusForbidden},
		{"a form post", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "enabled=true", http.StatusUnsupportedMediaType},
		{"a text/plain post", map[string]string{"Content-Type": "text/plain"}, enable, http.StatusUnsupportedMediaType},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := testHarness.do(http.MethodPost, "/api/me/view-as-player", requestOptions{token: dmToken, body: testCase.body, headers: testCase.headers})
			testHarness.expectStatus(response, testCase.status)
		})
	}
	members := testHarness.do(http.MethodGet, "/api/members", requestOptions{token: dmToken})
	testHarness.expectStatus(members, http.StatusOK)

	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": testPublicURL}
	request := requestOptions{token: dmToken, body: enable, headers: sameOrigin}
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/me/view-as-player", request), http.StatusNoContent)

	crossSiteRead := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: dmToken, headers: map[string]string{"Sec-Fetch-Site": "cross-site"}})
	testHarness.expectStatus(crossSiteRead, http.StatusOK)
}

func TestLoginOptions(t *testing.T) {
	testHarness := newHarness(t)
	response := testHarness.do(http.MethodGet, "/api/login-options", requestOptions{})
	testHarness.expectStatus(response, http.StatusOK)
	var options core.LoginOptions
	if err := json.Unmarshal(response.Body.Bytes(), &options); err != nil {
		t.Fatalf("decode login options: %v", err)
	}
	if options.DiscordEnabled {
		t.Fatal("Discord sign-in is offered without Discord credentials")
	}
	if !auth.DevAuthEnabled {
		if options.DevSignIn != nil {
			t.Fatalf("a production build lists dev sign-in members: %+v", options.DevSignIn)
		}
		return
	}
	if len(options.DevSignIn) != 5 || !options.DevSignIn[0].IsDM {
		t.Fatalf("dev sign-in = %+v; want all five members, the DM first", options.DevSignIn)
	}
}

// Core owns validation messages: the schema carries no length or pattern rules that would answer
// first in the schema validator's words (and before core trims).
func TestValidationMessagesComeFromCore(t *testing.T) {
	testHarness := newHarness(t)
	dmOptions := requestOptions{token: testHarness.signIn(testHarness.dm.ID)}

	for _, testCase := range []struct {
		method, path, body, want string
	}{
		{http.MethodPut, "/api/settings", `{"campaignName":"","colonyName":"Port Eldari","timezone":"UTC","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null}`, "Give the campaign a name."},
		{http.MethodPut, "/api/settings", `{"campaignName":"Colony","colonyName":"Port Eldari","timezone":"UTC","discordAnnounceChannelId":"12ab","discordReminderChannelId":null,"discordDmChannelId":null}`, "A channel ID is 17 to 20 digits."},
		{http.MethodPut, "/api/me/profile", `{"characterName":"x\u0000y"}`, "Remove the tabs, line breaks or other control characters."},
	} {
		dmOptions.body = testCase.body
		response := testHarness.do(testCase.method, testCase.path, dmOptions)
		testHarness.expectStatus(response, http.StatusUnprocessableEntity)
		if !strings.Contains(response.Body.String(), testCase.want) {
			t.Errorf("%s %s %s: body %s, want %q", testCase.method, testCase.path, testCase.body, response.Body.String(), testCase.want)
		}
	}

	dmOptions.body = `{"characterName":"  ` + strings.Repeat("a", 60) + `  "}`
	testHarness.expectStatus(testHarness.do(http.MethodPut, "/api/me/profile", dmOptions), http.StatusOK)
}
