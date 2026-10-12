package app

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

func decodeCampaignMe(t *testing.T, body string) core.CampaignMe {
	t.Helper()
	var me core.CampaignMe
	if err := json.Unmarshal([]byte(body), &me); err != nil {
		t.Fatalf("decode campaign me: %v; body: %s", err, body)
	}
	return me
}

func memberActionPath(campaign *campaignFixture, accountName string, action string) string {
	return campaignPathOf(campaign, "/members/"+campaign.members[accountName].String()+"/"+action)
}

const settingsBodyA = `{"campaignName":"Campaign Alpha","colonyName":"Port Alpha","timezone":"UTC","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null}`

// Removal is per campaign (docs/design/multi-campaign.md §1.2): Ana removed from B loses B at once, her
// B tab drops to the no-access screen live, and her session and her campaign A keep working.
func TestRemovalIsPerCampaign(t *testing.T) {
	testHarness := newHarness(t)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	dmToken := testHarness.signIn(testHarness.dmB)
	anaToken := testHarness.signIn(testHarness.ana)

	streamB := testHarness.openStream(campaignB, anaToken, "")
	streamA := testHarness.openStream(campaignA, anaToken, "")
	for _, stream := range []*liveStream{streamA, streamB} {
		if _, received := stream.nextMessage(2 * time.Second); !received {
			t.Fatal("no catch-up message on connect")
		}
	}

	testHarness.expectStatus(testHarness.do(http.MethodPost, memberActionPath(campaignB, "ana", "remove"), requestOptions{token: dmToken}), http.StatusOK)

	if message, received := streamB.nextMessage(2 * time.Second); !received || message.data != `{"topics":["me"]}` {
		t.Fatalf("removal sent %+v to Ana's B tab; want the me topic", message)
	}
	if message, received := streamA.nextMessage(300 * time.Millisecond); received {
		t.Fatalf("Ana's A tab heard B's removal: %+v", message)
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignB, "/colony"), requestOptions{token: anaToken}), http.StatusForbidden)
	me := testHarness.do(http.MethodGet, campaignPathOf(campaignB, "/me"), requestOptions{token: anaToken})
	testHarness.expectStatus(me, http.StatusOK)
	if status := decodeCampaignMe(t, me.Body.String()).Status; status != core.MemberStatusRemoved {
		t.Fatalf("Ana's status in B = %s, want REMOVED", status)
	}
	// Her session and campaign A are untouched.
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: anaToken}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/colony"), requestOptions{token: anaToken}), http.StatusOK)

	// The invite link can't bring her back, and pings nobody.
	pings := testHarness.notifier.waitingCount()
	join := testHarness.do(http.MethodPost, "/api/invites/"+campaignB.inviteCode+"/join", requestOptions{token: anaToken})
	testHarness.expectStatus(join, http.StatusOK)
	var joined core.MyCampaign
	if err := json.Unmarshal(join.Body.Bytes(), &joined); err != nil || joined.Status != core.MemberStatusRemoved {
		t.Fatalf("joining again = %+v, %v; want the membership unchanged, REMOVED", joined, err)
	}
	if testHarness.notifier.waitingCount() != pings {
		t.Fatal("a removed member opening the invite link pinged the DM")
	}

	// Re-admitting lets the same session in at once: there is no membership cache.
	testHarness.expectStatus(testHarness.do(http.MethodPost, memberActionPath(campaignB, "ana", "readmit"), requestOptions{token: dmToken}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignB, "/colony"), requestOptions{token: anaToken}), http.StatusOK)
}

