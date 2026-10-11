package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// A new DM's whole path over HTTP: ask, the owner approves (changing nothing else), the campaign opens,
// a friend joins by invite link and is let in, and a reset kills the old link.
func TestAskingForACampaignAndJoiningIt(t *testing.T) {
	testHarness := newHarness(t)
	miraToken := testHarness.signIn(testHarness.signUp("mira", "400000000000000002", "mira", "Mira"))
	ownerToken := testHarness.signIn(testHarness.owner)

	asked := testHarness.do(http.MethodPost, "/api/campaign-requests", requestOptions{token: miraToken, body: `{"campaignName":"Sky Isles","slug":"sky-isles","note":"Thursdays"}`})
	testHarness.expectStatus(asked, http.StatusCreated)
	var request core.CampaignRequestView
	if err := json.Unmarshal(asked.Body.Bytes(), &request); err != nil || request.Status != core.CampaignRequestStatusPending {
		t.Fatalf("request = %+v, %v", request, err)
	}
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/campaign-requests", requestOptions{token: miraToken, body: `{"campaignName":"Again","slug":"again","note":""}`}), http.StatusConflict)

	// Only the owner sees and decides requests.
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/admin/campaign-requests", requestOptions{token: miraToken}), http.StatusForbidden)
	list := testHarness.do(http.MethodGet, "/api/admin/campaign-requests", requestOptions{token: ownerToken})
	testHarness.expectStatus(list, http.StatusOK)
	if !strings.Contains(list.Body.String(), "Thursdays") {
		t.Fatalf("the owner's list lacks the request: %s", list.Body.String())
	}
	approved := testHarness.do(http.MethodPost, "/api/admin/campaign-requests/"+request.ID.String()+"/approve", requestOptions{token: ownerToken, body: `{"slug":null}`})
	testHarness.expectStatus(approved, http.StatusOK)
	var campaign core.AdminCampaign
	if err := json.Unmarshal(approved.Body.Bytes(), &campaign); err != nil || campaign.Slug != "sky-isles" {
		t.Fatalf("approved = %+v, %v", campaign, err)
	}
	skyIsles := &campaignFixture{id: campaign.ID, name: "Sky Isles", slug: "sky-isles", members: map[string]uuid.UUID{}}

	// The owner manages it but can't read it.
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(skyIsles, "/colony"), requestOptions{token: ownerToken}), http.StatusNotFound)
	board := testHarness.do(http.MethodGet, campaignPathOf(skyIsles, "/colony"), requestOptions{token: miraToken})
	testHarness.expectStatus(board, http.StatusOK)

	invite := testHarness.do(http.MethodGet, campaignPathOf(skyIsles, "/invite"), requestOptions{token: miraToken})
	testHarness.expectStatus(invite, http.StatusOK)
	var link core.InviteLink
	if err := json.Unmarshal(invite.Body.Bytes(), &link); err != nil || link.InviteCode == "" {
		t.Fatalf("invite = %+v, %v", link, err)
	}
	nicoToken := testHarness.signIn(testHarness.signUp("nico", "400000000000000003", "nico", "Nico"))
	info := testHarness.do(http.MethodGet, "/api/invites/"+link.InviteCode, requestOptions{token: nicoToken})
	testHarness.expectStatus(info, http.StatusOK)
	if !strings.Contains(info.Body.String(), `"dmDisplayName":"Mira"`) || strings.Contains(info.Body.String(), link.InviteCode) {
		t.Fatalf("invite info = %s; want the DM's name and no invite code", info.Body.String())
	}
	join := testHarness.do(http.MethodPost, "/api/invites/"+link.InviteCode+"/join", requestOptions{token: nicoToken})
	testHarness.expectStatus(join, http.StatusOK)
	var joined core.MyCampaign
	if err := json.Unmarshal(join.Body.Bytes(), &joined); err != nil || joined.Status != core.MemberStatusPending {
		t.Fatalf("join = %+v, %v", joined, err)
	}
	if testHarness.notifier.waitingCount() == 0 {
		t.Fatal("a new joiner pinged nobody")
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(skyIsles, "/colony"), requestOptions{token: nicoToken}), http.StatusForbidden)
	testHarness.expectStatus(testHarness.do(http.MethodPost, campaignPathOf(skyIsles, "/members/"+joined.MemberID.String()+"/approve"), requestOptions{token: miraToken}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(skyIsles, "/colony"), requestOptions{token: nicoToken}), http.StatusOK)

	reset := testHarness.do(http.MethodPost, campaignPathOf(skyIsles, "/invite/reset"), requestOptions{token: miraToken})
	testHarness.expectStatus(reset, http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/invites/"+link.InviteCode, requestOptions{token: nicoToken}), http.StatusNotFound)

	mine := testHarness.do(http.MethodGet, "/api/campaigns", requestOptions{token: nicoToken})
	testHarness.expectStatus(mine, http.StatusOK)
	if !strings.Contains(mine.Body.String(), `"slug":"sky-isles"`) {
		t.Fatalf("Nico's campaigns lack Sky Isles: %s", mine.Body.String())
	}
}

