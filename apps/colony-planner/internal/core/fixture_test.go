package core_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
)

// The site owner of every core test. Tests never read SITE_OWNER_DISCORD_ID.
const testOwnerDiscordID = "500000000000000005"

// testPublicURL is the site's own address, as PUBLIC_URL would give it.
const testPublicURL = "https://colony.example"

type countingNotifier struct {
	mutex    sync.Mutex
	waiting  []core.WaitingMember
	requests []core.RequestedCampaign
}

func (notifier *countingNotifier) MemberWaiting(_ context.Context, member core.WaitingMember) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.waiting = append(notifier.waiting, member)
}

func (notifier *countingNotifier) CampaignRequested(_ context.Context, request core.RequestedCampaign) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.requests = append(notifier.requests, request)
}

func (notifier *countingNotifier) pings() int {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	return len(notifier.waiting)
}

type recordingEvictor struct {
	mutex    sync.Mutex
	accounts []uuid.UUID
}

func (evictor *recordingEvictor) EvictSession([]byte) {}

func (evictor *recordingEvictor) EvictAll() {}

func (evictor *recordingEvictor) EvictAccount(accountID uuid.UUID) {
	evictor.mutex.Lock()
	defer evictor.mutex.Unlock()
	evictor.accounts = append(evictor.accounts, accountID)
}

// fakeDirectory is Beny Bot's channel list for one linked server. unreadable makes every read fail as
// Beny Bot's does when its gateway can't see the server.
type fakeDirectory struct {
	channels   map[string][]core.DiscordChannel
	unreadable bool
}

func (directory *fakeDirectory) TextChannels(_ context.Context, guildID string) ([]core.DiscordChannel, error) {
	if directory.unreadable {
		return nil, fmt.Errorf("guild %s: %w", guildID, core.ErrDiscordListUnavailable)
	}
	return directory.channels[guildID], nil
}

// fixture is a fresh database with the site owner signed in; campaigns are made through the real
// request and approval path.
type fixture struct {
	t         *testing.T
	context   context.Context
	service   *core.Service
	database  *dbtest.Database
	notifier  *countingNotifier
	evictor   *recordingEvictor
	directory *fakeDirectory
	clock     *testClock
	owner     core.Caller
}

type testClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *testClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.now
}

func (clock *testClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.now = clock.now.Add(duration)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	database := dbtest.New(t)
	test := &fixture{
		t:         t,
		context:   context.Background(),
		database:  database,
		notifier:  &countingNotifier{},
		evictor:   &recordingEvictor{},
		directory: &fakeDirectory{channels: map[string][]core.DiscordChannel{}},
		clock:     &testClock{now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)},
	}
	test.service = core.New(core.Options{
		Pool:               database.Pool,
		Notifier:           test.notifier,
		Logger:             slog.New(slog.DiscardHandler),
		SiteOwnerDiscordID: testOwnerDiscordID,
		Sessions:           test.evictor,
		Directory:          test.directory,
		Now:                test.clock.Now,
		PublicURL:          testPublicURL,
	})
	test.owner = test.caller(testOwnerDiscordID, "owner")
	return test
}

// caller signs a Discord identity in and returns its Caller, as the auth middleware would build it.
func (test *fixture) caller(discordUserID string, username string) core.Caller {
	test.t.Helper()
	accountID, err := test.service.SignIn(test.context, core.DiscordIdentity{UserID: discordUserID, Username: username})
	if err != nil {
		test.t.Fatalf("sign in %s: %v", username, err)
	}
	return core.Caller{AccountID: accountID, DiscordUserID: discordUserID, IsSiteOwner: test.service.IsSiteOwner(discordUserID)}
}

// campaign makes a campaign DM'd by dm through RequestCampaign and the owner's approval, and returns
// its ID and the DM's Viewer.
func (test *fixture) campaign(dm core.Caller, name string, slug string) (uuid.UUID, core.Viewer) {
	test.t.Helper()
	request, err := test.service.RequestCampaign(test.context, dm, core.CampaignRequestInput{CampaignName: name, Slug: slug})
	if err != nil {
		test.t.Fatalf("request %s: %v", name, err)
	}
	if request.Status == core.CampaignRequestStatusPending {
		if _, err := test.service.ApproveCampaignRequest(test.context, test.owner, request.ID, core.ApproveCampaignRequestInput{}); err != nil {
			test.t.Fatalf("approve %s: %v", name, err)
		}
	}
	mine, err := test.service.MyCampaignRequest(test.context, dm)
	if err != nil || mine.Request == nil || mine.Request.Campaign == nil {
		test.t.Fatalf("the approved request names no campaign: %+v, %v", mine, err)
	}
	campaignID := mine.Request.Campaign.ID
	return campaignID, test.enter(dm, campaignID)
}

func (test *fixture) enter(caller core.Caller, campaignID uuid.UUID) core.Viewer {
	test.t.Helper()
	viewer, err := test.service.EnterCampaign(test.context, caller, campaignID)
	if err != nil {
		test.t.Fatalf("enter campaign: %v", err)
	}
	return viewer
}

func (test *fixture) inviteCode(dm core.Viewer) string {
	test.t.Helper()
	invite, err := test.service.Invite(test.context, dm)
	if err != nil {
		test.t.Fatalf("read the invite: %v", err)
	}
	return invite.InviteCode
}

// player signs a new account in, joins through the invite link and, when approve, is let in.
func (test *fixture) player(dm core.Viewer, discordUserID string, username string, approve bool) (core.Caller, core.Viewer) {
	test.t.Helper()
	caller := test.caller(discordUserID, username)
	return caller, test.join(caller, dm, approve)
}

func (test *fixture) join(caller core.Caller, dm core.Viewer, approve bool) core.Viewer {
	test.t.Helper()
	membership, err := test.service.JoinByInvite(test.context, caller, test.inviteCode(dm))
	if err != nil {
		test.t.Fatalf("join: %v", err)
	}
	if approve {
		if _, err := test.service.ApproveMember(test.context, dm, membership.MemberID); err != nil {
			test.t.Fatalf("approve: %v", err)
		}
	}
	return test.enter(caller, dm.CampaignID())
}

func discordID(index int) string {
	return fmt.Sprintf("7%017d", index)
}

func isKind(err error, kind core.ErrorKind) bool {
	operationError, typed := core.AsError(err)
	return typed && operationError.Kind == kind
}
