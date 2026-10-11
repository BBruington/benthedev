package bot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
)

// Discord IDs of the test world. Tests never read SITE_OWNER_DISCORD_ID or DISCORD_GUILD_ID: every
// value comes from here.
const (
	testPublicURL = "http://localhost:5173"

	ownerDiscordID    = "200000000000000009"
	dmUserID          = "200000000000000001" // DM of campaign A
	activeUserID      = "200000000000000002" // Ana: a player in A and in B
	pendingUserID     = "200000000000000003" // waiting to join A
	removedUserID     = "200000000000000004" // removed from A
	strangerUserID    = "200000000000000005" // signed in, in no campaign
	unknownUserID     = "200000000000000006" // never signed in
	otherDmUserID     = "200000000000000007" // DM of campaign B
	otherOnlyUserID   = "200000000000000008" // a player in B only
	legacyTestGuildID = "100000000000000099"

	guildX = "100000000000000001" // linked to A in most tests
	guildY = "100000000000000002" // linked to B in some tests
	guildZ = "100000000000000003" // never linked

	channelXDesk    = "300000000000000001"
	channelXGeneral = "300000000000000002"
	channelXVoice   = "300000000000000003"
	channelXNews    = "300000000000000004"
	channelYDesk    = "300000000000000011"

	campaignAName = "Sample Colony"
	campaignASlug = "sample-colony"
	// campaignBName must never appear in an answer given in A's server.
	campaignBName = "Second Sample"
	campaignBSlug = "second-sample"

	manageServer = int64(discordgo.PermissionManageGuild)
)

type commandOverwrite struct {
	applicationID string
	guildID       string
	commands      []*discordgo.ApplicationCommand
}

type sentMessage struct {
	channelID string
	message   *discordgo.MessageSend
}

type fakeDiscordClient struct {
	mutex            sync.Mutex
	handlers         []any
	removedHandlers  int
	openCalls        int
	closeCalls       int
	openError        error
	overwrites       []commandOverwrite
	responses        []*discordgo.InteractionResponse
	messages         []sentMessage
	directChannels   []string
	sendError        error
	openedSignal     chan struct{}
	openedSignalOnce sync.Once
}

func newFakeDiscordClient() *fakeDiscordClient {
	return &fakeDiscordClient{openedSignal: make(chan struct{})}
}

func (fake *fakeDiscordClient) AddHandler(handler any) func() {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.handlers = append(fake.handlers, handler)
	return func() {
		fake.mutex.Lock()
		defer fake.mutex.Unlock()
		fake.removedHandlers++
	}
}

func (fake *fakeDiscordClient) Open() error {
	fake.mutex.Lock()
	fake.openCalls++
	openError := fake.openError
	fake.mutex.Unlock()
	fake.openedSignalOnce.Do(func() { close(fake.openedSignal) })
	return openError
}

func (fake *fakeDiscordClient) Close() error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.closeCalls++
	return nil
}

func (fake *fakeDiscordClient) ApplicationCommandBulkOverwrite(applicationID string, guildID string, commands []*discordgo.ApplicationCommand, _ ...discordgo.RequestOption) ([]*discordgo.ApplicationCommand, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.overwrites = append(fake.overwrites, commandOverwrite{applicationID: applicationID, guildID: guildID, commands: commands})
	return commands, nil
}

func (fake *fakeDiscordClient) InteractionRespond(_ *discordgo.Interaction, response *discordgo.InteractionResponse, _ ...discordgo.RequestOption) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.responses = append(fake.responses, response)
	return nil
}

func (fake *fakeDiscordClient) ChannelMessageSendComplex(channelID string, message *discordgo.MessageSend, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.messages = append(fake.messages, sentMessage{channelID: channelID, message: message})
	if fake.sendError != nil {
		return nil, fake.sendError
	}
	return &discordgo.Message{ChannelID: channelID, Content: message.Content}, nil
}

func (fake *fakeDiscordClient) UserChannelCreate(recipientID string, _ ...discordgo.RequestOption) (*discordgo.Channel, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.directChannels = append(fake.directChannels, recipientID)
	return &discordgo.Channel{ID: directChannelID(recipientID), Type: discordgo.ChannelTypeDM}, nil
}