func TestPendingPageOpensLiveOnApproval(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	dmToken := testHarness.signIn(testHarness.dmA)
	pendingToken := testHarness.signIn(testHarness.pendingA)

	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/me"), requestOptions{token: pendingToken}), http.StatusOK)
	stream := testHarness.openStream(campaignA, pendingToken, "")
	catchUp, received := stream.nextMessage(2 * time.Second)
	if !received || catchUp.data != `{"topics":["me"]}` {
		t.Fatalf("pending catch-up = %+v, want only the me topic", catchUp)
	}

	settingsPath := campaignPathOf(campaignA, "/settings")
	testHarness.expectStatus(testHarness.do(http.MethodPut, settingsPath, requestOptions{token: dmToken, body: settingsBodyA}), http.StatusOK)
	if message, received := stream.nextMessage(300 * time.Millisecond); received {
		t.Fatalf("a pending stream heard a broadcast: %+v", message)
	}

	testHarness.expectStatus(testHarness.do(http.MethodPost, memberActionPath(campaignA, "pendingA", "approve"), requestOptions{token: dmToken}), http.StatusOK)
	message, received := stream.nextMessage(2 * time.Second)
	if !received || message.data != `{"topics":["me"]}` {
		t.Fatalf("approval sent %+v, want the me topic", message)
	}
	me := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/me"), requestOptions{token: pendingToken})
	if status := decodeCampaignMe(t, me.Body.String()).Status; status != core.MemberStatusActive {
		t.Fatalf("status after approval = %s, want ACTIVE", status)
	}

	// The same stream now hears everything an active member hears, starting with the members list.
	message, received = stream.nextMessage(2 * time.Second)
	if !received || message.data != `{"topics":["members"]}` {
		t.Fatalf("after approval the stream got %+v, want the members topic", message)
	}
	testHarness.expectStatus(testHarness.do(http.MethodPut, settingsPath, requestOptions{token: dmToken, body: settingsBodyA}), http.StatusOK)
	message, received = stream.nextMessage(2 * time.Second)
	// The board and each member's .../me carry the campaign's names, so a settings change also
	// invalidates them.
	if !received || message.data != `{"topics":["settings","colony","me"]}` {
		t.Fatalf("after approval the stream got %+v, want the settings, colony and me topics", message)
	}
}

func TestDenyShowsNoAccessLive(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	dmToken := testHarness.signIn(testHarness.dmA)
	pendingToken := testHarness.signIn(testHarness.pendingA)
	stream := testHarness.openStream(campaignA, pendingToken, "")
	stream.nextMessage(2 * time.Second)

	testHarness.expectStatus(testHarness.do(http.MethodPost, memberActionPath(campaignA, "pendingA", "deny"), requestOptions{token: dmToken}), http.StatusOK)
	if message, received := stream.nextMessage(2 * time.Second); !received || message.data != `{"topics":["me"]}` {
		t.Fatalf("deny sent %+v, want the me topic", message)
	}
	me := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/me"), requestOptions{token: pendingToken})
	testHarness.expectStatus(me, http.StatusOK)
	if status := decodeCampaignMe(t, me.Body.String()).Status; status != core.MemberStatusRemoved {
		t.Fatalf("status after deny = %s, want REMOVED", status)
	}
}

