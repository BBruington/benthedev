package bot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

const (
	testGuildID   = "100000000000000001"
	testPublicURL = "http://localhost:5173"
	activeUserID  = "200000000000000001"
	dmUserID      = "200000000000000002"
	pendingUserID = "200000000000000003"
	removedUserID = "200000000000000004"
	strangerID    = "200000000000000005"
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

func (fake *fakeDiscordClient) recordedResponses() []*discordgo.InteractionResponse {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]*discordgo.InteractionResponse(nil), fake.responses...)
}

func (fake *fakeDiscordClient) recordedMessages() []sentMessage {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]sentMessage(nil), fake.messages...)
}

type fakeColonyService struct {
	mutex         sync.Mutex
	viewers       map[string]core.Viewer
	viewerError   error
	viewerLookups int
	channels      core.DiscordChannels
	channelsError error
}

func newFakeColonyService() *fakeColonyService {
	return &fakeColonyService{viewers: map[string]core.Viewer{
		activeUserID:  {MemberID: uuid.New(), Status: store.MemberStatusACTIVE},
		dmUserID:      {MemberID: uuid.New(), Status: store.MemberStatusACTIVE, IsDM: true},
		pendingUserID: {MemberID: uuid.New(), Status: store.MemberStatusPENDING},
		removedUserID: {MemberID: uuid.New(), Status: store.MemberStatusREMOVED},
	}}
}

func (fake *fakeColonyService) ViewerByDiscordID(_ context.Context, discordUserID string) (core.Viewer, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.viewerLookups++
	if fake.viewerError != nil {
		return core.Viewer{}, fake.viewerError
	}
	viewer, found := fake.viewers[discordUserID]
	if !found {
		return core.Viewer{}, core.NotFound("no member with that Discord account")
	}
	return viewer, nil
}

func (fake *fakeColonyService) DiscordChannels(context.Context) (core.DiscordChannels, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.channels, fake.channelsError
}

func (fake *fakeColonyService) setDmChannel(channelID string) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.channels.DmChannelID = &channelID
}

func (fake *fakeColonyService) lookups() int {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.viewerLookups
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

// newRunningTestBot returns a Bot wired to fakes, in the state Run leaves it in after connecting.
func newRunningTestBot(t *testing.T) (*Bot, *fakeDiscordClient, *fakeColonyService, *fakeClock) {
	t.Helper()
	client := newFakeDiscordClient()
	service := newFakeColonyService()
	clock := newFakeClock()
	benyBot := newBot(Options{
		Token:       "test-token",
		GuildID:     testGuildID,
		DmDiscordID: dmUserID,
		PublicURL:   testPublicURL + "/",
		Logger:      quietLogger(),
	}, client, clock.now)
	if err := benyBot.start(service); err != nil {
		t.Fatalf("start: %v", err)
	}
	return benyBot, client, service, clock
}

func commandInteraction(guildID string, discordUserID string, commandName string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID:      "300000000000000001",
		Type:    discordgo.InteractionApplicationCommand,
		GuildID: guildID,
		Member:  &discordgo.Member{User: &discordgo.User{ID: discordUserID}},
		Data:    discordgo.ApplicationCommandInteractionData{Name: commandName},
	}}
}

func requireOneEphemeralReply(t *testing.T, client *fakeDiscordClient, expectedContent string) {
	t.Helper()
	responses := client.recordedResponses()
	if len(responses) != 1 {
		t.Fatalf("got %d responses, want 1", len(responses))
	}
	response := responses[0]
	if response.Type != discordgo.InteractionResponseChannelMessageWithSource {
		t.Errorf("response type = %v, want a channel message", response.Type)
	}
	if response.Data == nil {
		t.Fatal("response has no data")
	}
	if response.Data.Content != expectedContent {
		t.Errorf("content = %q, want %q", response.Data.Content, expectedContent)
	}
	if response.Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
		t.Error("reply is not ephemeral")
	}
	if response.Data.AllowedMentions == nil || response.Data.AllowedMentions.Parse == nil || len(response.Data.AllowedMentions.Parse) != 0 {
		t.Errorf("reply allows mentions: %+v", response.Data.AllowedMentions)
	}
}

func TestCommandReplies(t *testing.T) {
	signInFirst := "Sign in at http://localhost:5173 first"
	for _, testCase := range []struct {
		name          string
		discordUserID string
		commandName   string
		expected      string
	}{
		{"an active player gets Boy", activeUserID, "beny", "Boy!"},
		{"the DM gets Boy", dmUserID, "beny", "Boy!"},
		{"an unknown Discord user is told to sign in", strangerID, "beny", signInFirst},
		{"a pending member is told to sign in", pendingUserID, "beny", signInFirst},
		{"a removed member is told to sign in", removedUserID, "beny", signInFirst},
		{"an unregistered command gets a polite refusal", activeUserID, "schedule", unknownCommandMessage},
		{"inactive callers learn nothing about which commands exist", strangerID, "schedule", signInFirst},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			benyBot, client, _, _ := newRunningTestBot(t)
			benyBot.handleInteraction(t.Context(), commandInteraction(testGuildID, testCase.discordUserID, testCase.commandName))
			requireOneEphemeralReply(t, client, testCase.expected)
		})
	}
}

