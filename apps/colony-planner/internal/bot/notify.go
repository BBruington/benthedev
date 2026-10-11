package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

// At most five waiting-member pings an hour (spec §2.1). Requests past the limit still show on /players.
const (
	waitingPingLimit  = 5
	waitingPingWindow = time.Hour
	// postTimeout bounds a background post, which also bounds how long shutdown waits for one.
	postTimeout = 5 * time.Second
)

// MemberWaiting implements core.Notifier. It returns at once: the post happens in the background,
// and a failure is only logged, because Discord posts are best-effort (spec §7.2).
func (benyBot *Bot) MemberWaiting(requestContext context.Context, member core.WaitingMember) {
	if !benyBot.Enabled() {
		return
	}
	benyBot.mutex.Lock()
	defer benyBot.mutex.Unlock()
	if benyBot.service == nil || benyBot.stopping {
		benyBot.options.Logger.Warn("Beny Bot isn't running; skipped the waiting-member ping", slog.String("member_id", member.MemberID.String()))
		return
	}
	service := benyBot.service
	// The sign-in request that called this usually ends before Discord answers.
	detachedContext := context.WithoutCancel(requestContext)
	benyBot.posts.Go(func() {
		postContext, cancel := context.WithTimeout(detachedContext, postTimeout)
		defer cancel()
		benyBot.postMemberWaiting(postContext, service, member)
	})
}

func (benyBot *Bot) postMemberWaiting(requestContext context.Context, service colonyService, member core.WaitingMember) {
	logger := benyBot.options.Logger.With(slog.String("member_id", member.MemberID.String()))
	channels, err := service.DiscordChannels(requestContext)
	if err != nil {
		logger.Error("Beny Bot couldn't read the DM channel from settings", slog.Any("error", err))
		return
	}
	if channels.DmChannelID == nil || *channels.DmChannelID == "" {
		logger.Info("no DM channel is set on /settings; skipped the waiting-member ping")
		return
	}
	if !benyBot.waitingPings.allow() {
		logger.Info("five waiting-member pings were sent in the last hour; skipped this one")
		return
	}
	_, err = benyBot.client.ChannelMessageSendComplex(*channels.DmChannelID, &discordgo.MessageSend{
		Content:         waitingMessage(member, benyBot.options.PublicURL),
		AllowedMentions: noMentions(),
		// The name is chosen by whoever signs in; no preview card may hang off Beny Bot's message.
		Flags: discordgo.MessageFlagsSuppressEmbeds,
	}, discordgo.WithContext(requestContext))
	if err != nil {
		logger.Warn("Beny Bot couldn't post the waiting-member ping", slog.Any("error", err))
	}
}

func waitingMessage(member core.WaitingMember, publicURL string) string {
	displayName := member.DisplayName
	if displayName == "" {
		displayName = member.DiscordUsername
	}
	return fmt.Sprintf("%s (@%s) is waiting to join: %s/players",
		escapeMarkdown(displayName), escapeMarkdown(member.DiscordUsername), publicURL)
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

// slidingWindowLimiter allows at most limit events in any window-long span.
type slidingWindowLimiter struct {
	mutex  sync.Mutex
	now    func() time.Time
	limit  int
	window time.Duration
	sent   []time.Time
}

func newSlidingWindowLimiter(limit int, window time.Duration, now func() time.Time) *slidingWindowLimiter {
	return &slidingWindowLimiter{now: now, limit: limit, window: window}
}

func (limiter *slidingWindowLimiter) allow() bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	current := limiter.now()
	recent := limiter.sent[:0]
	for _, sentAt := range limiter.sent {
		if current.Sub(sentAt) < limiter.window {
			recent = append(recent, sentAt)
		}
	}
	limiter.sent = recent
	if len(limiter.sent) >= limiter.limit {
		return false
	}
	limiter.sent = append(limiter.sent, current)
	return true
}
