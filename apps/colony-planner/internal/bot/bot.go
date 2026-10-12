// Package bot is Beny Bot, the Discord bot, running in-process over discordgo. It is off unless Beny Bot Dev's
// variables are set. One shared bot serves every campaign: each Discord server is linked to one campaign
// with /link, and every interaction is routed guild → linked campaign → membership
// (docs/design/multi-campaign.md §6). Every reply and post is built from the player projection, except
// the request prompts in the DM's own channel, which only the DM decides (spec §7.5).
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
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const (
	// Discord drops an interaction that gets no response within three seconds.
	interactionDeadline = 3 * time.Second
	// gatewayWorkTimeout bounds the database work a gateway event (Ready, GuildDelete, an acknowledged
	// button click) starts.
	gatewayWorkTimeout = 10 * time.Second
)

const (
	slowDownMessage       = "You're sending commands too quickly. Please slow down."
	unknownCommandMessage = "Beny Bot doesn't know that command."
	failureMessage        = "Something went wrong. Try again in a moment."
	useInServerMessage    = "Use Beny Bot in your campaign's server."
	manageServerMessage   = "Only someone with the Manage Server permission can do that."
)

type Options struct {
	Token string
	// LegacyGuildID is M1's DISCORD_GUILD_ID, read only for the one-time clean-up of the guild-scoped
	// commands M1 registered there; empty once the author has deleted it from .env.
	LegacyGuildID string
	// SiteOwnerDiscordID is the effective site owner, who gets a direct message when someone asks to run
	// a campaign. It decides nothing inside a campaign.
	SiteOwnerDiscordID string
	// PublicURL builds the links in Beny Bot's messages, e.g. <PublicURL>/c/<slug>/players.
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
	ChannelMessage(channelID string, messageID string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageEditComplex(edit *discordgo.MessageEdit, options ...discordgo.RequestOption) (*discordgo.Message, error)
	FollowupMessageCreate(interaction *discordgo.Interaction, wait bool, data *discordgo.WebhookParams, options ...discordgo.RequestOption) (*discordgo.Message, error)
	UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
}

// colonyService is what Beny Bot asks of core. *core.Service satisfies it; the bot's tests run it
// against a real core, because only core can build a Viewer.
type colonyService interface {
	ViewerForGuild(requestContext context.Context, guildID string, discordUserID string) (core.Viewer, error)
	DiscordTargetFor(requestContext context.Context, campaignID uuid.UUID) (core.DiscordTarget, bool, error)
	LinkGuild(requestContext context.Context, input core.LinkGuildInput) (core.LinkedGuild, error)
	UnlinkGuildByID(requestContext context.Context, guildID string) (bool, error)
	ReconcileGuilds(requestContext context.Context, presentGuildIDs []string) (int, error)
	SuggestRevealedNpcs(requestContext context.Context, viewer core.Viewer, prefix string, limit int) ([]core.Npc, error)
	FindRevealedNpc(requestContext context.Context, viewer core.Viewer, displayName string) (core.Npc, error)
	DecideRequestFromDiscord(requestContext context.Context, viewer core.Viewer, requestID uuid.UUID, grant bool) (core.RequestOutcome, error)
	GetRequest(requestContext context.Context, viewer core.Viewer, requestID uuid.UUID) (core.Request, error)
}

var (
	_ discordClient         = (*discordgo.Session)(nil)
	_ colonyService         = (*core.Service)(nil)
	_ core.Notifier         = (*Bot)(nil)
	_ core.RequestNotifier  = (*Bot)(nil)
	_ core.DiscordDirectory = (*Bot)(nil)
)

type Bot struct {
	options Options
	// client and state are nil when Beny Bot is off.
	client            discordClient
	state             gatewayState
	commands          []command
	commandLimiter    *userRateLimiter
	suggestionLimiter *userRateLimiter
	waitingPings      *slidingWindowLimiter
	ownerPings        *slidingWindowLimiter

	// mutex guards service, running and stopping. The notifier methods can be called from any request
	// goroutine, before Run starts or while it shuts down.
	mutex    sync.Mutex
	service  colonyService
	running  bool
	stopping bool
	posts    sync.WaitGroup
}

// New prepares Beny Bot without connecting. Without a bot token it returns a Bot that is off: Run only
// waits and the notifier methods do nothing, which is the default for local development. The caller
// passes the Bot to core as its Notifier, RequestNotifier and Directory, then calls Run with the core service.
func New(options Options) (*Bot, error) {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Token == "" {
		return newBot(options, nil, nil, time.Now), nil
	}
	session, err := discordgo.New("Bot " + options.Token)
	if err != nil {
		return nil, fmt.Errorf("bot: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildScheduledEvents
	return newBot(options, session, session.State, time.Now), nil
}

func newBot(options Options, client discordClient, state gatewayState, now func() time.Time) *Bot {
	options.PublicURL = strings.TrimRight(options.PublicURL, "/")
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Bot{
		options:           options,
		client:            client,
		state:             state,
		commands:          slashCommands(),
		commandLimiter:    newUserRateLimiter(now, commandBurst, commandRefillInterval),
		suggestionLimiter: newUserRateLimiter(now, suggestionBurst, suggestionRefillInterval),
		waitingPings:      newSlidingWindowLimiter(core.WaitingPingsPerHour, pingWindow, now),
		ownerPings:        newSlidingWindowLimiter(core.OwnerRequestPingsPerHour, pingWindow, now),
	}
}

// Enabled reports whether Beny Bot has a bot token and will connect to Discord.
func (benyBot *Bot) Enabled() bool {
	return benyBot.client != nil
}

// Directory is the channel directory core checks Settings against, or nil while Beny Bot is off, so
// core can tell "off" from "no channels".
func (benyBot *Bot) Directory() core.DiscordDirectory {
	if !benyBot.Enabled() {
		return nil
	}
	return benyBot
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
	removeGuildDeleteHandler := benyBot.client.AddHandler(func(_ *discordgo.Session, event *discordgo.GuildDelete) {
		benyBot.handleGuildDelete(lifetimeContext, event)
	})
	defer removeGuildDeleteHandler()

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

// handleReady runs on every fresh gateway session. It registers the commands globally (idempotent),
// clears M1's guild-scoped commands while LegacyGuildID is still set, and unlinks every linked server
// that removed Beny Bot while it was offline.
func (benyBot *Bot) handleReady(lifetimeContext context.Context, ready *discordgo.Ready) {
	if ready == nil {
		return
	}
	logger := benyBot.options.Logger
	logger.Info("Beny Bot is online", slog.Int("guilds", len(ready.Guilds)))
	benyBot.registerCommands(lifetimeContext, readyApplicationID(ready))
	benyBot.reconcileGuilds(lifetimeContext, ready.Guilds)
}

func (benyBot *Bot) registerCommands(lifetimeContext context.Context, applicationID string) {
	logger := benyBot.options.Logger
	if applicationID == "" {
		logger.Error("Discord's ready event names no application; Beny Bot's commands are not registered")
		return
	}
	definitions := make([]*discordgo.ApplicationCommand, 0, len(benyBot.commands))
	for _, slashCommand := range benyBot.commands {
		definitions = append(definitions, slashCommand.definition)
	}
	// Global commands reach every server the application is in; Discord may take a minute to show them.
	if _, err := benyBot.client.ApplicationCommandBulkOverwrite(applicationID, "", definitions, discordgo.WithContext(lifetimeContext)); err != nil {
		logger.Error("Beny Bot couldn't register its commands", slog.Any("error", err))
	}
	if benyBot.options.LegacyGuildID == "" {
		return
	}
	// M1 registered /beny in one guild. Left there, that server would show /beny twice. An empty list
	// (sent as [], not null) clears it.
	if _, err := benyBot.client.ApplicationCommandBulkOverwrite(applicationID, benyBot.options.LegacyGuildID, []*discordgo.ApplicationCommand{}, discordgo.WithContext(lifetimeContext)); err != nil {
		logger.Warn("Beny Bot couldn't clear the old guild commands; it tries again on its next start", slog.String("guild_id", benyBot.options.LegacyGuildID), slog.Any("error", err))
		return
	}
	logger.Info("old guild commands cleared; delete DISCORD_GUILD_ID from .env", slog.String("guild_id", benyBot.options.LegacyGuildID))
}

// reconcileGuilds unlinks every linked server missing from Ready's guild list: the bot was removed
// while it was offline, which is common locally.
func (benyBot *Bot) reconcileGuilds(lifetimeContext context.Context, guilds []*discordgo.Guild) {
	service := benyBot.currentService()
	if service == nil {
		return
	}
	presentGuildIDs := make([]string, 0, len(guilds))
	for _, guild := range guilds {
		if guild != nil && guild.ID != "" {
			presentGuildIDs = append(presentGuildIDs, guild.ID)
		}
	}
	workContext, cancel := context.WithTimeout(lifetimeContext, gatewayWorkTimeout)
	defer cancel()
	unlinked, err := service.ReconcileGuilds(workContext, presentGuildIDs)
	if err != nil {
		benyBot.options.Logger.Error("Beny Bot couldn't check its linked servers", slog.Int("unlinked", unlinked), slog.Any("error", err))
		return
	}
	if unlinked > 0 {
		benyBot.options.Logger.Info("unlinked servers that removed Beny Bot while it was offline", slog.Int("unlinked", unlinked))
	}
}

// handleGuildDelete unlinks a server that removed Beny Bot. Discord also sends GuildDelete with
// Unavailable set during an outage; that server still has the bot, so its link stays.
func (benyBot *Bot) handleGuildDelete(lifetimeContext context.Context, event *discordgo.GuildDelete) {
	if event == nil || event.Guild == nil || event.Unavailable || event.ID == "" {
		return
	}
	service := benyBot.currentService()
	if service == nil {
		return
	}
	workContext, cancel := context.WithTimeout(lifetimeContext, gatewayWorkTimeout)
	defer cancel()
	unlinked, err := service.UnlinkGuildByID(workContext, event.ID)
	if err != nil {
		benyBot.options.Logger.Error("Beny Bot couldn't unlink a server that removed it", slog.String("guild_id", event.ID), slog.Any("error", err))
		return
	}
	if unlinked {
		benyBot.options.Logger.Info("unlinked a server that removed Beny Bot", slog.String("guild_id", event.ID))
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
	if interaction.Type == discordgo.InteractionMessageComponent {
		benyBot.handleButton(lifetimeContext, interaction)
		return
	}
	isAutocomplete := interaction.Type == discordgo.InteractionApplicationCommandAutocomplete
	if interaction.Type != discordgo.InteractionApplicationCommand && !isAutocomplete {
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
	logger := benyBot.options.Logger.With(
		slog.String("command", data.Name),
		slog.String("guild_id", interaction.GuildID),
		slog.String("discord_user_id", discordUserID),
	)

	interactionContext, cancel := context.WithTimeout(lifetimeContext, interactionDeadline)
	defer cancel()
	if isAutocomplete {
		benyBot.respondWithChoices(interactionContext, logger, interaction, benyBot.suggestionsFor(interactionContext, logger, interaction, data, discordUserID))
		return
	}
	reply := func(content string) {
		benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: content})
	}

	if !benyBot.commandLimiter.allow(discordUserID) {
		reply(slowDownMessage)
		return
	}
	service := benyBot.currentService()
	if service == nil {
		return
	}
	// A direct message has no server, so no campaign to route to.
	if interaction.GuildID == "" {
		reply(useInServerMessage)
		return
	}
	slashCommand, found := benyBot.findCommand(data.Name)
	if found && slashCommand.handleServer != nil {
		benyBot.runServerCommand(interactionContext, logger, service, interaction, slashCommand, data, discordUserID)
		return
	}

	viewer, refusal := viewerFor(interactionContext, logger, service, interaction.GuildID, discordUserID)
	if refusal != "" {
		reply(refusal)
		return
	}
	if !found {
		logger.Warn("Beny Bot received a command it doesn't register")
		reply(unknownCommandMessage)
		return
	}
	response, err := slashCommand.handle(interactionContext, service, viewer, data)
	if err != nil {
		reply(refusalFor(logger, err, "Beny Bot's command failed"))
		return
	}
	benyBot.replyEphemeral(interactionContext, logger, interaction, response)
}

// viewerFor routes the server to its linked campaign and the caller to their membership. refusal is
// the line to answer with instead, or empty when the caller is let in.
func viewerFor(interactionContext context.Context, logger *slog.Logger, service colonyService, guildID string, discordUserID string) (viewer core.Viewer, refusal string) {
	viewer, err := service.ViewerForGuild(interactionContext, guildID, discordUserID)
	if err != nil {
		return core.Viewer{}, refusalFor(logger, err, "Beny Bot couldn't look up the caller")
	}
	// Discord is not the place for DM detail: Beny Bot answers from the player projection, even when the
	// DM runs the command (spec §7.2), so core's player filters apply to every command.
	return viewer.AsPlayer(), ""
}

// refusalFor is the line a failed lookup answers with: core's typed errors carry their own words (not
// linked, paused, not in this campaign, not let in, not found); anything else is logged and generic.
func refusalFor(logger *slog.Logger, err error, logMessage string) string {
	if operationError, isOperationError := core.AsError(err); isOperationError {
		return operationError.Message
	}
	logger.Error(logMessage, slog.Any("error", err))
	return failureMessage
}

// suggestionsFor answers an autocomplete request through the same routing as a command. Discord shows
// no message beside suggestions, so a refusal is the one choice offered: picking it runs the command,
// which answers with the whole line.
func (benyBot *Bot) suggestionsFor(interactionContext context.Context, logger *slog.Logger, interaction *discordgo.Interaction, data discordgo.ApplicationCommandInteractionData, discordUserID string) []*discordgo.ApplicationCommandOptionChoice {
	if !benyBot.suggestionLimiter.allow(discordUserID) {
		return refusalChoice(slowDownMessage)
	}
	service := benyBot.currentService()
	if service == nil {
		return refusalChoice(failureMessage)
	}
	if interaction.GuildID == "" {
		return refusalChoice(useInServerMessage)
	}
	viewer, refusal := viewerFor(interactionContext, logger, service, interaction.GuildID, discordUserID)
	if refusal != "" {
		return refusalChoice(refusal)
	}
	slashCommand, found := benyBot.findCommand(data.Name)
	if !found || slashCommand.suggest == nil {
		logger.Warn("Beny Bot received autocomplete for a command that offers none")
		return refusalChoice(unknownCommandMessage)
	}
	choices, err := slashCommand.suggest(interactionContext, service, viewer, data)
	if err != nil {
		return refusalChoice(refusalFor(logger, err, "Beny Bot's suggestions failed"))
	}
	return choices
}

// refusalChoice offers a line as the only choice, cut to Discord's 100-character limit.
func refusalChoice(line string) []*discordgo.ApplicationCommandOptionChoice {
	text := fitText(line, choiceMaxLength, keepText)
	return []*discordgo.ApplicationCommandOptionChoice{{Name: text, Value: text}}
}

// respondWithChoices answers an autocomplete request. Discord shows suggestions to the typist alone,
// so they need no ephemeral flag.
func (benyBot *Bot) respondWithChoices(interactionContext context.Context, logger *slog.Logger, interaction *discordgo.Interaction, choices []*discordgo.ApplicationCommandOptionChoice) {
	err := benyBot.client.InteractionRespond(interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	}, discordgo.WithContext(interactionContext))
	if err != nil {
		logger.Warn("Beny Bot couldn't suggest", slog.Any("error", err))
	}
}

// runServerCommand runs /link and /unlink: they act for the server, not for a campaign member, so they
// need Manage Server (or Administrator) instead of a membership. Discord hides them from other members
// through DefaultMemberPermissions, but a server's admins can override that, so it is checked again.
func (benyBot *Bot) runServerCommand(interactionContext context.Context, logger *slog.Logger, service colonyService, interaction *discordgo.Interaction, slashCommand command, data discordgo.ApplicationCommandInteractionData, discordUserID string) {
	if interaction.Member == nil || interaction.Member.Permissions&serverManagerPermissions == 0 {
		benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: manageServerMessage})
		return
	}
	response, err := slashCommand.handleServer(interactionContext, service, serverInvocation{
		guildID:       interaction.GuildID,
		guildName:     benyBot.guildName(interaction.GuildID),
		discordUserID: discordUserID,
		data:          data,
	})
	if err != nil {
		if operationError, isOperationError := core.AsError(err); isOperationError {
			logger.Info("Beny Bot refused a server command", slog.Int("kind", int(operationError.Kind)))
			benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: operationError.Message})
			return
		}
		logger.Error("Beny Bot's server command failed", slog.Any("error", err))
		benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: failureMessage})
		return
	}
	benyBot.replyEphemeral(interactionContext, logger, interaction, response)
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
