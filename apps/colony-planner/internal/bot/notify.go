package bot

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const (
	// pingWindow is the span of core.WaitingPingsPerHour and core.OwnerRequestPingsPerHour. Pings past
	// a limit are skipped; the request still shows on /players or /admin.
	pingWindow = time.Hour
	// postTimeout bounds a background post, which also bounds how long shutdown waits for one.
	postTimeout = 5 * time.Second
	// ownerPingKey is the owner limiter's one key: all owner pings share one allowance.
	ownerPingKey = "owner"
)

// discordUserIDPattern tells a real Discord user ID from the devauth owner seed:dm, who has no
// Discord account to message.
var discordUserIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)

// channelKind names one of a campaign's channels on Settings.
type channelKind string

const (
	announceChannel channelKind = "announce"
	reminderChannel channelKind = "reminder"
	dmChannel       channelKind = "dm"
	// sessionNoticeChannel is the reminder channel, or the announce channel while that is unset (spec
	// §7.2: the cancellation notice falls back this way, and reminders follow it).
	sessionNoticeChannel channelKind = "session_notice"
)

func (kind channelKind) in(target core.DiscordTarget) string {
	var channelID *string
	switch kind {
	case announceChannel:
		channelID = target.AnnounceChannelID
	case reminderChannel:
		channelID = target.ReminderChannelID
	case dmChannel:
		channelID = target.DmChannelID
	case sessionNoticeChannel:
		channelID = target.ReminderChannelID
		if channelID == nil || *channelID == "" {
			channelID = target.AnnounceChannelID
		}
	}
	if channelID == nil {
		return ""
	}
	return *channelID
}

// MemberWaiting implements core.Notifier: "<name> is waiting to join" in that campaign's DM channel. It
// returns at once: the post happens in the background, and a failure is only logged, because Discord
// posts are best-effort (spec §7.2).
func (benyBot *Bot) MemberWaiting(requestContext context.Context, member core.WaitingMember) {
	content := waitingMessage(member, benyBot.options.PublicURL)
	campaignKey := member.CampaignID.String()
	benyBot.inBackground(requestContext, "waiting-member ping", func(postContext context.Context, service colonyService) {
		benyBot.postToCampaign(postContext, service, member.CampaignID, dmChannel, &discordgo.MessageSend{Content: content}, func() bool {
			// One busy campaign never silences another.
			return benyBot.waitingPings.allow(campaignKey)
		})
	})
}

// CampaignRequested implements core.Notifier: a direct message to the site owner linking to /admin.
func (benyBot *Bot) CampaignRequested(requestContext context.Context, request core.RequestedCampaign) {
	if !benyBot.Enabled() {
		return
	}
	ownerDiscordID := benyBot.options.SiteOwnerDiscordID
	if !discordUserIDPattern.MatchString(ownerDiscordID) {
		benyBot.options.Logger.Info("the site owner has no Discord ID; skipped the campaign-request ping", slog.String("campaign_request_id", request.CampaignRequestID.String()))
		return
	}
	content := campaignRequestMessage(request, benyBot.options.PublicURL)
	benyBot.inBackground(requestContext, "campaign-request ping", func(postContext context.Context, _ colonyService) {
		logger := benyBot.options.Logger.With(slog.String("campaign_request_id", request.CampaignRequestID.String()))
		if !benyBot.ownerPings.allow(ownerPingKey) {
			logger.Info("the hourly limit of campaign-request pings was reached; skipped this one")
			return
		}
		channel, err := benyBot.client.UserChannelCreate(ownerDiscordID, discordgo.WithContext(postContext))
		if err != nil || channel == nil {
			logger.Warn("Beny Bot couldn't open a direct message to the site owner", slog.Any("error", err))
			return
		}
		benyBot.send(postContext, logger, channel.ID, &discordgo.MessageSend{Content: content})
	})
}

// inBackground runs a post after the request that caused it, which usually ends before Discord
// answers. It does nothing while Beny Bot is off, not yet running, or shutting down.
func (benyBot *Bot) inBackground(requestContext context.Context, what string, post func(postContext context.Context, service colonyService)) {
	if !benyBot.Enabled() {
		return
	}
	benyBot.mutex.Lock()
	defer benyBot.mutex.Unlock()
	if benyBot.service == nil || benyBot.stopping {
		benyBot.options.Logger.Warn("Beny Bot isn't running; skipped a post", slog.String("post", what))
		return
	}
	service := benyBot.service
	detachedContext := context.WithoutCancel(requestContext)
	benyBot.posts.Go(func() {
		postContext, cancel := context.WithTimeout(detachedContext, postTimeout)
		defer cancel()
		post(postContext, service)
	})
}

// postToCampaign is the one way Beny Bot posts for a campaign: pings, request prompts, reminders and
// event notices. It posts only to a channel campaignChannel vouches for. withinLimit runs last, so a
// skipped post never uses up an hourly allowance. It returns the posted message, or nil when it
// skipped the post or the post failed; failed (Discord refused the send, or the campaign's server
// couldn't be read) tells the two apart, so a reminder is retried only after a post that failed.
func (benyBot *Bot) postToCampaign(postContext context.Context, service colonyService, campaignID uuid.UUID, kind channelKind, message *discordgo.MessageSend, withinLimit func() bool) (posted *discordgo.Message, failed bool) {
	logger := benyBot.options.Logger.With(slog.String("campaign_id", campaignID.String()), slog.String("channel_kind", string(kind)))
	channelID, found, err := benyBot.campaignChannel(postContext, logger, service, campaignID, kind)
	if err != nil {
		return nil, true
	}
	if !found {
		return nil, false
	}
	if withinLimit != nil && !withinLimit() {
		logger.Info("the hourly limit of posts was reached; skipped this one")
		return nil, false
	}
	return benyBot.send(postContext, logger.With(slog.String("channel_id", channelID)), channelID, message)
}