func directChannelID(recipientID string) string {
	return "direct:" + recipientID
}

func (fake *fakeDiscordClient) recordedResponses() []*discordgo.InteractionResponse {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]*discordgo.InteractionResponse(nil), fake.responses...)
}

func (fake *fakeDiscordClient) messagesTo(channelID string) []sentMessage {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	var found []sentMessage
	for _, message := range fake.messages {
		if message.channelID == channelID {
			found = append(found, message)
		}
	}
	return found
}

func (fake *fakeDiscordClient) recordedOverwrites() []commandOverwrite {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]commandOverwrite(nil), fake.overwrites...)
}

// fakeGatewayState is discordgo's gateway cache for the servers Beny Bot is in.
type fakeGatewayState struct {
	sync.RWMutex
	guilds   map[string]*discordgo.Guild
	channels map[string]*discordgo.Channel
}

func newFakeGatewayState() *fakeGatewayState {
	state := &fakeGatewayState{guilds: map[string]*discordgo.Guild{}, channels: map[string]*discordgo.Channel{}}
	state.addGuild(guildX, "Colony Planner Test",
		&discordgo.Channel{ID: channelXDesk, Name: "dm-desk", Type: discordgo.ChannelTypeGuildText, Position: 2},
		&discordgo.Channel{ID: channelXGeneral, Name: "general", Type: discordgo.ChannelTypeGuildText, Position: 1},
		&discordgo.Channel{ID: channelXVoice, Name: "Tavern", Type: discordgo.ChannelTypeGuildVoice, Position: 0},
		&discordgo.Channel{ID: channelXNews, Name: "news", Type: discordgo.ChannelTypeGuildNews, Position: 3},
	)
	state.addGuild(guildY, "Ridgehold Table",
		&discordgo.Channel{ID: channelYDesk, Name: "dm-desk", Type: discordgo.ChannelTypeGuildText, Position: 1},
	)
	state.addGuild(guildZ, "Somebody Else's Server")
	return state
}

func (state *fakeGatewayState) addGuild(guildID string, name string, channels ...*discordgo.Channel) {
	state.Lock()
	defer state.Unlock()
	for _, channel := range channels {
		channel.GuildID = guildID
		state.channels[channel.ID] = channel
	}
	state.guilds[guildID] = &discordgo.Guild{ID: guildID, Name: name, Channels: channels}
}

// forgetGuild drops a server from the gateway state, as right after a restart before its GuildCreate
// arrives, or after Beny Bot has left it.
func (state *fakeGatewayState) forgetGuild(guildID string) {
	state.Lock()
	defer state.Unlock()
	delete(state.guilds, guildID)
}

// markUnavailable makes the gateway report a server as unavailable, as during a Discord outage.
func (state *fakeGatewayState) markUnavailable(guildID string) {
	state.Lock()
	defer state.Unlock()
	state.guilds[guildID].Unavailable = true
}

// moveChannel makes the gateway report a channel as another server's.
func (state *fakeGatewayState) moveChannel(channelID string, guildID string) {
	state.Lock()
	defer state.Unlock()
	state.channels[channelID].GuildID = guildID
}

func (state *fakeGatewayState) Guild(guildID string) (*discordgo.Guild, error) {
	state.RLock()
	defer state.RUnlock()
	if guild, found := state.guilds[guildID]; found {
		return guild, nil
	}
	return nil, discordgo.ErrStateNotFound
}

func (state *fakeGatewayState) Channel(channelID string) (*discordgo.Channel, error) {
	state.RLock()
	defer state.RUnlock()
	if channel, found := state.channels[channelID]; found {
		return channel, nil
	}
	return nil, discordgo.ErrStateNotFound
}

type fakeClock struct {
	mutex   sync.Mutex
	current time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{current: time.Date(2026, time.October, 10, 20, 0, 0, 0, time.UTC)}
}

func (clock *fakeClock) now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.current
}