func TestMemberDecisions(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	dmToken := testHarness.signIn(testHarness.dmA)
	dmOptions := requestOptions{token: dmToken}

	list := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/members"), dmOptions)
	testHarness.expectStatus(list, http.StatusOK)
	var groups core.MemberGroups
	if err := json.Unmarshal(list.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	// dmA, ana, otherA and dmB play here; pendingA waits; removedA was denied.
	if len(groups.Active) != 4 || len(groups.Pending) != 1 || len(groups.Removed) != 1 {
		t.Fatalf("groups = %d active, %d pending, %d removed; want 4, 1, 1", len(groups.Active), len(groups.Pending), len(groups.Removed))
	}
	if groups.Active[0].Dm == nil || !groups.Active[0].Dm.IsDM {
		t.Fatalf("the DM should be listed first among the active, flagged: %+v", groups.Active[0])
	}
	for _, member := range groups.Active[1:] {
		if member.Dm == nil || member.Dm.IsDM {
			t.Fatalf("only one DM per campaign; a DM elsewhere is a player here: %+v", member)
		}
	}

	for _, testCase := range []struct {
		name        string
		accountName string
		action      string
		status      int
	}{
		{"approving a removed member asks for re-admission", "removedA", "approve", http.StatusConflict},
		{"denying an active member asks for removal", "ana", "deny", http.StatusConflict},
		{"re-admitting a pending member asks for approval", "pendingA", "readmit", http.StatusConflict},
		{"the DM can't be removed", "dmA", "remove", http.StatusForbidden},
		{"approving twice is harmless", "ana", "approve", http.StatusOK},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			testHarness.expectStatus(testHarness.do(http.MethodPost, memberActionPath(campaignA, testCase.accountName, testCase.action), dmOptions), testCase.status)
		})
	}
	testHarness.expectStatus(testHarness.do(http.MethodPost, campaignPathOf(campaignA, "/members/"+uuid.NewString()+"/approve"), dmOptions), http.StatusNotFound)

	rename := testHarness.do(http.MethodPut, memberActionPath(campaignA, "otherA", "character-name"), requestOptions{token: dmToken, body: `{"characterName":"  Sela Windmere  "}`})
	testHarness.expectStatus(rename, http.StatusOK)
	if !strings.Contains(rename.Body.String(), `"characterName":"Sela Windmere"`) {
		t.Fatalf("character name not trimmed: %s", rename.Body.String())
	}
	tooLong := testHarness.do(http.MethodPut, memberActionPath(campaignA, "otherA", "character-name"), requestOptions{token: dmToken, body: `{"characterName":"` + strings.Repeat("x", 61) + `"}`})
	testHarness.expectStatus(tooLong, http.StatusUnprocessableEntity)
	if !strings.Contains(tooLong.Body.String(), "body.characterName") {
		t.Fatalf("422 lacks the field location: %s", tooLong.Body.String())
	}

	playerToken := testHarness.signIn(testHarness.ana)
	profile := testHarness.do(http.MethodPut, campaignPathOf(campaignA, "/me/profile"), requestOptions{token: playerToken, body: `{"characterName":null}`})
	testHarness.expectStatus(profile, http.StatusOK)
	me := decodeCampaignMe(t, profile.Body.String())
	if me.Member.CharacterName != nil || me.Member.Dm != nil {
		t.Fatalf("profile reply = %+v; want a cleared name and no dm object", me)
	}
}

func TestViewAsPlayer(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	dmToken := testHarness.signIn(testHarness.dmA)
	otherDeviceToken := testHarness.signIn(testHarness.dmA)
	dmOptions := requestOptions{token: dmToken}
	membersPath := campaignPathOf(campaignA, "/members")
	viewAsPlayerPath := campaignPathOf(campaignA, "/me/view-as-player")

	testHarness.expectStatus(testHarness.do(http.MethodGet, membersPath, dmOptions), http.StatusOK)
	// The switch ran under the DM projection, so it answers with no body at all (spec §2.1); the
	// client reads .../me again.
	enabled := testHarness.do(http.MethodPost, viewAsPlayerPath, requestOptions{token: dmToken, body: `{"enabled":true}`})
	testHarness.expectStatus(enabled, http.StatusNoContent)
	if enabled.Body.Len() != 0 {
		t.Fatalf("turning view as player on answered %q; want no body", enabled.Body.String())
	}

	me := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/me"), dmOptions)
	testHarness.expectStatus(me, http.StatusOK)
	decoded := decodeCampaignMe(t, me.Body.String())
	if !decoded.IsDM || !decoded.ViewAsPlayer || decoded.Member.Dm != nil {
		t.Fatalf("me while viewing as player = %+v; want isDm, viewAsPlayer and no dm object", decoded)
	}
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, membersPath, ""},
		{http.MethodGet, campaignPathOf(campaignA, "/settings"), ""},
		{http.MethodGet, campaignPathOf(campaignA, "/invite"), ""},
		{http.MethodPut, campaignPathOf(campaignA, "/settings"), settingsBodyA},
		{http.MethodPost, memberActionPath(campaignA, "pendingA", "approve"), ""},
	} {
		response := testHarness.do(request.method, request.path, requestOptions{token: dmToken, body: request.body})
		if response.Code != http.StatusForbidden {
			t.Errorf("%s %s while viewing as player = %d, want 403", request.method, request.path, response.Code)
		}
		for _, secret := range testHarness.dmOnlyValues(campaignA) {
			if strings.Contains(response.Body.String(), secret) {
				t.Errorf("%s %s leaked %q while viewing as player", request.method, request.path, secret)
			}
		}
	}

	// View as player belongs to one browser session; the DM's other device is unaffected.
	testHarness.expectStatus(testHarness.do(http.MethodGet, membersPath, requestOptions{token: otherDeviceToken}), http.StatusOK)

	testHarness.expectStatus(testHarness.do(http.MethodPost, viewAsPlayerPath, requestOptions{token: dmToken, body: `{"enabled":false}`}), http.StatusNoContent)
	testHarness.expectStatus(testHarness.do(http.MethodGet, membersPath, dmOptions), http.StatusOK)

	// Only a campaign's DM may turn it on there: a player, and A's DM inside B where they play.
	testHarness.expectStatus(testHarness.do(http.MethodPost, viewAsPlayerPath, requestOptions{token: testHarness.signIn(testHarness.ana), body: `{"enabled":true}`}), http.StatusForbidden)
	testHarness.expectStatus(testHarness.do(http.MethodPost, campaignPathOf(campaignB, "/me/view-as-player"), requestOptions{token: dmToken, body: `{"enabled":true}`}), http.StatusForbidden)
}