// campaignChannel reads the campaign's linked server and channel, and finds it only when the campaign
// is ACTIVE and linked, the channel is set, and the gateway knows that channel as a text channel of the
// linked server, so a DM can never make Beny Bot post or edit in someone else's server.
func (benyBot *Bot) campaignChannel(postContext context.Context, logger *slog.Logger, service colonyService, campaignID uuid.UUID, kind channelKind) (string, bool, error) {
	target, found, err := service.DiscordTargetFor(postContext, campaignID)
	if err != nil {
		logger.Error("Beny Bot couldn't read the campaign's Discord server", slog.Any("error", err))
		return "", false, err
	}
	if !found {
		logger.Info("the campaign has no linked server or is paused; skipped a post")
		return "", false, nil
	}
	channelID := kind.in(target)
	if channelID == "" {
		logger.Info("the campaign's channel isn't set; skipped a post")
		return "", false, nil
	}
	if !benyBot.channelInGuild(channelID, target.GuildID) {
		logger.Warn("the channel isn't a text channel of the campaign's linked server; skipped a post", slog.String("channel_id", channelID), slog.String("guild_id", target.GuildID))
		return "", false, nil
	}
	return channelID, true, nil
}

func (benyBot *Bot) send(postContext context.Context, logger *slog.Logger, channelID string, message *discordgo.MessageSend) (posted *discordgo.Message, discordRefused bool) {
	message.AllowedMentions = noMentions()
	// Names are chosen by whoever signs in; no preview card may hang off Beny Bot's message.
	message.Flags |= discordgo.MessageFlagsSuppressEmbeds
	posted, err := benyBot.client.ChannelMessageSendComplex(channelID, message, discordgo.WithContext(postContext))
	if err != nil {
		logger.Warn("Beny Bot couldn't post", slog.Any("error", err))
		return nil, true
	}
	return posted, false
}

// waitingMessage links to the campaign's Players page. The slug is a-z, 0-9 and dashes (core checks it),
// so it goes in the link unescaped.
func waitingMessage(member core.WaitingMember, publicURL string) string {
	return fmt.Sprintf("%s (@%s) is waiting to join: %s/c/%s/players",
		escapeMarkdown(displayNameOf(member.DisplayName, member.DiscordUsername)), escapeMarkdown(member.DiscordUsername), publicURL, member.CampaignSlug)
}

func campaignRequestMessage(request core.RequestedCampaign, publicURL string) string {
	return fmt.Sprintf("%s (@%s) asks to run %s: %s/admin",
		escapeMarkdown(displayNameOf(request.DisplayName, request.DiscordUsername)), escapeMarkdown(request.DiscordUsername),
		escapeMarkdown(request.CampaignName), publicURL)
}

func displayNameOf(displayName string, discordUsername string) string {
	if displayName == "" {
		return discordUsername
	}
	return displayName
}

// Names come from whoever signs in, so they are shown literally: no bold, headings, quotes, masked
// links, mention syntax, emoji codes or clickable links (an escaped colon stops "https://" from
// linking). Discord renders a backslash before any punctuation as that character.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`, `*`, `\*`, `_`, `\_`, `~`, `\~`, "`", "\\`", `|`, `\|`, `>`, `\>`, `<`, `\<`,
	`[`, `\[`, `]`, `\]`, `(`, `\(`, `)`, `\)`, `#`, `\#`, `-`, `\-`, `:`, `\:`, "\r", " ", "\n", " ",
)

func escapeMarkdown(text string) string {
	return markdownEscaper.Replace(text)
}

// slidingWindowLimiter allows at most limit events per key in any window-long span: per campaign for
// waiting pings, one shared key for the owner's pings.
type slidingWindowLimiter struct {
	mutex  sync.Mutex
	now    func() time.Time
	limit  int
	window time.Duration
	sent   map[string][]time.Time
}

func newSlidingWindowLimiter(limit int, window time.Duration, now func() time.Time) *slidingWindowLimiter {
	return &slidingWindowLimiter{now: now, limit: limit, window: window, sent: map[string][]time.Time{}}
}

func (limiter *slidingWindowLimiter) allow(key string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	current := limiter.now()
	// Forgetting every key whose window has passed keeps memory bounded by recent activity.
	for sentKey, sentTimes := range limiter.sent {
		recent := sentTimes[:0]
		for _, sentAt := range sentTimes {
			if current.Sub(sentAt) < limiter.window {
				recent = append(recent, sentAt)
			}
		}
		if len(recent) == 0 {
			delete(limiter.sent, sentKey)
			continue
		}
		limiter.sent[sentKey] = recent
	}
	if len(limiter.sent[key]) >= limiter.limit {
		return false
	}
	limiter.sent[key] = append(limiter.sent[key], current)
	return true
}