// Decision C3: a DM may delete their own campaign at any time after typing its slug; a player and a
// non-member can't, and the other campaign is untouched.
func TestDeletingACampaign(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	deletePath := campaignPathOf(campaignA, "/delete")
	confirm := `{"confirmSlug":"campaign-alpha"}`
	bOnlyToken := testHarness.signIn(testHarness.bOnly)
	before := testHarness.fingerprint(campaignB, testHarness.bOnly)

	testHarness.expectStatus(testHarness.do(http.MethodPost, deletePath, requestOptions{token: testHarness.signIn(testHarness.ana), body: confirm}), http.StatusForbidden)
	testHarness.expectStatus(testHarness.do(http.MethodPost, deletePath, requestOptions{token: testHarness.signIn(testHarness.dmB), body: confirm}), http.StatusForbidden)
	for _, token := range []string{bOnlyToken, testHarness.signIn(testHarness.outsider), testHarness.signIn(testHarness.owner)} {
		testHarness.expectStatus(testHarness.do(http.MethodPost, deletePath, requestOptions{token: token, body: confirm}), http.StatusNotFound)
	}
	// B's DM can't delete A through B's own route either: the route deletes the path's campaign only.
	testHarness.expectStatus(testHarness.do(http.MethodPost, campaignPathOf(campaignB, "/delete"), requestOptions{token: testHarness.signIn(testHarness.dmB), body: confirm}), http.StatusUnprocessableEntity)
	// The owner's delete needs a suspended campaign.
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/admin/campaigns/"+campaignA.id.String()+"/delete", requestOptions{token: testHarness.signIn(testHarness.owner), body: confirm}), http.StatusConflict)

	dmToken := testHarness.signIn(testHarness.dmA)
	anaToken := testHarness.signIn(testHarness.ana)
	stream := testHarness.openStream(campaignA, anaToken, "")
	stream.nextMessage(2 * time.Second)
	testHarness.expectStatus(testHarness.do(http.MethodPost, deletePath, requestOptions{token: dmToken, body: `{"confirmSlug":"campaign-alph"}`}), http.StatusUnprocessableEntity)
	testHarness.expectStatus(testHarness.do(http.MethodPost, deletePath, requestOptions{token: dmToken, body: confirm}), http.StatusNoContent)

	if !stream.waitForEnd(2 * time.Second) {
		t.Fatal("deleting the campaign left a member's stream open")
	}
	for _, token := range []string{dmToken, anaToken} {
		testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/me"), requestOptions{token: token}), http.StatusNotFound)
		testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/me", requestOptions{token: token}), http.StatusOK)
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/invites/"+campaignA.inviteCode, requestOptions{token: anaToken}), http.StatusNotFound)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignB, "/colony"), requestOptions{token: anaToken}), http.StatusOK)
	after := testHarness.fingerprint(campaignB, testHarness.bOnly)
	for name, hash := range after {
		if before[name] != hash {
			t.Errorf("deleting A changed B's %s", name)
		}
	}
}

// Decision C5: a player can leave, only themselves, from waiting or active; never from REMOVED and
// never as the DM. Leaving is a removal: the invite link can't undo it and pings nobody; the DM can.
func TestLeavingACampaign(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	leavePath := campaignPathOf(campaignA, "/leave")
	leave := func(who account) int {
		return testHarness.do(http.MethodPost, leavePath, requestOptions{token: testHarness.signIn(who)}).Code
	}

	if status := leave(testHarness.dmA); status != http.StatusForbidden {
		t.Errorf("the DM leaving = %d, want 403", status)
	}
	if status := leave(testHarness.removedA); status != http.StatusConflict {
		t.Errorf("leaving from REMOVED = %d, want 409", status)
	}
	if status := leave(testHarness.bOnly); status != http.StatusNotFound {
		t.Errorf("a non-member leaving = %d, want 404", status)
	}

	anaToken := testHarness.signIn(testHarness.ana)
	left := testHarness.do(http.MethodPost, leavePath, requestOptions{token: anaToken})
	testHarness.expectStatus(left, http.StatusOK)
	if me := decodeCampaignMe(t, left.Body.String()); me.Status != core.MemberStatusRemoved {
		t.Fatalf("after leaving = %+v; want REMOVED", me)
	}
	if status := leave(testHarness.pendingA); status != http.StatusOK {
		t.Errorf("leaving from PENDING = %d, want 200", status)
	}
	// Only the leaver changed: B and other members are untouched.
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(testHarness.campaignB, "/colony"), requestOptions{token: anaToken}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/colony"), requestOptions{token: testHarness.signIn(testHarness.otherA)}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/colony"), requestOptions{token: anaToken}), http.StatusForbidden)

	dmToken := testHarness.signIn(testHarness.dmA)
	members := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/members"), requestOptions{token: dmToken})
	var groups core.MemberGroups
	if err := json.Unmarshal(members.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	leftOnOwn := map[string]bool{}
	for _, member := range groups.Removed {
		leftOnOwn[member.DisplayName] = member.Dm != nil && member.Dm.LeftOnOwn
	}
	if !leftOnOwn["Ana"] || !leftOnOwn["Emil"] || leftOnOwn["Gus"] {
		t.Fatalf("left on own = %v; want Ana and Emil (left), not Gus (denied)", leftOnOwn)
	}

	pings := testHarness.notifier.waitingCount()
	join := testHarness.do(http.MethodPost, "/api/invites/"+campaignA.inviteCode+"/join", requestOptions{token: anaToken})
	testHarness.expectStatus(join, http.StatusOK)
	if !strings.Contains(join.Body.String(), `"status":"REMOVED"`) || testHarness.notifier.waitingCount() != pings {
		t.Fatalf("the invite link brought a leaver back or pinged: %s", join.Body.String())
	}
	if status := leave(testHarness.ana); status != http.StatusConflict {
		t.Errorf("leaving twice = %d, want 409", status)
	}
	testHarness.expectStatus(testHarness.do(http.MethodPost, memberActionPath(campaignA, "ana", "readmit"), requestOptions{token: dmToken}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/colony"), requestOptions{token: anaToken}), http.StatusOK)
}

// Joining by invite: a REMOVED member stays REMOVED and pings nobody; a suspended campaign answers 409
// and makes no membership; an unknown code reads like a dead link.
func TestJoiningByInvite(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	pings := testHarness.notifier.waitingCount()

	removedJoin := testHarness.do(http.MethodPost, "/api/invites/"+campaignA.inviteCode+"/join", requestOptions{token: testHarness.signIn(testHarness.removedA)})
	testHarness.expectStatus(removedJoin, http.StatusOK)
	if !strings.Contains(removedJoin.Body.String(), `"status":"REMOVED"`) || testHarness.notifier.waitingCount() != pings {
		t.Fatalf("a removed member's join = %s, pings %d → %d", removedJoin.Body.String(), pings, testHarness.notifier.waitingCount())
	}
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/invites/no-such-invite-code-0000000", requestOptions{token: testHarness.signIn(testHarness.outsider)}), http.StatusNotFound)
	testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/invites/"+campaignA.inviteCode, requestOptions{}), http.StatusUnauthorized)

	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/admin/campaigns/"+campaignA.id.String()+"/suspend", requestOptions{token: testHarness.signIn(testHarness.owner)}), http.StatusOK)
	outsiderToken := testHarness.signIn(testHarness.outsider)
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/invites/"+campaignA.inviteCode+"/join", requestOptions{token: outsiderToken}), http.StatusConflict)
	mine := testHarness.do(http.MethodGet, "/api/campaigns", requestOptions{token: outsiderToken})
	if !strings.Contains(mine.Body.String(), `"campaigns":[]`) {
		t.Fatalf("joining a suspended campaign made a membership: %s", mine.Body.String())
	}
	// Members see the paused screen from .../me, and nothing else.
	anaToken := testHarness.signIn(testHarness.ana)
	me := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/me"), requestOptions{token: anaToken})
	testHarness.expectStatus(me, http.StatusOK)
	if decodeCampaignMe(t, me.Body.String()).Campaign.Status != core.CampaignStatusSuspended {
		t.Fatalf("me in a paused campaign = %s", me.Body.String())
	}
	paused := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/colony"), requestOptions{token: anaToken})
	testHarness.expectStatus(paused, http.StatusForbidden)
	if !strings.Contains(paused.Body.String(), core.ErrCampaignSuspended.Message) {
		t.Fatalf("a paused campaign's read says %s", paused.Body.String())
	}
}

// Campaigns don't hear each other: B's traffic reaches no stream of A, even the streams of accounts in
// both; B's approval reaches only the joiner's B stream; A's stream IDs count only A's messages.
func TestCampaignStreamsAreSealed(t *testing.T) {
	testHarness := newHarness(t)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	anaToken := testHarness.signIn(testHarness.ana)
	dmBToken := testHarness.signIn(testHarness.dmB)
	anaInA := testHarness.openStream(campaignA, anaToken, "")
	dmBInA := testHarness.openStream(campaignA, dmBToken, "")
	anaInB := testHarness.openStream(campaignB, anaToken, "")
	var lastA string
	for _, stream := range []*liveStream{anaInA, dmBInA, anaInB} {
		catchUp, received := stream.nextMessage(2 * time.Second)
		if !received {
			t.Fatal("no catch-up message on connect")
		}
		if stream == anaInA {
			lastA = catchUp.id
		}
	}

	dmViewerB := testHarness.enter(testHarness.dmB, campaignB.id)
	board, err := testHarness.core.GetColonyBoard(testHarness.context, dmViewerB)
	if err != nil {
		t.Fatalf("read B's board: %v", err)
	}
	if _, err := testHarness.core.Advance(testHarness.context, dmViewerB, core.AdvanceInput{Days: 3, ExpectedVersion: board.Version}); err != nil {
		t.Fatalf("advance B: %v", err)
	}
	if _, received := anaInB.nextMessage(2 * time.Second); !received {
		t.Fatal("Ana's B tab heard nothing of B's advance")
	}

	// A newcomer asks to join B; the approval reaches their B stream only.
	newcomer := testHarness.signUp("newcomer", "400000000000000009", "newcomer", "Newcomer")
	newcomerToken := testHarness.signIn(newcomer)
	joined, err := testHarness.core.JoinByInvite(testHarness.context, newcomer.caller, campaignB.inviteCode)
	if err != nil {
		t.Fatalf("join B: %v", err)
	}
	waiting := testHarness.openStream(campaignB, newcomerToken, "")
	waiting.nextMessage(2 * time.Second)
	if _, err := testHarness.core.ApproveMember(testHarness.context, dmViewerB, joined.MemberID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if message, received := waiting.nextMessage(2 * time.Second); !received || message.data != `{"topics":["me"]}` {
		t.Fatalf("the joiner's B stream got %+v; want the me topic", message)
	}
	for name, stream := range map[string]*liveStream{"Ana's A tab": anaInA, "B's DM's A tab": dmBInA} {
		if message, received := stream.nextMessage(300 * time.Millisecond); received {
			t.Errorf("%s heard B's traffic: %+v", name, message)
		}
	}

	// A's resume point is still current: B's messages took no number from A's sequence.
	anaInA.cancel()
	resumed := testHarness.openStream(campaignA, anaToken, lastA)
	if message, received := resumed.nextMessage(300 * time.Millisecond); received {
		t.Fatalf("an up-to-date A stream got %+v after B's traffic; want nothing", message)
	}
}
