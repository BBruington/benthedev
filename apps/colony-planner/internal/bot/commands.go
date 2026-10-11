package bot

import (
	"context"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

// command is one slash command. The router has already checked the guild, the rate limit and that the
// caller is an ACTIVE member; it also makes the reply ephemeral, so a handler only builds the content
// from what the viewer may see.
type command struct {
	definition *discordgo.ApplicationCommand
	handle     func(requestContext context.Context, viewer core.Viewer, data discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error)
}

func slashCommands() []command {
	return []command{
		{
			definition: &discordgo.ApplicationCommand{Name: "beny", Description: "Beny Bot!"},
			handle:     answerBeny,
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

func answerBeny(context.Context, core.Viewer, discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
	return &discordgo.InteractionResponseData{Content: "Boy!"}, nil
}