func (clock *fakeClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.current = clock.current.Add(duration)
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// world is Beny Bot running against a real core and a fresh database: only core can build a Viewer,
// so routing is tested end to end. Campaign A (DM dmUserID) has Ana ACTIVE, one PENDING and one
// REMOVED member; campaign B (DM otherDmUserID) has Ana and otherOnlyUserID ACTIVE. Nothing is linked.
type world struct {
	t       *testing.T
	context context.Context
	bot     *Bot
	client  *fakeDiscordClient
	state   *fakeGatewayState
	service *core.Service
	clock   *fakeClock
	owner   core.Caller
	dmA     core.Viewer
	dmB     core.Viewer
}

func newWorld(t *testing.T) *world {
	t.Helper()
	database := dbtest.New(t)
	test := &world{
		t:       t,
		context: t.Context(),
		client:  newFakeDiscordClient(),
		state:   newFakeGatewayState(),
		clock:   newFakeClock(),
	}
	test.bot = newBot(Options{
		Token:              "test-token",
		SiteOwnerDiscordID: ownerDiscordID,
		PublicURL:          testPublicURL + "/",
		Logger:             quietLogger(),
	}, test.client, test.state, test.clock.now)
	test.service = core.New(core.Options{
		Pool:               database.Pool,
		Notifier:           test.bot,
		Logger:             quietLogger(),
		SiteOwnerDiscordID: ownerDiscordID,
		Directory:          test.bot.Directory(),
		Now:                test.clock.now,
	})
	if err := test.bot.start(test.service); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(test.bot.stop)

	test.owner = test.signIn(ownerDiscordID, "owner")
	test.dmA = test.campaign(test.signIn(dmUserID, "sample.dm"), campaignAName, campaignASlug)
	test.dmB = test.campaign(test.signIn(otherDmUserID, "hollis"), campaignBName, campaignBSlug)
	ana := test.signIn(activeUserID, "ana")
	test.join(ana, test.dmA, true)
	test.join(ana, test.dmB, true)
	test.join(test.signIn(pendingUserID, "jun"), test.dmA, false)
	removed := test.join(test.signIn(removedUserID, "gus"), test.dmA, true)
	if _, err := test.service.RemoveMember(test.context, test.dmA, removed.MemberID()); err != nil {
		t.Fatalf("remove: %v", err)
	}
	test.join(test.signIn(otherOnlyUserID, "brisa"), test.dmB, true)
	test.signIn(strangerUserID, "stranger")
	test.bot.posts.Wait()
	return test
}

func (test *world) signIn(discordUserID string, username string) core.Caller {
	test.t.Helper()
	accountID, err := test.service.SignIn(test.context, core.DiscordIdentity{UserID: discordUserID, Username: username, DisplayName: username})
	if err != nil {
		test.t.Fatalf("sign in %s: %v", username, err)
	}
	return core.Caller{AccountID: accountID, DiscordUserID: discordUserID, IsSiteOwner: test.service.IsSiteOwner(discordUserID)}
}

// campaign makes a campaign through RequestCampaign and the owner's approval, and returns its DM.
func (test *world) campaign(dm core.Caller, name string, slug string) core.Viewer {
	test.t.Helper()
	request, err := test.service.RequestCampaign(test.context, dm, core.CampaignRequestInput{CampaignName: name, Slug: slug})
	if err != nil {
		test.t.Fatalf("request %s: %v", name, err)
	}
	if _, err := test.service.ApproveCampaignRequest(test.context, test.owner, request.ID, core.ApproveCampaignRequestInput{}); err != nil {
		test.t.Fatalf("approve %s: %v", name, err)
	}
	mine, err := test.service.MyCampaignRequest(test.context, dm)
	if err != nil || mine.Request == nil || mine.Request.Campaign == nil {
		test.t.Fatalf("the approved request names no campaign: %+v, %v", mine, err)
	}
	return test.enter(dm, mine.Request.Campaign.ID)
}

func (test *world) enter(caller core.Caller, campaignID uuid.UUID) core.Viewer {
	test.t.Helper()
	viewer, err := test.service.EnterCampaign(test.context, caller, campaignID)
	if err != nil {
		test.t.Fatalf("enter campaign: %v", err)
	}
	return viewer
}

func (test *world) join(caller core.Caller, dm core.Viewer, approve bool) core.Viewer {
	test.t.Helper()
	invite, err := test.service.Invite(test.context, dm)
	if err != nil {
		test.t.Fatalf("read the invite: %v", err)
	}
	membership, err := test.service.JoinByInvite(test.context, caller, invite.InviteCode)
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

func (test *world) linkCode(dm core.Viewer) string {
	test.t.Helper()
	code, err := test.service.CreateLinkCode(test.context, dm)
	if err != nil {
		test.t.Fatalf("link code: %v", err)
	}
	return code.Code
}

// link runs /link in the server with a fresh code from dm's campaign, as a member with Manage Server.
func (test *world) link(guildID string, dm core.Viewer) {
	test.t.Helper()
	reply := test.run(serverCommand(guildID, dmUserID, "link", manageServer, codeOption(test.linkCode(dm))))
	if !strings.HasPrefix(reply, "Linked this server to ") {
		test.t.Fatalf("/link answered %q", reply)
	}
}

// run hands Beny Bot one interaction and returns its one reply, which must be ephemeral and
// mention-free. The clock moves on first, so a test's commands never trip the per-user limiter.
func (test *world) run(event *discordgo.InteractionCreate) string {
	test.t.Helper()
	test.clock.advance(commandRefillInterval)
	before := len(test.client.recordedResponses())
	test.bot.handleInteraction(test.context, event)
	responses := test.client.recordedResponses()
	if len(responses) != before+1 {
		test.t.Fatalf("got %d replies, want 1", len(responses)-before)
	}
	return requireEphemeral(test.t, responses[before])
}

func requireEphemeral(t *testing.T, response *discordgo.InteractionResponse) string {
	t.Helper()
	if response.Type != discordgo.InteractionResponseChannelMessageWithSource {
		t.Errorf("response type = %v, want a channel message", response.Type)
	}
	if response.Data == nil {
		t.Fatal("response has no data")
	}
	if response.Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
		t.Error("reply is not ephemeral")
	}
	if response.Data.AllowedMentions == nil || response.Data.AllowedMentions.Parse == nil || len(response.Data.AllowedMentions.Parse) != 0 {
		t.Errorf("reply allows mentions: %+v", response.Data.AllowedMentions)
	}
	return response.Data.Content
}

// slashCommand is a command from a server member without special permissions, or a direct message
// when guildID is empty.
func slashCommand(guildID string, discordUserID string, name string) *discordgo.InteractionCreate {
	return serverCommand(guildID, discordUserID, name, 0)
}

func serverCommand(guildID string, discordUserID string, name string, permissions int64, options ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.InteractionCreate {
	interaction := &discordgo.Interaction{
		ID:      "400000000000000001",
		Type:    discordgo.InteractionApplicationCommand,
		GuildID: guildID,
		Data:    discordgo.ApplicationCommandInteractionData{Name: name, Options: options},
	}
	user := &discordgo.User{ID: discordUserID}
	if guildID == "" {
		interaction.User = user
	} else {
		interaction.Member = &discordgo.Member{User: user, Permissions: permissions}
	}
	return &discordgo.InteractionCreate{Interaction: interaction}
}

func codeOption(code string) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{Name: "code", Type: discordgo.ApplicationCommandOptionString, Value: code}
}

func TestCommandsRouteByServerAndMembership(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)

	for _, testCase := range []struct {
		name          string
		guildID       string
		discordUserID string
		commandName   string
		expected      string
	}{
		{"an active player gets Boy", guildX, activeUserID, "beny", "Boy!"},
		{"the DM gets Boy", guildX, dmUserID, "beny", "Boy!"},
		{"a signed-in account in no campaign", guildX, strangerUserID, "beny", core.ErrNotInCampaign.Message},
		{"a Discord user who never signed in", guildX, unknownUserID, "beny", core.ErrNotInCampaign.Message},
		{"a member of another campaign only", guildX, otherOnlyUserID, "beny", core.ErrNotInCampaign.Message},
		{"another campaign's DM", guildX, otherDmUserID, "beny", core.ErrNotInCampaign.Message},
		{"a pending member", guildX, pendingUserID, "beny", core.ErrNotLetIn.Message},
		{"a removed member", guildX, removedUserID, "beny", core.ErrNotLetIn.Message},
		{"an unlinked server", guildZ, activeUserID, "beny", core.ErrGuildNotLinked.Message},
		{"a direct message", "", activeUserID, "beny", useInServerMessage},
		{"an unregistered command gets a polite refusal", guildX, activeUserID, "schedule", unknownCommandMessage},
		{"outsiders learn nothing about which commands exist", guildX, strangerUserID, "schedule", core.ErrNotInCampaign.Message},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reply := test.run(slashCommand(testCase.guildID, testCase.discordUserID, testCase.commandName))
			if reply != testCase.expected {
				t.Errorf("reply = %q, want %q", reply, testCase.expected)
			}
			if strings.Contains(reply, campaignBName) || strings.Contains(reply, campaignAName) {
				t.Errorf("reply names a campaign: %q", reply)
			}
		})
	}
}