func TestCommandsSeeThePlayerProjection(t *testing.T) {
	benyBot, client, _, _ := newRunningTestBot(t)
	var received core.Viewer
	benyBot.commands = append(benyBot.commands, command{
		definition: &discordgo.ApplicationCommand{Name: "whoami", Description: "test"},
		handle: func(_ context.Context, viewer core.Viewer, _ discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
			received = viewer
			return &discordgo.InteractionResponseData{Content: "ok"}, nil
		},
	})
	benyBot.handleInteraction(t.Context(), commandInteraction(testGuildID, dmUserID, "whoami"))
	requireOneEphemeralReply(t, client, "ok")
	if !received.IsDM || received.SeesDmFields() {
		t.Errorf("the DM's command saw IsDM=%v SeesDmFields=%v, want true and false", received.IsDM, received.SeesDmFields())
	}
}

func TestInteractionsOutsideTheGuildAreIgnored(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		guildID string
	}{
		{"another server", "999999999999999999"},
		{"a direct message", ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			benyBot, client, service, _ := newRunningTestBot(t)
			benyBot.handleInteraction(t.Context(), commandInteraction(testCase.guildID, activeUserID, "beny"))
			if responses := client.recordedResponses(); len(responses) != 0 {
				t.Errorf("got %d responses, want none", len(responses))
			}
			if lookups := service.lookups(); lookups != 0 {
				t.Errorf("looked up the caller %d times, want 0", lookups)
			}
		})
	}
}

func TestNonCommandInteractionsAreIgnored(t *testing.T) {
	benyBot, client, service, _ := newRunningTestBot(t)
	benyBot.handleInteraction(t.Context(), &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:    discordgo.InteractionMessageComponent,
		GuildID: testGuildID,
		Member:  &discordgo.Member{User: &discordgo.User{ID: activeUserID}},
		Data:    discordgo.MessageComponentInteractionData{CustomID: "req:1:grant"},
	}})
	benyBot.handleInteraction(t.Context(), nil)
	benyBot.handleInteraction(t.Context(), &discordgo.InteractionCreate{})
	if responses := client.recordedResponses(); len(responses) != 0 {
		t.Errorf("got %d responses, want none", len(responses))
	}
	if lookups := service.lookups(); lookups != 0 {
		t.Errorf("looked up the caller %d times, want 0", lookups)
	}
}

func TestViewerLookupFailureGetsAGenericReply(t *testing.T) {
	benyBot, client, service, _ := newRunningTestBot(t)
	service.viewerError = errors.New("connection refused")
	benyBot.handleInteraction(t.Context(), commandInteraction(testGuildID, activeUserID, "beny"))
	requireOneEphemeralReply(t, client, failureMessage)
}

func TestCommandsAreRateLimitedPerUser(t *testing.T) {
	benyBot, client, service, clock := newRunningTestBot(t)
	for range commandBurst {
		benyBot.handleInteraction(t.Context(), commandInteraction(testGuildID, activeUserID, "beny"))
	}
	benyBot.handleInteraction(t.Context(), commandInteraction(testGuildID, activeUserID, "beny"))
	// Another member has their own allowance.
	benyBot.handleInteraction(t.Context(), commandInteraction(testGuildID, dmUserID, "beny"))
	clock.advance(commandRefillInterval)
	benyBot.handleInteraction(t.Context(), commandInteraction(testGuildID, activeUserID, "beny"))

	expected := []string{"Boy!", "Boy!", "Boy!", slowDownMessage, "Boy!", "Boy!"}
	responses := client.recordedResponses()
	if len(responses) != len(expected) {
		t.Fatalf("got %d responses, want %d", len(responses), len(expected))
	}
	for index, response := range responses {
		if response.Data.Content != expected[index] {
			t.Errorf("response %d = %q, want %q", index, response.Data.Content, expected[index])
		}
		if response.Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
			t.Errorf("response %d is not ephemeral", index)
		}
	}
	// The limited command never reached the database.
	if lookups := service.lookups(); lookups != len(expected)-1 {
		t.Errorf("looked up callers %d times, want %d", lookups, len(expected)-1)
	}
}

