package bot

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const (
	requestButtonPrefix  = "req"
	grantButtonChoice    = "grant"
	declineButtonChoice  = "decline"
	openInAppSuffix      = " Open in app."
	openInAppLabel       = "Open in app"
	onlyDmDecidesMessage = "Only the DM can decide."
	unknownButtonMessage = "Beny Bot doesn't know that button."
)

// RequestCreated implements core.RequestNotifier: the request's prompt with Grant and Decline buttons in
// the campaign's DM channel (spec §7.5). It returns at once; the post and storing its message ID happen
// in the background, and a failure is only logged.
func (benyBot *Bot) RequestCreated(requestContext context.Context, prompt core.RequestPrompt) {
	benyBot.inBackground(requestContext, "request prompt", func(postContext context.Context, service colonyService) {
		posted := benyBot.postToCampaign(postContext, service, prompt.CampaignID, dmChannel, &discordgo.MessageSend{
			Content:    requestPromptText(prompt),
			Components: benyBot.requestButtons(postContext, service, prompt.CampaignID, prompt.RequestID),
		}, nil)
		if posted == nil {
			return
		}
		logger := benyBot.options.Logger.With(slog.String("campaign_id", prompt.CampaignID.String()), slog.String("request_id", prompt.RequestID.String()))
		decided, err := prompt.RecordMessage(postContext, posted.ID)
		if err != nil {
			logger.Error("Beny Bot couldn't store a request prompt's message; later decisions won't edit it", slog.Any("error", err))
			return
		}
		// Decided while this was being posted: no decision could edit it, so its buttons go now.
		if decided != nil {
			benyBot.editPrompt(postContext, logger, posted, decided.Text, nil)
		}
	})
}

// requestPromptText words a prompt as spec §7.5 does. Character names are the players' own text, so
// every part is escaped and can't turn into Discord formatting.
func requestPromptText(prompt core.RequestPrompt) string {
	return escapeMarkdown(prompt.RequesterName) + " requests **" + escapeMarkdown(prompt.Title) + "** from " +
		escapeMarkdown(prompt.PatronName) + " (" + escapeMarkdown(prompt.Terms) + "). Favor now " + escapeMarkdown(prompt.FavorNow) + "."
}

// RequestDecided implements core.RequestNotifier: it edits the stored prompt to show the outcome,
// whether the decision was made on the website or in Discord, and brings the buttons back when an undo
// leaves the request waiting again.
func (benyBot *Bot) RequestDecided(requestContext context.Context, outcome core.RequestOutcome) {
	if outcome.DiscordMessageID == "" {
		return
	}
	benyBot.inBackground(requestContext, "request outcome", func(postContext context.Context, service colonyService) {
		logger := benyBot.options.Logger.With(
			slog.String("campaign_id", outcome.CampaignID.String()),
			slog.String("request_id", outcome.RequestID.String()),
			slog.String("status", string(outcome.Status)),
		)
		channelID, found := benyBot.campaignChannel(postContext, logger, service, outcome.CampaignID, dmChannel)
		if !found {
			return
		}
		prompt, err := benyBot.client.ChannelMessage(channelID, outcome.DiscordMessageID, discordgo.WithContext(postContext))
		if err != nil || prompt == nil {
			// Reading a message needs Read Message History; without it the outcome alone still replaces
			// buttons that no longer decide anything.
			logger.Warn("Beny Bot couldn't read a request prompt; it shows the outcome alone", slog.String("channel_id", channelID), slog.Any("error", err))
			prompt = &discordgo.Message{ID: outcome.DiscordMessageID, ChannelID: channelID}
		}
		var buttons []discordgo.MessageComponent
		if outcome.Undone {
			buttons = benyBot.requestButtons(postContext, service, outcome.CampaignID, outcome.RequestID)
		}
		benyBot.editPrompt(postContext, logger, prompt, outcome.Text, buttons)
	})
}

