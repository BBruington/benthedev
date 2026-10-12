package bot

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

// serverManagerPermissions may run /link and /unlink: Manage Server, or Administrator, which implies it.
const serverManagerPermissions = discordgo.PermissionManageGuild | discordgo.PermissionAdministrator

// linkCodeOptionMaxLength leaves room for the code typed with spaces or without its BENY- prefix.
const linkCodeOptionMaxLength = 32

// command is one slash command, with exactly one handler.
//
// handle serves a campaign command: the router has already checked the rate limit, routed the server
// to its campaign and checked that the caller is an ACTIVE member, so it only builds the content from
// what the (player projection) viewer may see. A core error it returns is answered with core's words.
//
// suggest, when set, answers the command's autocomplete through the same routing, with the same
// player-projection viewer.
//
// handleServer serves a command that acts for the server rather than a campaign (/link, /unlink): the
// router has checked the rate limit and Manage Server instead.
//
// Either way the router makes the reply ephemeral and mention-free.
type command struct {
	definition   *discordgo.ApplicationCommand
	handle       func(requestContext context.Context, service colonyService, viewer core.Viewer, data discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error)
	suggest      func(requestContext context.Context, service colonyService, viewer core.Viewer, data discordgo.ApplicationCommandInteractionData) ([]*discordgo.ApplicationCommandOptionChoice, error)
	handleServer func(requestContext context.Context, service colonyService, invocation serverInvocation) (*discordgo.InteractionResponseData, error)
}

// serverInvocation is a server command as its handler sees it.
type serverInvocation struct {
	guildID string
	// guildName is the gateway's name for the server, or empty when the gateway hasn't seen it.
	guildName     string
	discordUserID string
	data          discordgo.ApplicationCommandInteractionData
}

func slashCommands() []command {
	// Discord shows these to members with Manage Server; Administrator implies it.
	serverManagersOnly := int64(discordgo.PermissionManageGuild)
	// Commands work only in servers. Direct messages still get an answer if a client sends one anyway.
	inServersOnly := []discordgo.InteractionContextType{discordgo.InteractionContextGuild}
	return []command{
		{
			definition: &discordgo.ApplicationCommand{Name: "beny", Description: "Beny Bot!", Contexts: &inServersOnly},
			handle:     answerBeny,
		},
		{
			definition: &discordgo.ApplicationCommand{
				Name:                     "link",
				Description:              "Link this server to a campaign with the code from its Settings",
				DefaultMemberPermissions: &serverManagersOnly,
				Contexts:                 &inServersOnly,
				Options: []*discordgo.ApplicationCommandOption{{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "code",
					Description: "The link code, like BENY-XXXXX-XXXXX",
					Required:    true,
					MaxLength:   linkCodeOptionMaxLength,
				}},
			},
			handleServer: linkServer,
		},
		{
			definition: &discordgo.ApplicationCommand{
				Name:                     "unlink",
				Description:              "Unlink this server from its campaign",
				DefaultMemberPermissions: &serverManagersOnly,
				Contexts:                 &inServersOnly,
			},
			handleServer: unlinkServer,
		},
		{
			definition: &discordgo.ApplicationCommand{
				Name:        "npc",
				Description: "Look up an NPC the party knows",
				Contexts:    &inServersOnly,
				Options: []*discordgo.ApplicationCommandOption{{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        npcViewSubcommand,
					Description: "Show what the party knows about an NPC",
					Options: []*discordgo.ApplicationCommandOption{{
						Type:         discordgo.ApplicationCommandOptionString,
						Name:         npcNameOption,
						Description:  "The name the party knows them by",
						Required:     true,
						Autocomplete: true,
						MaxLength:    choiceMaxLength,
					}},
				}},
			},
			handle:  viewNpc,
			suggest: suggestNpcs,
		},
	}
}

func (benyBot *Bot) findCommand(name string) (command, bool) {
	for _, slashCommand := range benyBot.commands {
		if slashCommand.definition.Name == name {
			return slashCommand, true
		}
	}
	return command{}, false
}

func answerBeny(context.Context, colonyService, core.Viewer, discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
	return &discordgo.InteractionResponseData{Content: "Boy!"}, nil
}

// linkServer is /link code:<it>. The code is the DM's consent and Manage Server the server's; core
// checks the code (live, single use) and refuses a server or campaign that is already linked, or a
// suspended campaign, with its own words.
func linkServer(requestContext context.Context, service colonyService, invocation serverInvocation) (*discordgo.InteractionResponseData, error) {
	linked, err := service.LinkGuild(requestContext, core.LinkGuildInput{
		Code:          stringOption(invocation.data.Options, "code"),
		GuildID:       invocation.guildID,
		GuildName:     invocation.guildName,
		DiscordUserID: invocation.discordUserID,
	})
	if err != nil {
		return nil, err
	}
	return &discordgo.InteractionResponseData{
		Content: fmt.Sprintf("Linked this server to **%s**. Pick channels in the campaign's Settings.", escapeMarkdown(linked.CampaignName)),
	}, nil
}

// unlinkServer is /unlink. It names no campaign: whoever manages the server may not be in it.
func unlinkServer(requestContext context.Context, service colonyService, invocation serverInvocation) (*discordgo.InteractionResponseData, error) {
	unlinked, err := service.UnlinkGuildByID(requestContext, invocation.guildID)
	if err != nil {
		return nil, err
	}
	if !unlinked {
		return &discordgo.InteractionResponseData{Content: "This server isn't linked to a campaign."}, nil
	}
	return &discordgo.InteractionResponseData{
		Content: "Unlinked this server from its campaign. A DM can link it again with a new code from their campaign's Settings.",
	}, nil
}

// subcommandOptions reads the options of the subcommand called name, or reports that another ran.
func subcommandOptions(data discordgo.ApplicationCommandInteractionData, name string) ([]*discordgo.ApplicationCommandInteractionDataOption, bool) {
	for _, option := range data.Options {
		if option != nil && option.Type == discordgo.ApplicationCommandOptionSubCommand && option.Name == name {
			return option.Options, true
		}
	}
	return nil, false
}

// stringOption reads a string option without discordgo's StringValue, which panics on another type.
func stringOption(options []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, option := range options {
		if option == nil || option.Name != name {
			continue
		}
		if value, isString := option.Value.(string); isString {
			return value
		}
	}
	return ""
}
