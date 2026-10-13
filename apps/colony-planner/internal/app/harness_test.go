package app

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

const (
	// testOwnerDiscordID is the harness's site owner. Tests never read SITE_OWNER_DISCORD_ID from the
	// environment.
	testOwnerDiscordID = "500000000000000005"
	testPublicURL      = "http://localhost:5173"
	// testHost is the Host header a browser sends through the Vite proxy.
	testHost = "localhost:5173"

	// The three tokens of the cross-campaign sweep (docs/design/multi-campaign.md §4.4). canaryA is in
	// campaign A's DM-only values; canaryB in every value of campaign B its players may see; canaryBdm in
	// B's DM-only values. canaryBdm contains canaryB, so "no canaryB" forbids both.
	canaryA   = "CANARY-A-a91f"
	canaryB   = "CANARY-B-77c3"
	canaryBdm = canaryB + "-DM"

	// Numeric stand-ins for DM-only values that accept only digits: each campaign's Discord server and
	// channel IDs.
	guildA   = "660066006600660066"
	channelA = "990099009900990099"
	guildB   = "550055005500550055"
	channelB = "880088008800880088"
)

type recordingNotifier struct {
	mutex    sync.Mutex
	waiting  []core.WaitingMember
	requests []core.RequestedCampaign
}

func (notifier *recordingNotifier) MemberWaiting(_ context.Context, member core.WaitingMember) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.waiting = append(notifier.waiting, member)
}

func (notifier *recordingNotifier) CampaignRequested(_ context.Context, request core.RequestedCampaign) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.requests = append(notifier.requests, request)
}

func (notifier *recordingNotifier) waitingCount() int {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	return len(notifier.waiting)
}

// fakeDirectory is Beny Bot's gateway state: the text channels of each linked server.
type fakeDirectory struct {
	mutex    sync.Mutex
	channels map[string][]core.DiscordChannel
}

func (directory *fakeDirectory) TextChannels(_ context.Context, guildID string) ([]core.DiscordChannel, error) {
	directory.mutex.Lock()
	defer directory.mutex.Unlock()
	return directory.channels[guildID], nil
}

// account is one signed-in identity of the harness.
type account struct {
	name   string
	caller core.Caller
}

// campaignFixture is one campaign of the harness and the rows its routes name.
type campaignFixture struct {
	id         uuid.UUID
	name       string
	slug       string
	requestID  uuid.UUID
	inviteCode string
	// linkCodes are every /link code issued for the campaign: DM-only literals.
	linkCodes []string
	// members maps an account name to its membership ID here.
	members map[string]uuid.UUID
	// pathIDs names a real row of this campaign for each path parameter of a campaign route.
	pathIDs map[string]string
	// arrivalEntryID is a SHIPMENT_ARRIVED entry, for cargo bodies.
	arrivalEntryID uuid.UUID
	// hiddenIDs are the IDs of rows players must never see (hidden NPCs and places): DM-only literals.
	hiddenIDs []string
	// keptPlaceID is a hidden place no sweep deletes, for valid NPC bodies in pass 4.
	keptPlaceID uuid.UUID
	// hiddenNpcID is a hidden named colonist.
	hiddenNpcID uuid.UUID
	// hiddenImageIDs are images only hidden things use, or nothing: players read each like a missing one.
	hiddenImageIDs []uuid.UUID
	// requester is a member no sweep caller signs in as: their declined request's note is DM-only to
	// everyone else, so only they and the DM may read it.
	requester account
	// declinedRequestID is the requester's request whose decision note is a canary.
	declinedRequestID uuid.UUID
	// hiddenQuestID, hiddenPatronID and hiddenRequestID are hidden rows players must read exactly like
	// missing ones. The request was made while its patron and action were hidden, both revealed since.
	hiddenQuestID   uuid.UUID
	hiddenPatronID  uuid.UUID
	hiddenRequestID uuid.UUID
	// archivedIDs are an archived patron and action: players may still read their history, so they
	// aren't canaries, but no player list or picker may hold them.
	archivedIDs []string
	// failureCost is the revealed quest's DM-only failure cost: its gold delta and failure favor.
	failureCost []int64
}