// Sessions are cached, so ordinary requests run no query; memberships are not, so a change to one is
// seen by the very next request.
func TestSessionCacheAvoidsQueriesAndMembershipsAreReadFresh(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	playerToken := testHarness.signIn(testHarness.ana)
	queries := testHarness.database.Queries

	testHarness.expectStatus(testHarness.do(http.MethodGet, "/healthz", requestOptions{token: playerToken}), http.StatusOK)
	queries.Reset()
	for range 3 {
		testHarness.expectStatus(testHarness.do(http.MethodGet, "/healthz", requestOptions{token: playerToken}), http.StatusOK)
	}
	if count := queries.Count(); count != 0 {
		t.Fatalf("repeat requests ran %d queries; a cached session should run none", count)
	}

	if _, err := testHarness.database.Pool.Exec(testHarness.context, "UPDATE campaign_member SET status = 'PENDING' WHERE id = $1", campaignA.members["ana"]); err != nil {
		t.Fatalf("change status directly: %v", err)
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/colony"), requestOptions{token: playerToken}), http.StatusForbidden)
}

// `mise run reset` under a running server deletes every account and session the cache still holds.
// The first request that notices signs everyone out, instead of leaving a stale session able to act.
func TestSessionsEndWhenTheDatabaseIsReset(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	dmToken := testHarness.signIn(testHarness.dmA)
	otherDeviceToken := testHarness.signIn(testHarness.dmA)
	playerToken := testHarness.signIn(testHarness.ana)
	for _, token := range []string{dmToken, otherDeviceToken, playerToken} {
		testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: token}), http.StatusOK)
	}

	if _, err := testHarness.database.Pool.Exec(testHarness.context, "TRUNCATE account CASCADE"); err != nil {
		t.Fatalf("simulate a reset: %v", err)
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: dmToken}), http.StatusUnauthorized)
	testHarness.expectStatus(testHarness.do(http.MethodPut, campaignPathOf(campaignA, "/settings"), requestOptions{token: otherDeviceToken, body: settingsBodyA}), http.StatusUnauthorized)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/members"), requestOptions{token: dmToken}), http.StatusUnauthorized)
	cleared := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: playerToken})
	testHarness.expectStatus(cleared, http.StatusUnauthorized)
	if !strings.Contains(cleared.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("the stale cookie was not cleared: %q", cleared.Header().Get("Set-Cookie"))
	}
}

func TestSignOut(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.signIn(testHarness.ana)
	otherDeviceToken := testHarness.signIn(testHarness.ana)
	stream := testHarness.openStream(testHarness.campaignA, token, "")
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

	// Any account may sign out: a waiting joiner, and someone in no campaign at all.
	for _, who := range []account{testHarness.pendingA, testHarness.outsider} {
		testHarness.expectStatus(testHarness.do(http.MethodPost, "/auth/logout", requestOptions{token: testHarness.signIn(who)}), http.StatusNoContent)
	}
}

