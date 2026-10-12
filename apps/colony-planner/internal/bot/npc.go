package bot

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

const (
	npcViewSubcommand = "view"
	npcNameOption     = "name"

	noKnownNpcsMessage    = "The party doesn't know any NPCs yet."
	noMatchingNpcsMessage = "No NPC the party knows has a name starting like that."
)

// Discord's limits, counted in characters.
const (
	embedTitleMaxLength      = 256
	embedFieldValueMaxLength = 1024
	embedMaxLength           = 6000
	// choiceMaxLength caps an autocomplete choice's name and value, so the name option allows as much.
	choiceMaxLength = 100
)

// viewNpc is /npc view name:. Core answers from the player projection whoever runs it (Open Question
// 11), so a hidden NPC, a true name behind a known name and DM notes never reach Discord.
func viewNpc(requestContext context.Context, service colonyService, viewer core.Viewer, data discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
	options, isView := subcommandOptions(data, npcViewSubcommand)
	if !isView {
		return &discordgo.InteractionResponseData{Content: unknownCommandMessage}, nil
	}
	npc, err := service.FindRevealedNpc(requestContext, viewer, stringOption(options, npcNameOption))
	if err != nil {
		return nil, err
	}
	return &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{npcEmbed(npc)}}, nil
}

// suggestNpcs offers the revealed NPCs whose display name starts with what has been typed. The value
// is the display name itself, so a picked choice and a typed name take the same path.
func suggestNpcs(requestContext context.Context, service colonyService, viewer core.Viewer, data discordgo.ApplicationCommandInteractionData) ([]*discordgo.ApplicationCommandOptionChoice, error) {
	options, isView := subcommandOptions(data, npcViewSubcommand)
	if !isView {
		return refusalChoice(unknownCommandMessage), nil
	}
	prefix := stringOption(options, npcNameOption)
	npcs, err := service.SuggestRevealedNpcs(requestContext, viewer, prefix, core.BotNpcSuggestionLimit)
	if err != nil {
		return nil, err
	}
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(npcs))
	// Names match ignoring case, so a second NPC under the same name could only ever show the first.
	offered := map[string]bool{}
	for _, npc := range npcs {
		key := strings.ToLower(npc.DisplayName)
		if offered[key] {
			continue
		}
		offered[key] = true
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: npc.DisplayName, Value: npc.DisplayName})
	}
	if len(choices) > 0 {
		return choices, nil
	}
	if strings.TrimSpace(prefix) == "" {
		return refusalChoice(noKnownNpcsMessage), nil
	}
	return refusalChoice(noMatchingNpcsMessage), nil
}

// npcEmbed shows an NPC's player-visible fields, short facts first. Long prose is cut to fit Discord's
// limits; the site has the whole text.
func npcEmbed(npc core.Npc) *discordgo.MessageEmbed {
	title := fitText(npc.DisplayName, embedTitleMaxLength, escapeMarkdown)
	fields := embedFields{remaining: embedMaxLength - utf8.RuneCountInString(title)}
	fields.add("Also known as", strings.Join(npc.Aliases, ", "), false, escapeMarkdown)
	fields.add("Race", npc.Race, true, escapeMarkdown)
	fields.add("Class", npc.CharacterClass, true, escapeMarkdown)
	if npc.Level != nil {
		fields.add("Level", strconv.FormatInt(*npc.Level, 10), true, keepText)
	}
	fields.add("Age", npc.Age, true, escapeMarkdown)
	fields.add("Status", enumLabel(npc.Status, core.NpcStatusUnknown), true, keepText)
	fields.add("Health", enumLabel(npc.Health, core.NpcHealthUnknown), true, keepText)
	fields.add("Relation", enumLabel(npc.Relation, core.NpcRelationUnknown), true, keepText)
	if npc.ColonyRole != nil {
		fields.add("Colony role", npc.ColonyRole.Label, true, escapeMarkdown)
	}
	if npc.CurrentPlace != nil {
		fields.add("Current place", npc.CurrentPlace.Name, true, escapeMarkdown)
	}
	if npc.OriginPlace != nil {
		fields.add("Origin", npc.OriginPlace.Name, true, escapeMarkdown)
	}
	fields.add("Labels", strings.Join(npc.Labels, ", "), false, escapeMarkdown)
	fields.add("Appearance", npc.Appearance, false, escapeProse)
	fields.add("Personality", npc.Personality, false, escapeProse)
	fields.add("Backstory", npc.Backstory, false, escapeProse)
	fields.add("Notes", npc.PlayerNotes, false, escapeProse)
	return &discordgo.MessageEmbed{Title: title, Fields: fields.fields}
}

// embedFields collects fields while the embed stays within Discord's total length. A blank value adds
// nothing, because Discord refuses an empty field.
type embedFields struct {
	remaining int
	fields    []*discordgo.MessageEmbedField
}

func (collected *embedFields) add(name string, value string, inline bool, escape func(string) string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	room := min(embedFieldValueMaxLength, collected.remaining-utf8.RuneCountInString(name))
	if room <= 0 {
		return
	}
	fitted := fitText(value, room, escape)
	collected.remaining -= utf8.RuneCountInString(name) + utf8.RuneCountInString(fitted)
	collected.fields = append(collected.fields, &discordgo.MessageEmbedField{Name: name, Value: fitted, Inline: inline})
}

// enumLabel turns ALLY into Ally. Unknown tells the reader nothing, so it is left out.
func enumLabel[Value ~string](value Value, unknown Value) string {
	if value == "" || value == unknown {
		return ""
	}
	return string(value[:1]) + strings.ToLower(string(value[1:]))
}

// fitText escapes text and cuts it to at most limit characters, ending a cut with "…". A cut escapes
// one character at a time, so it never splits an escape in two.
func fitText(text string, limit int, escape func(string) string) string {
	if escaped := escape(text); utf8.RuneCountInString(escaped) <= limit {
		return escaped
	}
	var fitted strings.Builder
	used := 0
	for _, character := range text {
		escaped := escape(string(character))
		length := utf8.RuneCountInString(escaped)
		if used+length > limit-1 {
			break
		}
		fitted.WriteString(escaped)
		used += length
	}
	return fitted.String() + "…"
}

// escapeProse escapes text as escapeMarkdown does but keeps its line breaks, which paragraphs need.
func escapeProse(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = escapeMarkdown(line)
	}
	return strings.Join(lines, "\n")
}

// keepText leaves text as it is, where Discord shows it without markdown (autocomplete choices) or
// the app wrote it.
func keepText(text string) string {
	return text
}