func TestReadyRegistersGuildCommandsAndClearsGlobalOnes(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		ready                 *discordgo.Ready
		expectedApplicationID string
	}{
		{
			"application ID from the ready event",
			&discordgo.Ready{User: &discordgo.User{ID: "400000000000000001"}, Application: &discordgo.Application{ID: "400000000000000002"}},
			"400000000000000002",
		},
		{
			"bot user ID when the event names no application",
			&discordgo.Ready{User: &discordgo.User{ID: "400000000000000001"}},
			"400000000000000001",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			benyBot, client, _, _ := newRunningTestBot(t)
			benyBot.handleReady(t.Context(), testCase.ready)

			if len(client.overwrites) != 2 {
				t.Fatalf("got %d overwrites, want 2", len(client.overwrites))
			}
			guildOverwrite, globalOverwrite := client.overwrites[0], client.overwrites[1]
			if guildOverwrite.guildID != testGuildID || guildOverwrite.applicationID != testCase.expectedApplicationID {
				t.Errorf("guild overwrite went to application %q, guild %q", guildOverwrite.applicationID, guildOverwrite.guildID)
			}
			var names []string
			for _, definition := range guildOverwrite.commands {
				names = append(names, definition.Name)
			}
			if len(names) != 1 || names[0] != "beny" {
				t.Errorf("guild commands = %v, want [benyBot]", names)
			}
			if globalOverwrite.guildID != "" || globalOverwrite.applicationID != testCase.expectedApplicationID {
				t.Errorf("global overwrite went to application %q, guild %q", globalOverwrite.applicationID, globalOverwrite.guildID)
			}
			encoded, err := json.Marshal(globalOverwrite.commands)
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != "[]" {
				t.Errorf("global commands encode as %s, want []", encoded)
			}
		})
	}
}

func TestReadyWithoutAnApplicationRegistersNothing(t *testing.T) {
	benyBot, client, _, _ := newRunningTestBot(t)
	benyBot.handleReady(t.Context(), &discordgo.Ready{})
	benyBot.handleReady(t.Context(), nil)
	if len(client.overwrites) != 0 {
		t.Errorf("got %d overwrites, want none", len(client.overwrites))
	}
}

func TestRunConnectsServesAndDisconnects(t *testing.T) {
	client := newFakeDiscordClient()
	service := newFakeColonyService()
	benyBot := newBot(Options{Token: "test-token", GuildID: testGuildID, PublicURL: testPublicURL, Logger: quietLogger()}, client, newFakeClock().now)

	runContext, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- benyBot.run(runContext, service) }()
	<-client.openedSignal

	client.mutex.Lock()
	handlers := append([]any(nil), client.handlers...)
	client.mutex.Unlock()
	if len(handlers) != 2 {
		t.Fatalf("registered %d handlers, want 2", len(handlers))
	}
	readyHandler, isReadyHandler := handlers[0].(func(*discordgo.Session, *discordgo.Ready))
	interactionHandler, isInteractionHandler := handlers[1].(func(*discordgo.Session, *discordgo.InteractionCreate))
	if !isReadyHandler || !isInteractionHandler {
		t.Fatalf("handlers have the wrong shapes for discordgo: %T, %T", handlers[0], handlers[1])
	}
	readyHandler(nil, &discordgo.Ready{User: &discordgo.User{ID: "400000000000000001"}})
	interactionHandler(nil, commandInteraction(testGuildID, activeUserID, "beny"))
	requireOneEphemeralReply(t, client, "Boy!")

	cancel()
	if err := <-result; err != nil {
		t.Fatalf("run: %v", err)
	}
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if client.openCalls != 1 || client.closeCalls != 1 {
		t.Errorf("open %d times and close %d times, want 1 and 1", client.openCalls, client.closeCalls)
	}
	if client.removedHandlers != 2 {
		t.Errorf("removed %d handlers, want 2", client.removedHandlers)
	}
	if len(client.overwrites) != 2 {
		t.Errorf("got %d overwrites, want 2", len(client.overwrites))
	}
}

func TestRunReportsAFailedConnection(t *testing.T) {
	client := newFakeDiscordClient()
	client.openError = errors.New("websocket: bad handshake")
	benyBot := newBot(Options{Token: "test-token", GuildID: testGuildID, Logger: quietLogger()}, client, newFakeClock().now)
	if err := benyBot.run(t.Context(), newFakeColonyService()); err == nil {
		t.Fatal("expected an error")
	}
	if err := benyBot.run(t.Context(), newFakeColonyService()); err == nil {
		t.Fatal("expected a second Run to be refused")
	}
}

func TestNewRequiresTheGuildWithAToken(t *testing.T) {
	if _, err := New(Options{Token: "test-token", Logger: quietLogger()}); err == nil {
		t.Fatal("expected an error without DISCORD_GUILD_ID")
	}
	benyBot, err := New(Options{Token: "test-token", GuildID: testGuildID, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	if !benyBot.Enabled() {
		t.Fatal("Beny Bot should be on with a token and a guild")
	}
	session, isSession := benyBot.client.(*discordgo.Session)
	if !isSession {
		t.Fatalf("client is %T, want a discordgo session", benyBot.client)
	}
	expectedIntents := discordgo.IntentsGuilds | discordgo.IntentsGuildScheduledEvents
	if session.Identify.Intents != expectedIntents {
		t.Errorf("intents = %d, want %d", session.Identify.Intents, expectedIntents)
	}
}

func TestBenyBotIsOffWithoutAToken(t *testing.T) {
	benyBot, err := New(Options{GuildID: testGuildID, Logger: quietLogger()})
	if err != nil {
		t.Fatal(err)
	}
	if benyBot.Enabled() {
		t.Fatal("Beny Bot should be off without DISCORD_BOT_TOKEN")
	}
	benyBot.MemberWaiting(t.Context(), core.WaitingMember{MemberID: uuid.New(), DisplayName: "Ana", DiscordUsername: "ana"})

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
