package bot

import (
	"net/url"
	"strconv"

	"github.com/bwmarrin/discordgo"
)

// invitePermissions is what Beny Bot asks for when invited (spec §7.1): View Channel, Send Messages, Embed
// Links, Read Message History and Manage Events.
const invitePermissions = discordgo.PermissionViewChannel |
	discordgo.PermissionSendMessages |
	discordgo.PermissionEmbedLinks |
	discordgo.PermissionReadMessageHistory |
	discordgo.PermissionManageEvents

// InviteURL is the address that adds Beny Bot to a Discord server, for /settings. It is empty without a
// client ID. The client ID is public; nothing secret goes in the link.
func InviteURL(clientID string) string {
	if clientID == "" {
		return ""
	}
	// Built by hand so it reads exactly like the link in docs/discord-dev-setup.md.
	return "https://discord.com/oauth2/authorize?client_id=" + url.QueryEscape(clientID) +
		"&scope=bot+applications.commands&permissions=" + strconv.FormatInt(invitePermissions, 10)
}