// A command in server X reaches only the campaign linked to X, whoever runs it.
func TestServersNeverReachAnotherCampaign(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.link(guildY, test.dmB)

	var reached []uuid.UUID
	var dmFlags []bool
	test.bot.commands = append(test.bot.commands, command{
		definition: &discordgo.ApplicationCommand{Name: "whereami", Description: "test"},
		handle: func(_ context.Context, viewer core.Viewer, _ discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
			reached = append(reached, viewer.CampaignID())
			dmFlags = append(dmFlags, viewer.IsDM())
			return &discordgo.InteractionResponseData{Content: "ok"}, nil
		},
	})

	// Ana plays in both: each server answers for its own campaign.
	test.run(slashCommand(guildX, activeUserID, "whereami"))
	test.run(slashCommand(guildY, activeUserID, "whereami"))
	// B's DM in A's server is nobody there, and A's DM in B's server likewise.
	if reply := test.run(slashCommand(guildX, otherDmUserID, "whereami")); reply != core.ErrNotInCampaign.Message {
		t.Errorf("B's DM in A's server got %q", reply)
	}
	if reply := test.run(slashCommand(guildY, dmUserID, "whereami")); reply != core.ErrNotInCampaign.Message {
		t.Errorf("A's DM in B's server got %q", reply)
	}
	if reply := test.run(slashCommand(guildX, otherOnlyUserID, "whereami")); reply != core.ErrNotInCampaign.Message {
		t.Errorf("B's player in A's server got %q", reply)
	}
	expected := []uuid.UUID{test.dmA.CampaignID(), test.dmB.CampaignID()}
	if !slices.Equal(reached, expected) {
		t.Fatalf("commands reached campaigns %v, want %v", reached, expected)
	}
	if slices.Contains(dmFlags, true) {
		t.Error("a player's command reached a campaign as its DM")
	}
}

