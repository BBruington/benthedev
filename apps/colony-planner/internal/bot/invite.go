package bot

import (
	"net/url"
	"strconv"

	"github.com/bwmarrin/discordgo"
)

// invitePermissions is what Beny Bot asks for when a DM invites it (docs/design/multi-campaign.md §6.1):
// View Channel, Send Messages, Embed Links, Manage Events and Send Polls. No feature reads messages, so
// it doesn't ask for Read Message History: the site owner runs Beny Bot and must not be able to read a
// campaign's server through it.
const invitePermissions = discordgo.PermissionViewChannel |
	discordgo.PermissionSendMessages |
	discordgo.PermissionEmbedLinks |
	discordgo.PermissionManageEvents |
	discordgo.PermissionSendPolls

// InviteURL is the address that adds Beny Bot to a Discord server, for Settings. It is empty without a
// client ID. The client ID is public; nothing secret goes in the link.
func InviteURL(clientID string) string {
	if clientID == "" {
		return ""
	}
	// Built by hand so it reads exactly like the link in docs/discord-dev-setup.md.
	return "https://discord.com/oauth2/authorize?client_id=" + url.QueryEscape(clientID) +
		"&scope=bot+applications.commands&permissions=" + strconv.FormatInt(invitePermissions, 10)
}