func TestSignOutEverywhere(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.signIn(testHarness.ana)
	otherDeviceToken := testHarness.signIn(testHarness.ana)
	streamA := testHarness.openStream(testHarness.campaignA, otherDeviceToken, "")
	streamB := testHarness.openStream(testHarness.campaignB, otherDeviceToken, "")
	streamA.nextMessage(2 * time.Second)
	streamB.nextMessage(2 * time.Second)

	testHarness.expectStatus(testHarness.do(http.MethodPost, "/auth/logout-everywhere", requestOptions{token: token}), http.StatusNoContent)
	for _, stream := range []*liveStream{streamA, streamB} {
		if !stream.waitForEnd(2 * time.Second) {
			t.Fatal("sign out everywhere left a stream open")
		}
	}
	for _, signedOut := range []string{token, otherDeviceToken} {
		testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: signedOut}), http.StatusUnauthorized)
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: testHarness.signIn(testHarness.otherA)}), http.StatusOK)
	// Signing out everywhere needs no campaign: an account with none may do it.
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/auth/logout-everywhere", requestOptions{token: testHarness.signIn(testHarness.outsider)}), http.StatusNoContent)
}

func TestCrossSiteRequestsAreRefused(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	dmToken := testHarness.signIn(testHarness.dmA)
	enable := `{"enabled":true}`
	viewAsPlayerPath := campaignPathOf(campaignA, "/me/view-as-player")

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
			response := testHarness.do(http.MethodPost, viewAsPlayerPath, requestOptions{token: dmToken, body: testCase.body, headers: testCase.headers})
			testHarness.expectStatus(response, testCase.status)
		})
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/members"), requestOptions{token: dmToken}), http.StatusOK)

	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": testPublicURL}
	testHarness.expectStatus(testHarness.do(http.MethodPost, viewAsPlayerPath, requestOptions{token: dmToken, body: enable, headers: sameOrigin}), http.StatusNoContent)

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
			t.Fatalf("a production build lists dev sign-in accounts: %+v", options.DevSignIn)
		}
		return
	}
	if len(options.DevSignIn) != 9 {
		t.Fatalf("dev sign-in lists %d accounts; want each of the harness's nine once", len(options.DevSignIn))
	}
	owner := slices.IndexFunc(options.DevSignIn, func(entry core.DevSignInAccount) bool { return entry.IsSiteOwner })
	if owner < 0 || options.DevSignIn[owner].AccountID != testHarness.owner.caller.AccountID {
		t.Fatalf("dev sign-in doesn't flag the site owner: %+v", options.DevSignIn)
	}
	hollis := slices.IndexFunc(options.DevSignIn, func(entry core.DevSignInAccount) bool { return entry.AccountID == testHarness.dmB.caller.AccountID })
	if hollis < 0 || len(options.DevSignIn[hollis].Memberships) != 2 {
		t.Fatalf("dev sign-in should show the DM of B with both memberships: %+v", options.DevSignIn)
	}
}

// Core owns validation messages: the schema carries no length or pattern rules that would answer
// first in the schema validator's words (and before core trims).
func TestValidationMessagesComeFromCore(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	dmOptions := requestOptions{token: testHarness.signIn(testHarness.dmA)}

	for _, testCase := range []struct {
		method, path, body, want string
	}{
		{http.MethodPut, campaignPathOf(campaignA, "/settings"), `{"campaignName":"","colonyName":"Port Alpha","timezone":"UTC","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null}`, "Give the campaign a name."},
		{http.MethodPut, campaignPathOf(campaignA, "/settings"), `{"campaignName":"Colony","colonyName":"Port Alpha","timezone":"UTC","discordAnnounceChannelId":"12ab","discordReminderChannelId":null,"discordDmChannelId":null}`, "Pick a text channel of the linked server."},
		{http.MethodPut, campaignPathOf(campaignA, "/me/profile"), `{"characterName":"x\u0000y"}`, "Remove the tabs, line breaks or other control characters."},
		{http.MethodPost, "/api/campaign-requests", `{"campaignName":"Sky Isles","slug":"Sky Isles","note":""}`, "slug"},
	} {
		dmOptions.body = testCase.body
		response := testHarness.do(testCase.method, testCase.path, dmOptions)
		testHarness.expectStatus(response, http.StatusUnprocessableEntity)
		if !strings.Contains(response.Body.String(), testCase.want) {
			t.Errorf("%s %s %s: body %s, want %q", testCase.method, testCase.path, testCase.body, response.Body.String(), testCase.want)
		}
	}

	dmOptions.body = `{"characterName":"  ` + strings.Repeat("a", 60) + `  "}`
	testHarness.expectStatus(testHarness.do(http.MethodPut, campaignPathOf(campaignA, "/me/profile"), dmOptions), http.StatusOK)
}
