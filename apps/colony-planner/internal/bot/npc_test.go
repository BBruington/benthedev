package bot

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const (
	notFoundNpcMessage = "No NPC the party knows goes by that name."

	// Every value below is a canary: none may appear in any reply or suggestion in A's server.
	hiddenTrueName     = "Bramwell Hiddentrue"
	hiddenKnownName    = "Brother Quiet"
	revealedTrueName   = "Selwyn Truename"
	revealedKnownName  = "The Hooded Stranger"
	dmNotesCanary      = "DMNOTESCANARY"
	hiddenPlaceName    = "The Drowned Chapel"
	campaignBNpcName   = "Brakka of Ridgehold"
	campaignBHiddenNpc = "Brenna Hiddenb"
)

// newNpcWorld links server X to campaign A and server Y to campaign B, and gives each campaign
// revealed and hidden NPCs whose names share a prefix, so a leak shows up in a prefix search.
func newNpcWorld(t *testing.T) *world {
	t.Helper()
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.link(guildY, test.dmB)

	chapel := test.place(test.dmA, hiddenPlaceName, false)
	docks := test.place(test.dmA, "Port Eldari Docks", true)
	level := int64(3)
	brannoc := test.npc(test.dmA, core.NpcInput{
		Name:           "Brannoc",
		Revealed:       true,
		Aliases:        []string{"Old Bran"},
		Labels:         []string{"farmer"},
		Race:           "Human",
		CharacterClass: "Fighter",
		Level:          &level,
		Age:            "40s",
		Appearance:     "Broad\nshouldered",
		Personality:    "*Gruff*",
		PlayerNotes:    "Owes Pell a favor.",
		DmNotes:        dmNotesCanary,
		Status:         core.NpcStatusAlive,
		Health:         core.NpcHealthInjured,
		Relation:       core.NpcRelationAlly,
		CurrentPlaceID: &chapel,
		OriginPlaceID:  &docks,
	})
	test.joinColony(test.dmA, brannoc, "FARMER")
	test.npc(test.dmA, core.NpcInput{Name: "Bramble", Revealed: true, DmNotes: dmNotesCanary})
	test.npc(test.dmA, core.NpcInput{Name: hiddenTrueName, KnownName: hiddenKnownName, DmNotes: dmNotesCanary})
	test.npc(test.dmA, core.NpcInput{Name: revealedTrueName, KnownName: revealedKnownName, Revealed: true, DmNotes: dmNotesCanary})
	test.npc(test.dmB, core.NpcInput{Name: campaignBNpcName, Revealed: true})
	test.npc(test.dmB, core.NpcInput{Name: campaignBHiddenNpc})
	return test
}

func (test *world) place(dm core.Viewer, name string, revealed bool) uuid.UUID {
	test.t.Helper()
	place, err := test.service.CreatePlace(test.context, dm, core.PlaceInput{Name: name, Revealed: revealed})
	if err != nil {
		test.t.Fatalf("create place %s: %v", name, err)
	}
	return place.ID
}

func (test *world) npc(dm core.Viewer, input core.NpcInput) uuid.UUID {
	test.t.Helper()
	npc, err := test.service.CreateNpc(test.context, dm, input)
	if err != nil {
		test.t.Fatalf("create NPC %s: %v", input.Name, err)
	}
	return npc.ID
}

func (test *world) joinColony(dm core.Viewer, npcID uuid.UUID, roleKey string) {
	test.t.Helper()
	board, err := test.service.GetColonyBoard(test.context, dm)
	if err != nil {
		test.t.Fatalf("read the colony: %v", err)
	}
	if _, err := test.service.JoinColony(test.context, dm, npcID, core.JoinColonyInput{ExpectedVersion: board.Version, RoleKey: roleKey}); err != nil {
		test.t.Fatalf("join the colony: %v", err)
	}
}

func npcView(guildID string, discordUserID string, name string) *discordgo.InteractionCreate {
	return serverCommand(guildID, discordUserID, "npc", 0, &discordgo.ApplicationCommandInteractionDataOption{
		Name: npcViewSubcommand,
		Type: discordgo.ApplicationCommandOptionSubCommand,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{Name: npcNameOption, Type: discordgo.ApplicationCommandOptionString, Value: name},
		},
	})
}

