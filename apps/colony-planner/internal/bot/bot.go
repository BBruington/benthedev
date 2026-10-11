// Package bot is Beny Bot, the Discord bot, running in-process over discordgo. It is off unless Beny Bot Dev's
// variables are set. Every reply and post is built from the player projection.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

// Discord drops an interaction that gets no response within three seconds.
const interactionDeadline = 3 * time.Second

const (
	slowDownMessage       = "You're sending commands too quickly. Please slow down."
	unknownCommandMessage = "Beny Bot doesn't know that command."
	failureMessage        = "Something went wrong. Try again in a moment."
)

type Options struct {
	Token   string
	GuildID string
	// DmDiscordID decides who may press Grant and Decline.
	DmDiscordID string
	// PublicURL builds the links in Beny Bot's messages, e.g. <PublicURL>/players.
	PublicURL string
	Logger    *slog.Logger
}

// discordClient is the part of *discordgo.Session that Beny Bot uses. It is the one interface at the
// Discord boundary (spec §8.6), so tests drive Beny Bot without a gateway connection.
type discordClient interface {
	AddHandler(handler any) func()
	Open() error
	Close() error
	ApplicationCommandBulkOverwrite(applicationID string, guildID string, commands []*discordgo.ApplicationCommand, options ...discordgo.RequestOption) ([]*discordgo.ApplicationCommand, error)
	InteractionRespond(interaction *discordgo.Interaction, response *discordgo.InteractionResponse, options ...discordgo.RequestOption) error
	ChannelMessageSendComplex(channelID string, message *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

// colonyService is what Beny Bot reads from core. *core.Service satisfies it; tests substitute a fake so
// routing is tested without Postgres.
type colonyService interface {
	ViewerByDiscordID(requestContext context.Context, discordUserID string) (core.Viewer, error)
	DiscordChannels(requestContext context.Context) (core.DiscordChannels, error)
}

var (
	_ discordClient = (*discordgo.Session)(nil)
	_ colonyService = (*core.Service)(nil)
	_ core.Notifier = (*Bot)(nil)
)

type Bot struct {
	options Options
	// client is nil when Beny Bot is off.
	client         discordClient
	commands       []command
	commandLimiter *userRateLimiter
	waitingPings   *slidingWindowLimiter

	// mutex guards service, running and stopping. MemberWaiting can be called from any request
	// goroutine, before Run starts or while it shuts down.
	mutex    sync.Mutex
	service  colonyService
	running  bool
	stopping bool
	posts    sync.WaitGroup
}

// New prepares Beny Bot without connecting. Without a bot token it returns a Bot that is off: Run only
// waits and MemberWaiting does nothing, which is the default for local development. The caller passes
// the Bot to core as its Notifier, then calls Run with the core service once both exist.
func New(options Options) (*Bot, error) {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Token == "" {
		return newBot(options, nil, time.Now), nil
	}
	if options.GuildID == "" {
		return nil, errors.New("bot: DISCORD_GUILD_ID is required when DISCORD_BOT_TOKEN is set")
	}
	session, err := discordgo.New("Bot " + options.Token)
	if err != nil {
		return nil, fmt.Errorf("bot: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildScheduledEvents
	return newBot(options, session, time.Now), nil
}

func newBot(options Options, client discordClient, now func() time.Time) *Bot {
	options.PublicURL = strings.TrimRight(options.PublicURL, "/")
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Bot{
		options:        options,
		client:         client,
		commands:       slashCommands(),
		commandLimiter: newUserRateLimiter(now),
		waitingPings:   newSlidingWindowLimiter(waitingPingLimit, waitingPingWindow, now),
	}
}

// Enabled reports whether Beny Bot has a bot token and will connect to Discord.
func (benyBot *Bot) Enabled() bool {
	return benyBot.client != nil
}

// Run connects to the Discord gateway and serves interactions until its context is cancelled. When
// Beny Bot is off it only waits, so the caller may run it unconditionally.
func (benyBot *Bot) Run(lifetimeContext context.Context, service *core.Service) error {
	if !benyBot.Enabled() {
		<-lifetimeContext.Done()
		return nil
	}
	if service == nil {
		return errors.New("bot: Run needs the core service")
	}
	return benyBot.run(lifetimeContext, service)
}

func (benyBot *Bot) run(lifetimeContext context.Context, service colonyService) error {
	if err := benyBot.start(service); err != nil {
		return err
	}
	removeReadyHandler := benyBot.client.AddHandler(func(_ *discordgo.Session, ready *discordgo.Ready) {
		benyBot.handleReady(lifetimeContext, ready)
	})
	defer removeReadyHandler()
	removeInteractionHandler := benyBot.client.AddHandler(func(_ *discordgo.Session, event *discordgo.InteractionCreate) {
		benyBot.handleInteraction(lifetimeContext, event)
	})
	defer removeInteractionHandler()

	if err := benyBot.client.Open(); err != nil {
		benyBot.stop()
		return fmt.Errorf("bot: connect to Discord: %w", err)
	}
	<-lifetimeContext.Done()
	benyBot.stop()
	if err := benyBot.client.Close(); err != nil {
		return fmt.Errorf("bot: disconnect from Discord: %w", err)
	}
	return nil
}

func (benyBot *Bot) start(service colonyService) error {
	benyBot.mutex.Lock()
	defer benyBot.mutex.Unlock()
	if benyBot.running {
		return errors.New("bot: Run was called twice")
	}
	benyBot.running = true
	benyBot.service = service
	return nil
}

// stop refuses new background posts, then waits for the ones already sent to Discord.
func (benyBot *Bot) stop() {
	benyBot.mutex.Lock()
	benyBot.stopping = true
	benyBot.mutex.Unlock()
	benyBot.posts.Wait()
}

func (benyBot *Bot) currentService() colonyService {
	benyBot.mutex.Lock()
	defer benyBot.mutex.Unlock()
	return benyBot.service
}

// handleReady runs on every fresh gateway session, so the commands are re-registered after each
// reconnect. Both overwrites are idempotent.
func (benyBot *Bot) handleReady(lifetimeContext context.Context, ready *discordgo.Ready) {
	applicationID := readyApplicationID(ready)
	if applicationID == "" {
		benyBot.options.Logger.Error("Discord's ready event names no application; Beny Bot's commands are not registered")
		return
	}
	benyBot.options.Logger.Info("Beny Bot is online", slog.String("guild_id", benyBot.options.GuildID))

	definitions := make([]*discordgo.ApplicationCommand, 0, len(benyBot.commands))
	for _, slashCommand := range benyBot.commands {
		definitions = append(definitions, slashCommand.definition)
	}
	if _, err := benyBot.client.ApplicationCommandBulkOverwrite(applicationID, benyBot.options.GuildID, definitions, discordgo.WithContext(lifetimeContext)); err != nil {
		benyBot.options.Logger.Error("Beny Bot couldn't register its commands in the guild", slog.Any("error", err))
	}
	// Global commands show up in every guild the application is in. Overwriting them with an empty list
	// (sent as [], not null) clears any that an older build left behind (spec §7.2).
	if _, err := benyBot.client.ApplicationCommandBulkOverwrite(applicationID, "", []*discordgo.ApplicationCommand{}, discordgo.WithContext(lifetimeContext)); err != nil {
		benyBot.options.Logger.Error("Beny Bot couldn't clear its global commands", slog.Any("error", err))
	}
}

func readyApplicationID(ready *discordgo.Ready) string {
	if ready == nil {
		return ""
	}
	if ready.Application != nil && ready.Application.ID != "" {
		return ready.Application.ID
	}
	// A current application's bot user has the application's ID.
	if ready.User != nil {
		return ready.User.ID
	}
	return ""
}

func (benyBot *Bot) handleInteraction(lifetimeContext context.Context, event *discordgo.InteractionCreate) {
	if event == nil || event.Interaction == nil {
		return
	}
	interaction := event.Interaction
	// Beny Bot serves one guild. Another server, or a direct message, is not Beny Bot's to answer.
	if interaction.GuildID != benyBot.options.GuildID {
		return
	}
	// Buttons (M3) and autocomplete (M2) are routed here once they exist.
	if interaction.Type != discordgo.InteractionApplicationCommand {
		return
	}
	data, isCommand := interaction.Data.(discordgo.ApplicationCommandInteractionData)
	if !isCommand {
		return
	}
	discordUserID := interactionUserID(interaction)
	if discordUserID == "" {
		return
	}
	logger := benyBot.options.Logger.With(slog.String("command", data.Name), slog.String("discord_user_id", discordUserID))

	interactionContext, cancel := context.WithTimeout(lifetimeContext, interactionDeadline)
	defer cancel()

	if !benyBot.commandLimiter.allow(discordUserID) {
		benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: slowDownMessage})
		return
	}
	service := benyBot.currentService()
	if service == nil {
		return
	}
	viewer, err := service.ViewerByDiscordID(interactionContext, discordUserID)
	if err != nil {
		if operationError, isOperationError := core.AsError(err); isOperationError && operationError.Kind == core.KindNotFound {
			benyBot.replyEphemeral(interactionContext, logger, interaction, benyBot.signInFirst())
			return
		}
		logger.Error("Beny Bot couldn't look up the caller", slog.Any("error", err))
		benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: failureMessage})
		return
	}
	if !viewer.IsActive() {
		benyBot.replyEphemeral(interactionContext, logger, interaction, benyBot.signInFirst())
		return
	}
	// Discord is not the place for DM detail: Beny Bot answers from the player projection, even when the DM
	// runs the command (spec §7.2), so core's player filters apply to every command.
	viewer.ViewAsPlayer = true

	slashCommand, found := benyBot.findCommand(data.Name)
	if !found {
		logger.Warn("Beny Bot received a command it doesn't register")
		benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: unknownCommandMessage})
		return
	}
	reply, err := slashCommand.handle(interactionContext, viewer, data)
	if err != nil {
		logger.Error("Beny Bot's command failed", slog.Any("error", err))
		benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: failureMessage})
		return
	}
	benyBot.replyEphemeral(interactionContext, logger, interaction, reply)
}

func interactionUserID(interaction *discordgo.Interaction) string {
	if interaction.Member != nil && interaction.Member.User != nil {
		return interaction.Member.User.ID
	}
	if interaction.User != nil {
		return interaction.User.ID
	}
	return ""
}

func (benyBot *Bot) signInFirst() *discordgo.InteractionResponseData {
	return &discordgo.InteractionResponseData{Content: fmt.Sprintf("Sign in at %s first", benyBot.options.PublicURL)}
}

// replyEphemeral is the only way Beny Bot answers an interaction. Every reply is visible to the caller
// alone and can't ping anyone, whoever runs the command (spec §7.2).
func (benyBot *Bot) replyEphemeral(interactionContext context.Context, logger *slog.Logger, interaction *discordgo.Interaction, data *discordgo.InteractionResponseData) {
	data.Flags |= discordgo.MessageFlagsEphemeral
	data.AllowedMentions = noMentions()
	err := benyBot.client.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	}, discordgo.WithContext(interactionContext))
	if err != nil {
		logger.Warn("Beny Bot couldn't reply", slog.Any("error", err))
	}
}

// noMentions lets a message name people without notifying them. Discord expects an empty list here,
// not null.
func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}