func TestCommandsSeeThePlayerProjection(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	var received core.Viewer
	test.bot.commands = append(test.bot.commands, command{
		definition: &discordgo.ApplicationCommand{Name: "whoami", Description: "test"},
		handle: func(_ context.Context, viewer core.Viewer, _ discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
			received = viewer
			return &discordgo.InteractionResponseData{Content: "ok"}, nil
		},
	})
	if reply := test.run(slashCommand(guildX, dmUserID, "whoami")); reply != "ok" {
		t.Fatalf("reply = %q", reply)
	}
	if !received.IsDM() || received.SeesDmFields() || received.CampaignID() != test.dmA.CampaignID() {
		t.Errorf("the DM's command saw IsDM=%v SeesDmFields=%v campaign=%v, want true, false and A", received.IsDM(), received.SeesDmFields(), received.CampaignID())
	}
}

func TestSuspendedAndDeletedCampaignsAnswerInTheirServer(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)

	if _, err := test.service.SuspendCampaign(test.context, test.owner, test.dmA.CampaignID()); err != nil {
		t.Fatal(err)
	}
	if reply := test.run(slashCommand(guildX, activeUserID, "beny")); reply != core.ErrCampaignSuspended.Message {
		t.Errorf("a suspended campaign's server answered %q", reply)
	}
	if _, err := test.service.UnsuspendCampaign(test.context, test.owner, test.dmA.CampaignID()); err != nil {
		t.Fatal(err)
	}
	if reply := test.run(slashCommand(guildX, activeUserID, "beny")); reply != "Boy!" {
		t.Errorf("after unsuspending, the server answered %q", reply)
	}

	if err := test.service.DeleteOwnCampaign(test.context, test.dmA, core.DeleteCampaignInput{ConfirmSlug: campaignASlug}); err != nil {
		t.Fatal(err)
	}
	if reply := test.run(slashCommand(guildX, activeUserID, "beny")); reply != core.ErrGuildNotLinked.Message {
		t.Errorf("a deleted campaign's server answered %q", reply)
	}
}