// harness is the whole server on a fresh test schema with the accounts and campaigns of the
// cross-campaign sweep (docs/design/multi-campaign.md §4.4):
//
//	account               campaign A        campaign B
//	dmA                   DM                ACTIVE player
//	ana                   ACTIVE player     ACTIVE player
//	otherA, pendingA,     ACTIVE, PENDING,  -
//	removedA              REMOVED
//	dmB                   ACTIVE player     DM
//	bOnly                 -                 ACTIVE player
//	outsider, owner       -                 -
//
// Both campaigns are made through the real path: RequestCampaign, then the owner's approval.
// logBuffer collects the server's log output, so a test can check what the operator would read.
type logBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (logs *logBuffer) Write(data []byte) (int, error) {
	logs.mutex.Lock()
	defer logs.mutex.Unlock()
	return logs.buffer.Write(data)
}

func (logs *logBuffer) String() string {
	logs.mutex.Lock()
	defer logs.mutex.Unlock()
	return logs.buffer.String()
}

type harness struct {
	t         *testing.T
	logs      *logBuffer
	context   context.Context
	database  *dbtest.Database
	core      *core.Service
	auth      *auth.Service
	cache     *auth.SessionCache
	server    *Server
	handler   http.Handler
	notifier  *recordingNotifier
	directory *fakeDirectory

	owner, dmA, ana, otherA, pendingA, removedA, dmB, bOnly, outsider account

	campaignA, campaignB *campaignFixture
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	database := dbtest.New(t)
	logs := &logBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	cache := auth.NewSessionCache()
	notifier := &recordingNotifier{}
	directory := &fakeDirectory{channels: map[string][]core.DiscordChannel{}}
	service := core.New(core.Options{
		Pool:               database.Pool,
		Notifier:           notifier,
		Logger:             logger,
		SiteOwnerDiscordID: testOwnerDiscordID,
		Sessions:           cache,
		Directory:          directory,
		PublicURL:          testPublicURL,
	})
	authService, err := auth.New(auth.Options{
		Core:               service,
		Logger:             logger,
		SessionCache:       cache,
		PublicURL:          testPublicURL,
		AppEnvironment:     "test",
		SiteOwnerDiscordID: testOwnerDiscordID,
	})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	server, err := New(Options{
		Core:                   service,
		Auth:                   authService,
		Logger:                 logger,
		StreamHeartbeat:        50 * time.Millisecond,
		StreamLifetime:         10 * time.Second,
		ImageUploadReadTimeout: time.Second,
		RequestReadTimeout:     testRequestReadTimeout,
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	testHarness := &harness{
		t:         t,
		logs:      logs,
		context:   context.Background(),
		database:  database,
		core:      service,
		auth:      authService,
		cache:     cache,
		server:    server,
		handler:   server.Handler(),
		notifier:  notifier,
		directory: directory,
	}
	// Usernames and Discord IDs are DM-only (MemberDm). Accounts in only one campaign carry that
	// campaign's DM token; accounts the owner sees as a DM, or that sit in both campaigns, carry none.
	testHarness.owner = testHarness.signUp("owner", testOwnerDiscordID, "site.owner", "Site Owner")
	testHarness.dmA = testHarness.signUp("dmA", "100000000000000001", "dee.dm", "Dee the DM")
	testHarness.ana = testHarness.signUp("ana", "100000000000000002", "ana", "Ana")
	testHarness.otherA = testHarness.signUp("otherA", "100000000000000003", "bram."+canaryA, "Bram")
	testHarness.pendingA = testHarness.signUp("pendingA", "100000000000000004", "emil."+canaryA, "Emil")
	testHarness.removedA = testHarness.signUp("removedA", "100000000000000006", "gus."+canaryA, "Gus")
	testHarness.dmB = testHarness.signUp("dmB", "200000000000000001", "hollis", "Hollis")
	testHarness.bOnly = testHarness.signUp("bOnly", "200000000000000002", "iko."+canaryBdm, "Iko")
	testHarness.outsider = testHarness.signUp("outsider", "300000000000000001", "quinn", "Quinn")

	testHarness.campaignA = testHarness.createCampaign(testHarness.dmA, "Campaign Alpha", "campaign-alpha")
	testHarness.campaignB = testHarness.createCampaign(testHarness.dmB, "Second Sample", "second-sample")
	testHarness.join(testHarness.campaignA, testHarness.dmA, testHarness.ana, "approve")
	testHarness.join(testHarness.campaignA, testHarness.dmA, testHarness.otherA, "approve")
	testHarness.join(testHarness.campaignA, testHarness.dmA, testHarness.pendingA, "")
	testHarness.join(testHarness.campaignA, testHarness.dmA, testHarness.removedA, "deny")
	testHarness.join(testHarness.campaignA, testHarness.dmA, testHarness.dmB, "approve")
	testHarness.join(testHarness.campaignB, testHarness.dmB, testHarness.dmA, "approve")
	testHarness.join(testHarness.campaignB, testHarness.dmB, testHarness.ana, "approve")
	testHarness.join(testHarness.campaignB, testHarness.dmB, testHarness.bOnly, "approve")
	return testHarness
}

// signUp signs a Discord identity in through core, as the OAuth2 callback would.
func (testHarness *harness) signUp(name, discordUserID, username, displayName string) account {
	testHarness.t.Helper()
	accountID, err := testHarness.core.SignIn(testHarness.context, core.DiscordIdentity{
		UserID:      discordUserID,
		Username:    username,
		DisplayName: displayName,
	})
	if err != nil {
		testHarness.t.Fatalf("sign up %s: %v", name, err)
	}
	return account{name: name, caller: core.Caller{
		AccountID:     accountID,
		DiscordUserID: discordUserID,
		IsSiteOwner:   testHarness.core.IsSiteOwner(discordUserID),
	}}
}

func (testHarness *harness) createCampaign(dm account, name, slug string) *campaignFixture {
	testHarness.t.Helper()
	request, err := testHarness.core.RequestCampaign(testHarness.context, dm.caller, core.CampaignRequestInput{
		CampaignName: name,
		Slug:         slug,
		Note:         "Please",
	})
	if err != nil {
		testHarness.t.Fatalf("request %s: %v", name, err)
	}
	approved, err := testHarness.core.ApproveCampaignRequest(testHarness.context, testHarness.owner.caller, request.ID, core.ApproveCampaignRequestInput{})
	if err != nil {
		testHarness.t.Fatalf("approve %s: %v", name, err)
	}
	campaign := &campaignFixture{
		id:        approved.ID,
		name:      name,
		slug:      slug,
		requestID: request.ID,
		members:   map[string]uuid.UUID{},
		pathIDs:   map[string]string{},
	}
	dmViewer := testHarness.enter(dm, campaign.id)
	campaign.members[dm.name] = dmViewer.MemberID()
	invite, err := testHarness.core.Invite(testHarness.context, dmViewer)
	if err != nil {
		testHarness.t.Fatalf("read %s's invite: %v", name, err)
	}
	campaign.inviteCode = invite.InviteCode
	return campaign
}

// join asks to join through the invite link; decision is "approve", "deny" or "" (stay waiting).
func (testHarness *harness) join(campaign *campaignFixture, dm account, joiner account, decision string) {
	testHarness.t.Helper()
	joined, err := testHarness.core.JoinByInvite(testHarness.context, joiner.caller, campaign.inviteCode)
	if err != nil {
		testHarness.t.Fatalf("%s joins %s: %v", joiner.name, campaign.name, err)
	}
	campaign.members[joiner.name] = joined.MemberID
	dmViewer := testHarness.enter(dm, campaign.id)
	switch decision {
	case "approve":
		_, err = testHarness.core.ApproveMember(testHarness.context, dmViewer, joined.MemberID)
	case "deny":
		_, err = testHarness.core.DenyMember(testHarness.context, dmViewer, joined.MemberID)
	}
	if err != nil {
		testHarness.t.Fatalf("%s %s in %s: %v", decision, joiner.name, campaign.name, err)
	}
}

// enter is core.EnterCampaign for an account, as the access gate calls it.
func (testHarness *harness) enter(who account, campaignID uuid.UUID) core.Viewer {
	testHarness.t.Helper()
	viewer, err := testHarness.core.EnterCampaign(testHarness.context, who.caller, campaignID)
	if err != nil {
		testHarness.t.Fatalf("%s enters %s: %v", who.name, campaignID, err)
	}
	return viewer
}

// signIn starts a web session for the account and returns its cookie value.
func (testHarness *harness) signIn(who account) string {
	testHarness.t.Helper()
	session, err := testHarness.core.CreateSession(testHarness.context, who.caller.AccountID)
	if err != nil {
		testHarness.t.Fatalf("create session for %s: %v", who.name, err)
	}
	return session.Token
}

// campaignPathOf is the API path of a campaign route: campaignPathOf(campaign, "/colony").
func campaignPathOf(campaign *campaignFixture, suffix string) string {
	return "/api/campaigns/" + campaign.id.String() + suffix
}

type requestOptions struct {
	token   string
	body    string
	headers map[string]string
	context context.Context
	// host replaces testHost.
	host string
}

func (testHarness *harness) do(method, path string, options requestOptions) *httptest.ResponseRecorder {
	testHarness.t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(options.body))
	request.Host = testHost
	if options.host != "" {
		request.Host = options.host
	}
	if options.context != nil {
		request = request.WithContext(options.context)
	}
	if options.body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if options.token != "" {
		request.AddCookie(&http.Cookie{Name: testHarness.auth.SessionCookieName(), Value: options.token})
	}
	for name, value := range options.headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	testHarness.handler.ServeHTTP(recorder, request)
	return recorder
}

func (testHarness *harness) expectStatus(response *httptest.ResponseRecorder, status int) {
	testHarness.t.Helper()
	if response.Code != status {
		testHarness.t.Fatalf("status = %d, want %d; body: %s", response.Code, status, response.Body.String())
	}
}

// streamEvent is one SSE frame: a message (id, event, data) or a comment.
type streamEvent struct {
	id      string
	event   string
	data    string
	comment string
	raw     string
}

// liveStream is GET /api/campaigns/{campaignId}/stream over a real HTTP connection.
type liveStream struct {
	events chan streamEvent
	cancel context.CancelFunc
	status int
}

func (testHarness *harness) openStream(campaign *campaignFixture, token string, lastEventID string) *liveStream {
	testHarness.t.Helper()
	server := httptest.NewServer(testHarness.handler)
	testHarness.t.Cleanup(server.Close)
	return testHarness.openStreamOn(server, campaign, token, lastEventID)
}

// openStreamOn opens the stream on a server the test started, such as one with the app's read limits.
func (testHarness *harness) openStreamOn(server *httptest.Server, campaign *campaignFixture, token string, lastEventID string) *liveStream {
	testHarness.t.Helper()
	streamContext, cancel := context.WithCancel(context.Background())
	testHarness.t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(streamContext, http.MethodGet, server.URL+campaignPathOf(campaign, "/stream"), nil)
	if err != nil {
		testHarness.t.Fatalf("build stream request: %v", err)
	}
	request.AddCookie(&http.Cookie{Name: testHarness.auth.SessionCookieName(), Value: token})
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		testHarness.t.Fatalf("open stream: %v", err)
	}
	stream := &liveStream{events: make(chan streamEvent, 64), cancel: cancel, status: response.StatusCode}
	go func() {
		defer close(stream.events)
		defer func() { _ = response.Body.Close() }()
		reader := bufio.NewReader(response.Body)
		var current streamEvent
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			current.raw += line
			line = strings.TrimRight(line, "\n")
			switch {
			case line == "":
				if current.raw != "\n" {
					stream.events <- current
				}
				current = streamEvent{}
			case strings.HasPrefix(line, ":"):
				current.comment = strings.TrimSpace(strings.TrimPrefix(line, ":"))
			case strings.HasPrefix(line, "id: "):
				current.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				current.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return stream
}

// nextMessage skips heartbeats and returns the next `event:` message, or false if the stream ended or
// nothing arrived in time.
func (stream *liveStream) nextMessage(timeout time.Duration) (streamEvent, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case event, open := <-stream.events:
			if !open {
				return streamEvent{}, false
			}
			if event.event != "" {
				return event, true
			}
		case <-deadline:
			return streamEvent{}, false
		}
	}
}

// waitForEnd reports whether the server closed the stream within timeout.
func (stream *liveStream) waitForEnd(timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case _, open := <-stream.events:
			if !open {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
