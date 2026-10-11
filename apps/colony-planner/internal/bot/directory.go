package bot

import (
	"cmp"
	"context"
	"slices"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

// gatewayState is the part of discordgo's State (the gateway's cache of the servers and channels Beny
// Bot is in) that Beny Bot reads. Guild and Channel return pointers into the cache, which the gateway
// goroutine rewrites in place, so their fields are read under RLock; Guild and Channel take that lock
// themselves, so they are never called while holding it.
type gatewayState interface {
	Guild(guildID string) (*discordgo.Guild, error)
	Channel(channelID string) (*discordgo.Channel, error)
	RLock()
	RUnlock()
}

var _ gatewayState = (*discordgo.State)(nil)

// textChannelTypes are the channels Beny Bot can post plain messages to.
var textChannelTypes = []discordgo.ChannelType{discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews}

// TextChannels implements core.DiscordDirectory for the Settings channel picker: the linked server's
// text channels, in Discord's order. A server Beny Bot isn't in (or hasn't heard about yet) has none.
func (benyBot *Bot) TextChannels(_ context.Context, guildID string) ([]core.DiscordChannel, error) {
	if benyBot.state == nil || guildID == "" {
		return []core.DiscordChannel{}, nil
	}
	guild, err := benyBot.state.Guild(guildID)
	if err != nil {
		return []core.DiscordChannel{}, nil
	}
	type positionedChannel struct {
		channel  core.DiscordChannel
		position int
	}
	benyBot.state.RLock()
	positioned := make([]positionedChannel, 0, len(guild.Channels))
	for _, channel := range guild.Channels {
		if channel != nil && slices.Contains(textChannelTypes, channel.Type) {
			positioned = append(positioned, positionedChannel{
				channel:  core.DiscordChannel{ID: channel.ID, Name: channel.Name},
				position: channel.Position,
			})
		}
	}
	benyBot.state.RUnlock()

	slices.SortStableFunc(positioned, func(first positionedChannel, second positionedChannel) int {
		return cmp.Or(cmp.Compare(first.position, second.position), cmp.Compare(first.channel.ID, second.channel.ID))
	})
	channels := make([]core.DiscordChannel, 0, len(positioned))
	for _, entry := range positioned {
		channels = append(channels, entry.channel)
	}
	return channels, nil
}

// channelInGuild reports whether the gateway knows channelID as a text channel of guildID. postToCampaign
// checks it before every post, so a stored channel ID can never make Beny Bot post into another server.
func (benyBot *Bot) channelInGuild(channelID string, guildID string) bool {
	if benyBot.state == nil || channelID == "" || guildID == "" {
		return false
	}
	channel, err := benyBot.state.Channel(channelID)
	if err != nil || channel == nil {
		return false
	}
	benyBot.state.RLock()
	defer benyBot.state.RUnlock()
	return channel.GuildID == guildID && slices.Contains(textChannelTypes, channel.Type)
}

// guildName is the gateway's name for a server, for the DM's Settings card after /link.
func (benyBot *Bot) guildName(guildID string) string {
	if benyBot.state == nil {
		return ""
	}
	guild, err := benyBot.state.Guild(guildID)
	if err != nil || guild == nil {
		return ""
	}
	benyBot.state.RLock()
	defer benyBot.state.RUnlock()
	return guild.Name
}