// failingService answers every read with an infrastructure error.
type failingService struct{}

var errDatabaseDown = errors.New("connection refused")

func (failingService) ViewerForGuild(context.Context, string, string) (core.Viewer, error) {
	return core.Viewer{}, errDatabaseDown
}

func (failingService) DiscordTargetFor(context.Context, uuid.UUID) (core.DiscordTarget, bool, error) {
	return core.DiscordTarget{}, false, errDatabaseDown
}

func (failingService) LinkGuild(context.Context, core.LinkGuildInput) (core.LinkedGuild, error) {
	return core.LinkedGuild{}, errDatabaseDown
}

func (failingService) UnlinkGuildByID(context.Context, string) (bool, error) {
	return false, errDatabaseDown
}

func (failingService) ReconcileGuilds(context.Context, []string) (int, error) {
	return 0, errDatabaseDown
}

func TestFailuresGetAGenericReply(t *testing.T) {
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, client, newFakeGatewayState(), newFakeClock().now)
	if err := benyBot.start(failingService{}); err != nil {
		t.Fatal(err)
	}
	benyBot.handleInteraction(t.Context(), slashCommand(guildX, activeUserID, "beny"))
	benyBot.handleInteraction(t.Context(), serverCommand(guildX, activeUserID, "link", manageServer, codeOption("BENY-22222-22222")))
	benyBot.handleInteraction(t.Context(), serverCommand(guildX, activeUserID, "unlink", manageServer))
	responses := client.recordedResponses()
	if len(responses) != 3 {
		t.Fatalf("got %d replies, want 3", len(responses))
	}
	for _, response := range responses {
		if content := requireEphemeral(t, response); content != failureMessage {
			t.Errorf("reply = %q, want %q", content, failureMessage)
		}
	}
	// A failed check unlinks nothing and doesn't stop Beny Bot.
	benyBot.handleReady(t.Context(), &discordgo.Ready{User: &discordgo.User{ID: "500000000000000001"}})
	benyBot.handleGuildDelete(t.Context(), &discordgo.GuildDelete{Guild: &discordgo.Guild{ID: guildX}})
}

func TestNonCommandInteractionsAreIgnored(t *testing.T) {
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, client, newFakeGatewayState(), newFakeClock().now)
	if err := benyBot.start(failingService{}); err != nil {
		t.Fatal(err)
	}
	benyBot.handleInteraction(t.Context(), &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:    discordgo.InteractionMessageComponent,
		GuildID: guildX,
		Member:  &discordgo.Member{User: &discordgo.User{ID: activeUserID}},
		Data:    discordgo.MessageComponentInteractionData{CustomID: "req:1:grant"},
	}})
	benyBot.handleInteraction(t.Context(), nil)
	benyBot.handleInteraction(t.Context(), &discordgo.InteractionCreate{})
	if responses := client.recordedResponses(); len(responses) != 0 {
		t.Errorf("got %d responses, want none", len(responses))
	}
}

func TestCommandsAreRateLimitedPerUser(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.clock.advance(time.Hour)
	before := len(test.client.recordedResponses())

	for range commandBurst {
		test.bot.handleInteraction(test.context, slashCommand(guildX, activeUserID, "beny"))
	}
	test.bot.handleInteraction(test.context, slashCommand(guildX, activeUserID, "beny"))
	// Another member has their own allowance.
	test.bot.handleInteraction(test.context, slashCommand(guildX, dmUserID, "beny"))
	test.clock.advance(commandRefillInterval)
	test.bot.handleInteraction(test.context, slashCommand(guildX, activeUserID, "beny"))

	expected := []string{"Boy!", "Boy!", "Boy!", slowDownMessage, "Boy!", "Boy!"}
	responses := test.client.recordedResponses()[before:]
	if len(responses) != len(expected) {
		t.Fatalf("got %d responses, want %d", len(responses), len(expected))
	}
	for index, response := range responses {
		if content := requireEphemeral(t, response); content != expected[index] {
			t.Errorf("response %d = %q, want %q", index, content, expected[index])
		}
	}
}