// handleButton decides a request from its prompt's Grant or Decline button. The click is routed like a
// command, through the server's linked campaign and the clicker's membership, and only that campaign's
// DM may decide; core loads the request with that campaign, so a button can't reach another one's.
func (benyBot *Bot) handleButton(lifetimeContext context.Context, interaction *discordgo.Interaction) {
	data, isComponent := interaction.Data.(discordgo.MessageComponentInteractionData)
	discordUserID := interactionUserID(interaction)
	if !isComponent || discordUserID == "" {
		return
	}
	logger := benyBot.options.Logger.With(slog.String("guild_id", interaction.GuildID), slog.String("discord_user_id", discordUserID))
	interactionContext, cancel := context.WithTimeout(lifetimeContext, interactionDeadline)
	defer cancel()
	reply := func(content string) {
		benyBot.replyEphemeral(interactionContext, logger, interaction, &discordgo.InteractionResponseData{Content: content})
	}

	requestID, grant, isRequestButton := parseRequestButton(data.CustomID)
	if !isRequestButton {
		logger.Warn("Beny Bot received a click on a button it doesn't make")
		reply(unknownButtonMessage)
		return
	}
	logger = logger.With(slog.String("request_id", requestID.String()), slog.Bool("grant", grant))
	if !benyBot.commandLimiter.allow(discordUserID) {
		reply(slowDownMessage)
		return
	}
	service := benyBot.currentService()
	if service == nil {
		return
	}
	if interaction.GuildID == "" {
		reply(useInServerMessage)
		return
	}
	// Unlike a command, deciding needs the DM's own rights, so this viewer is not turned into a player's.
	viewer, err := service.ViewerForGuild(interactionContext, interaction.GuildID, discordUserID)
	if err != nil {
		reply(refusalFor(logger, err, "Beny Bot couldn't look up the clicker"))
		return
	}
	if !viewer.IsDM() {
		reply(onlyDmDecidesMessage)
		return
	}
	err = benyBot.client.InteractionRespond(interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate}, discordgo.WithContext(interactionContext))
	if err != nil {
		// Discord takes one answer per click: another process already took this one, or the DM saw the
		// click fail. Either way deciding here would act on a click this process doesn't own.
		logger.Warn("Beny Bot couldn't acknowledge a request button; it decided nothing", slog.Any("error", err))
		return
	}

	// The acknowledgement buys Discord's fifteen minutes, so the decision gets its own deadline.
	decisionContext, cancelDecision := context.WithTimeout(lifetimeContext, gatewayWorkTimeout)
	defer cancelDecision()
	outcome, err := service.DecideRequestFromDiscord(decisionContext, viewer, requestID, grant)
	if err == nil {
		benyBot.editPrompt(decisionContext, logger, interaction.Message, outcome.Text, nil)
		return
	}
	followUp := func(content string) {
		benyBot.followUpEphemeral(decisionContext, logger, interaction, content)
	}
	operationError, isOperationError := core.AsError(err)
	if !isOperationError {
		logger.Error("Beny Bot couldn't decide a request", slog.Any("error", err))
		followUp(failureMessage)
		return
	}
	if operationError.Kind != core.KindConflict {
		followUp(operationError.Message)
		return
	}
	request, err := service.GetRequest(decisionContext, viewer, requestID)
	if err != nil {
		followUp(refusalFor(logger, err, "Beny Bot couldn't read a request it couldn't decide"))
		return
	}
	if request.Status != core.RequestStatusPending {
		// A second click, or a decision on the website, got there first and edits the prompt itself.
		logger.Info("a request button was clicked after its request was decided")
		followUp(operationError.Message)
		return
	}
	// The request still waits (a shortfall), so the DM can top up and click again.
	buttons := benyBot.requestButtons(decisionContext, service, viewer.CampaignID(), requestID)
	benyBot.editPrompt(decisionContext, logger, interaction.Message, operationError.Message+openInAppSuffix, buttons)
}