// npcAutocomplete is Discord asking for suggestions while typed is in the name box.
func npcAutocomplete(guildID string, discordUserID string, typed string) *discordgo.InteractionCreate {
	event := npcView(guildID, discordUserID, typed)
	event.Type = discordgo.InteractionApplicationCommandAutocomplete
	data, _ := event.Data.(discordgo.ApplicationCommandInteractionData)
	data.Options[0].Options[0].Focused = true
	return event
}

// runFull is run for a reply that may carry an embed: it returns the whole response.
func (test *world) runFull(event *discordgo.InteractionCreate) *discordgo.InteractionResponse {
	test.t.Helper()
	test.run(event)
	responses := test.client.recordedResponses()
	return responses[len(responses)-1]
}

// suggest hands Beny Bot one autocomplete request and returns the names of the choices it offers. Each
// choice's value must be its name, within Discord's limits.
func (test *world) suggest(event *discordgo.InteractionCreate) []string {
	test.t.Helper()
	test.clock.advance(commandRefillInterval)
	before := len(test.client.recordedResponses())
	test.bot.handleInteraction(test.context, event)
	responses := test.client.recordedResponses()
	if len(responses) != before+1 {
		test.t.Fatalf("got %d answers, want 1", len(responses)-before)
	}
	return requireChoices(test.t, responses[before])
}

func requireChoices(t *testing.T, response *discordgo.InteractionResponse) []string {
	t.Helper()
	if response.Type != discordgo.InteractionApplicationCommandAutocompleteResult {
		t.Fatalf("response type = %v, want autocomplete choices", response.Type)
	}
	choices := response.Data.Choices
	// Discord refuses an empty list of choices, and more than 25.
	if len(choices) == 0 || len(choices) > core.BotNpcSuggestionLimit {
		t.Fatalf("got %d choices", len(choices))
	}
	names := make([]string, 0, len(choices))
	for _, choice := range choices {
		value, isString := choice.Value.(string)
		if !isString || value != choice.Name {
			t.Errorf("choice %q has value %v", choice.Name, choice.Value)
		}
		if length := utf8.RuneCountInString(choice.Name); length == 0 || length > choiceMaxLength {
			t.Errorf("choice %q is %d characters long", choice.Name, length)
		}
		names = append(names, choice.Name)
	}
	return names
}

// requireNoCanary fails if a response carries any value players must never see in A's server.
func requireNoCanary(t *testing.T, response *discordgo.InteractionResponse) {
	t.Helper()
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"Bramwell", "Hiddentrue", hiddenKnownName, "Selwyn", "Truename", dmNotesCanary, "Drowned", campaignBNpcName, "Brenna"} {
		if strings.Contains(string(encoded), canary) {
			t.Errorf("a reply in A's server contains %q: %s", canary, encoded)
		}
	}
}

func TestNpcViewIsRegisteredWithAutocomplete(t *testing.T) {
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, newFakeDiscordClient(), newFakeGatewayState(), newFakeClock().now)
	npcCommand, found := benyBot.findCommand("npc")
	if !found || npcCommand.handle == nil || npcCommand.suggest == nil {
		t.Fatal("/npc isn't registered with a handler and suggestions")
	}
	options := npcCommand.definition.Options
	if len(options) != 1 || options[0].Type != discordgo.ApplicationCommandOptionSubCommand || options[0].Name != "view" {
		t.Fatalf("/npc offers %+v, want the view subcommand", options)
	}
	nameOptions := options[0].Options
	if len(nameOptions) != 1 {
		t.Fatalf("/npc view has %d options, want name", len(nameOptions))
	}
	name := nameOptions[0]
	if name.Name != "name" || name.Type != discordgo.ApplicationCommandOptionString || !name.Required || !name.Autocomplete {
		t.Errorf("/npc view's name option is %+v", name)
	}
}