func commandNames(definitions []*discordgo.ApplicationCommand) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	return names
}

func TestReadyRegistersCommandsGloballyAndClearsTheLegacyGuild(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		legacyGuildID         string
		ready                 *discordgo.Ready
		expectedApplicationID string
	}{
		{
			"application ID from the ready event, legacy guild still set",
			legacyTestGuildID,
			&discordgo.Ready{User: &discordgo.User{ID: "500000000000000001"}, Application: &discordgo.Application{ID: "500000000000000002"}},
			"500000000000000002",
		},
		{
			"bot user ID when the event names no application, legacy guild deleted",
			"",
			&discordgo.Ready{User: &discordgo.User{ID: "500000000000000001"}},
			"500000000000000001",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := newFakeDiscordClient()
			benyBot := newBot(Options{Token: "test-token", LegacyGuildID: testCase.legacyGuildID, Logger: quietLogger()}, client, newFakeGatewayState(), newFakeClock().now)
			benyBot.handleReady(t.Context(), testCase.ready)

			overwrites := client.recordedOverwrites()
			expectedOverwrites := 1
			if testCase.legacyGuildID != "" {
				expectedOverwrites = 2
			}
			if len(overwrites) != expectedOverwrites {
				t.Fatalf("got %d overwrites, want %d", len(overwrites), expectedOverwrites)
			}
			global := overwrites[0]
			if global.guildID != "" || global.applicationID != testCase.expectedApplicationID {
				t.Errorf("command registration went to application %q, guild %q; want global", global.applicationID, global.guildID)
			}
			if names := commandNames(global.commands); !slices.Equal(names, []string{"beny", "link", "unlink"}) {
				t.Errorf("global commands = %v", names)
			}
			for _, definition := range global.commands {
				if definition.Contexts == nil || !slices.Equal(*definition.Contexts, []discordgo.InteractionContextType{discordgo.InteractionContextGuild}) {
					t.Errorf("/%s is offered outside servers", definition.Name)
				}
				serverManagersOnly := definition.Name == "link" || definition.Name == "unlink"
				if serverManagersOnly && (definition.DefaultMemberPermissions == nil || *definition.DefaultMemberPermissions != manageServer) {
					t.Errorf("/%s is offered to members without Manage Server", definition.Name)
				}
				if !serverManagersOnly && definition.DefaultMemberPermissions != nil {
					t.Errorf("/%s is hidden from players", definition.Name)
				}
			}
			if testCase.legacyGuildID == "" {
				return
			}
			legacy := overwrites[1]
			if legacy.guildID != testCase.legacyGuildID || legacy.applicationID != testCase.expectedApplicationID {
				t.Errorf("legacy clean-up went to application %q, guild %q", legacy.applicationID, legacy.guildID)
			}
			encoded, err := json.Marshal(legacy.commands)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != "[]" {
				t.Errorf("legacy guild commands encode as %s, want []", encoded)
			}
		})
	}
}

func TestReadyWithoutAnApplicationRegistersNothing(t *testing.T) {
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, client, newFakeGatewayState(), newFakeClock().now)
	benyBot.handleReady(t.Context(), &discordgo.Ready{})
	benyBot.handleReady(t.Context(), nil)
	if overwrites := client.recordedOverwrites(); len(overwrites) != 0 {
		t.Errorf("got %d overwrites, want none", len(overwrites))
	}
}