// editPrompt keeps the prompt's first line, which says what was asked, and puts the outcome under it,
// replacing any earlier outcome line. No buttons means the request no longer waits.
func (benyBot *Bot) editPrompt(editContext context.Context, logger *slog.Logger, prompt *discordgo.Message, outcomeLine string, buttons []discordgo.MessageComponent) {
	if prompt == nil {
		logger.Warn("Beny Bot has no request prompt to edit")
		return
	}
	content := outcomeLine
	if request, _, _ := strings.Cut(prompt.Content, "\n"); request != "" {
		content = request + "\n" + outcomeLine
	}
	components := []discordgo.MessageComponent{}
	if buttons != nil {
		components = buttons
	}
	_, err := benyBot.client.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:              prompt.ID,
		Channel:         prompt.ChannelID,
		Content:         &content,
		Components:      &components,
		AllowedMentions: noMentions(),
		Flags:           discordgo.MessageFlagsSuppressEmbeds,
	}, discordgo.WithContext(editContext))
	if err != nil {
		logger.Warn("Beny Bot couldn't edit a request prompt", slog.String("channel_id", prompt.ChannelID), slog.Any("error", err))
	}
}

// followUpEphemeral answers a click Beny Bot already acknowledged, visible to the clicker alone.
func (benyBot *Bot) followUpEphemeral(followUpContext context.Context, logger *slog.Logger, interaction *discordgo.Interaction, content string) {
	_, err := benyBot.client.FollowupMessageCreate(interaction, false, &discordgo.WebhookParams{
		Content:         content,
		Flags:           discordgo.MessageFlagsEphemeral,
		AllowedMentions: noMentions(),
	}, discordgo.WithContext(followUpContext))
	if err != nil {
		logger.Warn("Beny Bot couldn't follow up on a click", slog.Any("error", err))
	}
}

// requestButtons are a waiting request's Grant and Decline, plus a link to its approval sheet on the
// website (spec §6.5), where the DM can adjust favor or tweak the grant.
func (benyBot *Bot) requestButtons(lookupContext context.Context, service colonyService, campaignID uuid.UUID, requestID uuid.UUID) []discordgo.MessageComponent {
	buttons := []discordgo.MessageComponent{
		discordgo.Button{Label: "Grant", Style: discordgo.SuccessButton, CustomID: requestButtonID(requestID, grantButtonChoice)},
		discordgo.Button{Label: "Decline", Style: discordgo.DangerButton, CustomID: requestButtonID(requestID, declineButtonChoice)},
	}
	// Without a linked server nothing is posted or edited, so a failed lookup only drops the link.
	target, found, err := service.DiscordTargetFor(lookupContext, campaignID)
	if err == nil && found && target.CampaignSlug != "" && benyBot.options.PublicURL != "" {
		reviewURL := benyBot.options.PublicURL + "/c/" + url.PathEscape(target.CampaignSlug) + "/requests?review=" + requestID.String()
		buttons = append(buttons, discordgo.Button{Label: openInAppLabel, Style: discordgo.LinkButton, URL: reviewURL})
	}
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}}
}

func requestButtonID(requestID uuid.UUID, choice string) string {
	return requestButtonPrefix + ":" + requestID.String() + ":" + choice
}

// parseRequestButton reads req:{requestId}:grant or req:{requestId}:decline.
func parseRequestButton(customID string) (requestID uuid.UUID, grant bool, isRequestButton bool) {
	parts := strings.Split(customID, ":")
	if len(parts) != 3 || parts[0] != requestButtonPrefix {
		return uuid.UUID{}, false, false
	}
	requestID, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.UUID{}, false, false
	}
	switch parts[2] {
	case grantButtonChoice:
		return requestID, true, true
	case declineButtonChoice:
		return requestID, false, true
	default:
		return uuid.UUID{}, false, false
	}
}