func TestNpcAutocompleteSuggestsOnlyRevealedDisplayNamesFromTheServersCampaign(t *testing.T) {
	test := newNpcWorld(t)
	for _, testCase := range []struct {
		name          string
		guildID       string
		discordUserID string
		typed         string
		expected      []string
	}{
		{"a prefix, ignoring case, skips hidden NPCs and campaign B", guildX, activeUserID, "BR", []string{"Bramble", "Brannoc"}},
		{"nothing typed lists every revealed NPC", guildX, activeUserID, "", []string{"Bramble", "Brannoc", revealedKnownName}},
		{"the DM gets the player projection", guildX, dmUserID, "br", []string{"Bramble", "Brannoc"}},
		{"a known name matches", guildX, activeUserID, "the h", []string{revealedKnownName}},
		{"a hidden NPC's true name finds nothing", guildX, activeUserID, "Bramw", []string{noMatchingNpcsMessage}},
		{"a hidden NPC's known name finds nothing", guildX, activeUserID, "Brother", []string{noMatchingNpcsMessage}},
		{"the DM can't find a hidden NPC either", guildX, dmUserID, "Bramw", []string{noMatchingNpcsMessage}},
		{"a true name behind a known name finds nothing", guildX, activeUserID, "Selwyn", []string{noMatchingNpcsMessage}},
		{"server Y answers for campaign B only", guildY, activeUserID, "br", []string{campaignBNpcName}},
		{"campaign B's DM in B's server sees no hidden NPC", guildY, otherDmUserID, "", []string{campaignBNpcName}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			before := len(test.client.recordedResponses())
			suggestions := test.suggest(npcAutocomplete(testCase.guildID, testCase.discordUserID, testCase.typed))
			if !slices.Equal(suggestions, testCase.expected) {
				t.Errorf("suggestions = %q, want %q", suggestions, testCase.expected)
			}
			if testCase.guildID == guildX {
				requireNoCanary(t, test.client.recordedResponses()[before])
			}
		})
	}
}

func TestNpcAutocompleteWithNoRevealedNpcs(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.npc(test.dmA, core.NpcInput{Name: hiddenTrueName})
	if suggestions := test.suggest(npcAutocomplete(guildX, activeUserID, "")); !slices.Equal(suggestions, []string{noKnownNpcsMessage}) {
		t.Errorf("suggestions = %q", suggestions)
	}
}

func TestNpcViewRepliesFromThePlayerProjection(t *testing.T) {
	test := newNpcWorld(t)

	response := test.runFull(npcView(guildX, activeUserID, "  brannoc "))
	requireNoCanary(t, response)
	if len(response.Data.Embeds) != 1 {
		t.Fatalf("got %d embeds, want 1", len(response.Data.Embeds))
	}
	embed := response.Data.Embeds[0]
	if embed.Title != "Brannoc" {
		t.Errorf("title = %q", embed.Title)
	}
	fields := map[string]string{}
	for _, field := range embed.Fields {
		fields[field.Name] = field.Value
	}
	expected := map[string]string{
		"Also known as": "Old Bran",
		"Race":          "Human",
		"Class":         "Fighter",
		"Level":         "3",
		"Age":           "40s",
		"Status":        "Alive",
		"Health":        "Injured",
		"Relation":      "Ally",
		"Colony role":   "Farmer",
		"Origin":        "Port Eldari Docks",
		"Labels":        "farmer",
		"Appearance":    "Broad\nshouldered",
		"Personality":   `\*Gruff\*`,
		"Notes":         "Owes Pell a favor.",
	}
	for name, value := range expected {
		if fields[name] != value {
			t.Errorf("field %q = %q, want %q", name, fields[name], value)
		}
	}
	// The current place is hidden, so the field is left out rather than named.
	if _, shown := fields["Current place"]; shown {
		t.Error("a hidden current place is shown")
	}
	if len(fields) != len(expected) {
		t.Errorf("fields = %v, want exactly %v", fields, expected)
	}

	// The DM gets the same projection: the known name, never the true one or the notes.
	dmResponse := test.runFull(npcView(guildX, dmUserID, revealedKnownName))
	requireNoCanary(t, dmResponse)
	if len(dmResponse.Data.Embeds) != 1 || dmResponse.Data.Embeds[0].Title != revealedKnownName {
		t.Errorf("the DM's /npc view answered %+v", dmResponse.Data)
	}
	// Unknown status, health and relation say nothing, so they are left out.
	bramble := test.runFull(npcView(guildX, activeUserID, "Bramble"))
	if embeds := bramble.Data.Embeds; len(embeds) != 1 || len(embeds[0].Fields) != 1 || embeds[0].Fields[0].Name != "Health" {
		t.Errorf("Bramble's embed is %+v, want only the default health", bramble.Data)
	}

	for _, testCase := range []struct {
		name          string
		guildID       string
		discordUserID string
		npcName       string
	}{
		{"a hidden NPC's true name", guildX, activeUserID, hiddenTrueName},
		{"a hidden NPC's known name", guildX, activeUserID, hiddenKnownName},
		{"a hidden NPC's true name, run by the DM", guildX, dmUserID, hiddenTrueName},
		{"a true name behind a known name", guildX, activeUserID, revealedTrueName},
		{"a true name behind a known name, run by the DM", guildX, dmUserID, revealedTrueName},
		{"campaign B's NPC in A's server", guildX, activeUserID, campaignBNpcName},
		{"campaign B's hidden NPC in B's server", guildY, otherDmUserID, campaignBHiddenNpc},
		{"a prefix is not a name", guildX, activeUserID, "Bran"},
		{"nothing typed", guildX, activeUserID, ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := test.runFull(npcView(testCase.guildID, testCase.discordUserID, testCase.npcName))
			if response.Data.Content != notFoundNpcMessage || len(response.Data.Embeds) != 0 {
				t.Errorf("reply = %q with %d embeds, want %q", response.Data.Content, len(response.Data.Embeds), notFoundNpcMessage)
			}
			if testCase.guildID == guildX {
				requireNoCanary(t, response)
			}
		})
	}

	inB := test.runFull(npcView(guildY, activeUserID, campaignBNpcName))
	if len(inB.Data.Embeds) != 1 || inB.Data.Embeds[0].Title != campaignBNpcName {
		t.Errorf("campaign B's server answered %+v", inB.Data)
	}
}

