package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

// testOwnerDiscordID is the harness's site owner; tests never read SITE_OWNER_DISCORD_ID or
// DM_DISCORD_ID from the environment.
const testOwnerDiscordID = "200000000000000002"

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

func (notifier *recordingNotifier) count() int {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	return len(notifier.waiting)
}

type authHarness struct {
	t        *testing.T
	database *dbtest.Database
	core     *core.Service
	auth     *Service
	notifier *recordingNotifier
}

// newAuthHarness builds auth on a fresh schema. options.SiteOwnerDiscordID defaults to
// testOwnerDiscordID; the harness passes the same value to core, as cmd/colony does.
func newAuthHarness(t *testing.T, options Options) *authHarness {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	cache := NewSessionCache()
	notifier := &recordingNotifier{}
	if options.SiteOwnerDiscordID == "" {
		options.SiteOwnerDiscordID = testOwnerDiscordID
	}
	service := core.New(core.Options{
		Pool:               database.Pool,
		Notifier:           notifier,
		Logger:             logger,
		SiteOwnerDiscordID: options.SiteOwnerDiscordID,
		Sessions:           cache,
	})
	options.Core = service
	options.Logger = logger
	options.SessionCache = cache
	if options.PublicURL == "" {
		options.PublicURL = "http://localhost:5173"
	}
	if options.AppEnvironment == "" {
		options.AppEnvironment = "test"
	}
	authService, err := New(options)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return &authHarness{t: t, database: database, core: service, auth: authService, notifier: notifier}
}

// createAccount signs a Discord identity in through core and returns its Caller.
func (harness *authHarness) createAccount(discordUserID, displayName string) core.Caller {
	harness.t.Helper()
	accountID, err := harness.core.SignIn(context.Background(), core.DiscordIdentity{UserID: discordUserID, Username: displayName, DisplayName: displayName})
	if err != nil {
		harness.t.Fatalf("create account %s: %v", displayName, err)
	}
	return core.Caller{AccountID: accountID, DiscordUserID: discordUserID, IsSiteOwner: harness.core.IsSiteOwner(discordUserID)}
}

// createCampaign makes a campaign DM'd by dm through the real request and the owner's approval, and
// returns its ID and invite code.
func (harness *authHarness) createCampaign(dm core.Caller, name, slug string) (uuid.UUID, string) {
	harness.t.Helper()
	testContext := context.Background()
	owner := harness.createAccount(harness.auth.options.SiteOwnerDiscordID, "Owner")
	request, err := harness.core.RequestCampaign(testContext, dm, core.CampaignRequestInput{CampaignName: name, Slug: slug})
	if err != nil {
		harness.t.Fatalf("request %s: %v", name, err)
	}
	// The owner's own request is approved on submit; anyone else's waits for the owner.
	var campaignID uuid.UUID
	if request.Campaign != nil {
		campaignID = request.Campaign.ID
	} else {
		approved, err := harness.core.ApproveCampaignRequest(testContext, owner, request.ID, core.ApproveCampaignRequestInput{})
		if err != nil {
			harness.t.Fatalf("approve %s: %v", name, err)
		}
		campaignID = approved.ID
	}
	viewer, err := harness.core.EnterCampaign(testContext, dm, campaignID)
	if err != nil {
		harness.t.Fatalf("enter %s: %v", name, err)
	}
	invite, err := harness.core.Invite(testContext, viewer)
	if err != nil {
		harness.t.Fatalf("invite to %s: %v", name, err)
	}
	return campaignID, invite.InviteCode
}

// serve runs handler behind the session middleware, as the real server does.
func (harness *authHarness) serve(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	harness.auth.Middleware(handler).ServeHTTP(recorder, request)
	return recorder
}

// callerFor resolves a session cookie the way every request does.
func (harness *authHarness) callerFor(cookie *http.Cookie) (core.Caller, bool) {
	harness.t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	var caller core.Caller
	var signedIn bool
	harness.serve(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		caller, signedIn = CallerFromContext(request.Context())
	}), request)
	return caller, signedIn
}

func responseCookie(response *httptest.ResponseRecorder, name string) *http.Cookie {
	var found *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			found = cookie
		}
	}
	return found
}

func (harness *authHarness) countAccounts() int64 {
	harness.t.Helper()
	count, err := store.New(harness.database.Pool).CountAccounts(context.Background())
	if err != nil {
		harness.t.Fatalf("count accounts: %v", err)
	}
	return count
}

func (harness *authHarness) memberships(caller core.Caller) []core.MyCampaign {
	harness.t.Helper()
	mine, err := harness.core.MyCampaigns(context.Background(), caller)
	if err != nil {
		harness.t.Fatalf("my campaigns: %v", err)
	}
	return mine.Campaigns
}