func TestRunConnectsServesAndDisconnects(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", PublicURL: testPublicURL, Logger: quietLogger()}, client, test.state, newFakeClock().now)

	runContext, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- benyBot.run(runContext, test.service) }()
	<-client.openedSignal

	client.mutex.Lock()
	handlers := append([]any(nil), client.handlers...)
	client.mutex.Unlock()
	if len(handlers) != 3 {
		t.Fatalf("registered %d handlers, want 3", len(handlers))
	}
	readyHandler, isReadyHandler := handlers[0].(func(*discordgo.Session, *discordgo.Ready))
	interactionHandler, isInteractionHandler := handlers[1].(func(*discordgo.Session, *discordgo.InteractionCreate))
	_, isGuildDeleteHandler := handlers[2].(func(*discordgo.Session, *discordgo.GuildDelete))
	if !isReadyHandler || !isInteractionHandler || !isGuildDeleteHandler {
		t.Fatalf("handlers have the wrong shapes for discordgo: %T, %T, %T", handlers[0], handlers[1], handlers[2])
	}
	readyHandler(nil, &discordgo.Ready{User: &discordgo.User{ID: "500000000000000001"}, Guilds: []*discordgo.Guild{{ID: guildX}}})
	interactionHandler(nil, slashCommand(guildX, activeUserID, "beny"))
	responses := client.recordedResponses()
	if len(responses) != 1 || requireEphemeral(t, responses[0]) != "Boy!" {
		t.Fatalf("got %d responses, want Boy!", len(responses))
	}

	cancel()
	if err := <-result; err != nil {
		t.Fatalf("run: %v", err)
	}
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if client.openCalls != 1 || client.closeCalls != 1 {
		t.Errorf("open %d times and close %d times, want 1 and 1", client.openCalls, client.closeCalls)
	}
	if client.removedHandlers != 3 {
		t.Errorf("removed %d handlers, want 3", client.removedHandlers)
	}
	if len(client.overwrites) != 1 {
		t.Errorf("got %d overwrites, want 1", len(client.overwrites))
	}
}

func TestRunReportsAFailedConnection(t *testing.T) {
	client := newFakeDiscordClient()
	client.openError = errors.New("websocket: bad handshake")
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, client, newFakeGatewayState(), newFakeClock().now)
	if err := benyBot.run(t.Context(), failingService{}); err == nil {
		t.Fatal("expected an error")
	}
	if err := benyBot.run(t.Context(), failingService{}); err == nil {
		t.Fatal("expected a second Run to be refused")
	}
}

func TestNewWithATokenNeedsNoGuild(t *testing.T) {
	benyBot, err := New(Options{Token: "test-token", Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	if !benyBot.Enabled() || benyBot.Directory() == nil {
		t.Fatal("Beny Bot should be on with a token")
	}
	session, isSession := benyBot.client.(*discordgo.Session)
	if !isSession {
		t.Fatalf("client is %T, want a discordgo session", benyBot.client)
	}
	if benyBot.state != session.State {
		t.Error("Beny Bot doesn't read the session's gateway state")
	}
	expectedIntents := discordgo.IntentsGuilds | discordgo.IntentsGuildScheduledEvents
	if session.Identify.Intents != expectedIntents {
		t.Errorf("intents = %d, want %d", session.Identify.Intents, expectedIntents)
	}
}

func TestBenyBotIsOffWithoutAToken(t *testing.T) {
	benyBot, err := New(Options{LegacyGuildID: legacyTestGuildID, SiteOwnerDiscordID: ownerDiscordID, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	if benyBot.Enabled() {
		t.Fatal("Beny Bot should be off without DISCORD_BOT_TOKEN")
	}
	if benyBot.Directory() != nil {
		t.Error("an off Beny Bot offers a channel directory, so core can't tell it is off")
	}
	benyBot.MemberWaiting(t.Context(), core.WaitingMember{CampaignID: uuid.New(), MemberID: uuid.New(), DisplayName: "Ana", DiscordUsername: "ana"})
	benyBot.CampaignRequested(t.Context(), core.RequestedCampaign{CampaignRequestID: uuid.New(), CampaignName: "Sky Isles", DisplayName: "Mira", DiscordUsername: "mira"})

	runContext, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- benyBot.Run(runContext, nil) }()
	select {
	case err := <-result:
		t.Fatalf("Run returned before its context ended: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	if err := <-result; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