func TestNpcViewRoutesByServerAndMembership(t *testing.T) {
	test := newNpcWorld(t)
	for _, testCase := range []struct {
		name          string
		guildID       string
		discordUserID string
		expected      string
	}{
		{"a signed-in account in no campaign", guildX, strangerUserID, core.ErrNotInCampaign.Message},
		{"a Discord user who never signed in", guildX, unknownUserID, core.ErrNotInCampaign.Message},
		{"a member of another campaign only", guildX, otherOnlyUserID, core.ErrNotInCampaign.Message},
		{"another campaign's DM", guildX, otherDmUserID, core.ErrNotInCampaign.Message},
		{"a pending member", guildX, pendingUserID, core.ErrNotLetIn.Message},
		{"a removed member", guildX, removedUserID, core.ErrNotLetIn.Message},
		{"an unlinked server", guildZ, activeUserID, core.ErrGuildNotLinked.Message},
		{"a direct message", "", activeUserID, useInServerMessage},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := test.runFull(npcView(testCase.guildID, testCase.discordUserID, "Brannoc"))
			if response.Data.Content != testCase.expected || len(response.Data.Embeds) != 0 {
				t.Errorf("/npc view answered %q with %d embeds, want %q", response.Data.Content, len(response.Data.Embeds), testCase.expected)
			}
			suggestions := test.suggest(npcAutocomplete(testCase.guildID, testCase.discordUserID, "br"))
			if expected := fitText(testCase.expected, choiceMaxLength, keepText); !slices.Equal(suggestions, []string{expected}) {
				t.Errorf("suggestions = %q, want only %q", suggestions, expected)
			}
		})
	}

	if _, err := test.service.SuspendCampaign(test.context, test.owner, test.dmA.CampaignID()); err != nil {
		t.Fatal(err)
	}
	if reply := test.run(npcView(guildX, activeUserID, "Brannoc")); reply != core.ErrCampaignSuspended.Message {
		t.Errorf("a suspended campaign's server answered %q", reply)
	}
	if suggestions := test.suggest(npcAutocomplete(guildX, activeUserID, "br")); !slices.Equal(suggestions, []string{core.ErrCampaignSuspended.Message}) {
		t.Errorf("a suspended campaign's server suggested %q", suggestions)
	}
}

// Suggestions arrive as a name is typed: they have their own allowance and never use up the commands'.
func TestNpcSuggestionsAreRateLimitedApartFromCommands(t *testing.T) {
	test := newNpcWorld(t)
	test.clock.advance(time.Hour)
	before := len(test.client.recordedResponses())
	for range suggestionBurst + 1 {
		test.bot.handleInteraction(test.context, npcAutocomplete(guildX, activeUserID, "br"))
	}
	for range commandBurst {
		test.bot.handleInteraction(test.context, slashCommand(guildX, activeUserID, "beny"))
	}
	responses := test.client.recordedResponses()[before:]
	if len(responses) != suggestionBurst+1+commandBurst {
		t.Fatalf("got %d responses", len(responses))
	}
	for index, response := range responses[:suggestionBurst] {
		if suggestions := requireChoices(t, response); !slices.Equal(suggestions, []string{"Bramble", "Brannoc"}) {
			t.Errorf("suggestion %d = %q", index, suggestions)
		}
	}
	if suggestions := requireChoices(t, responses[suggestionBurst]); !slices.Equal(suggestions, []string{slowDownMessage}) {
		t.Errorf("past the allowance, suggestions = %q", suggestions)
	}
	for _, response := range responses[suggestionBurst+1:] {
		if content := requireEphemeral(t, response); content != "Boy!" {
			t.Errorf("a command after many suggestions answered %q", content)
		}
	}
}

func TestNpcViewFailuresGetAGenericReply(t *testing.T) {
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, client, newFakeGatewayState(), newFakeClock().now)
	if err := benyBot.start(failingService{}); err != nil {
		t.Fatal(err)
	}
	benyBot.handleInteraction(t.Context(), npcView(guildX, activeUserID, "Brannoc"))
	benyBot.handleInteraction(t.Context(), npcAutocomplete(guildX, activeUserID, "br"))
	responses := client.recordedResponses()
	if len(responses) != 2 {
		t.Fatalf("got %d replies, want 2", len(responses))
	}
	if content := requireEphemeral(t, responses[0]); content != failureMessage {
		t.Errorf("/npc view answered %q", content)
	}
	if suggestions := requireChoices(t, responses[1]); !slices.Equal(suggestions, []string{failureMessage}) {
		t.Errorf("suggestions = %q", suggestions)
	}
}

// An NPC filled to every limit core allows, with text that doubles when escaped, still fits Discord's
// embed limits, and no cut splits an escape.
func TestNpcEmbedFitsDiscordsLimits(t *testing.T) {
	long := func(character string, length int) string { return strings.Repeat(character, length) }
	many := func(character string, length int) []string {
		values := make([]string, 20)
		for index := range values {
			values[index] = long(character, length)
		}
		return values
	}
	level := int64(100)
	embed := npcEmbed(core.Npc{
		DisplayName:    long("*", 80),
		Aliases:        many("_", 80),
		Labels:         many("~", 40),
		Race:           long("`", 80),
		CharacterClass: long("|", 80),
		Level:          &level,
		Age:            long(">", 80),
		Appearance:     long("<", 4000),
		Personality:    long("[", 4000),
		Backstory:      long("]", 4000),
		PlayerNotes:    long("#", 4000),
		Status:         core.NpcStatusDead,
		Health:         core.NpcHealthDead,
		Relation:       core.NpcRelationEnemy,
		CurrentPlace:   &core.PlaceReference{Name: long("(", 80)},
		OriginPlace:    &core.PlaceReference{Name: long(")", 80)},
		ColonyRole:     &core.ColonyRoleReference{Label: long("-", 80)},
	})
	total := utf8.RuneCountInString(embed.Title)
	if utf8.RuneCountInString(embed.Title) > embedTitleMaxLength {
		t.Errorf("the title is %d characters", utf8.RuneCountInString(embed.Title))
	}
	if len(embed.Fields) > 25 {
		t.Errorf("the embed has %d fields", len(embed.Fields))
	}
	for _, field := range embed.Fields {
		length := utf8.RuneCountInString(field.Value)
		total += utf8.RuneCountInString(field.Name) + length
		if length == 0 || length > embedFieldValueMaxLength {
			t.Errorf("field %q is %d characters", field.Name, length)
		}
		if strings.HasSuffix(strings.TrimSuffix(field.Value, "…"), `\`) {
			t.Errorf("field %q was cut inside an escape", field.Name)
		}
	}
	if total > embedMaxLength {
		t.Errorf("the embed is %d characters, over Discord's %d", total, embedMaxLength)
	}
	if embed.Fields[0].Name != "Also known as" {
		t.Errorf("the first field is %q", embed.Fields[0].Name)
	}
}
