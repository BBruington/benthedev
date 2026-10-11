package bot

import (
	"errors"
	"slices"
	"testing"

	"github.com/BBruington/colony-planner/internal/core"
)

func TestTextChannelsListOnlyTheServersTextChannelsInOrder(t *testing.T) {
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, newFakeDiscordClient(), newFakeGatewayState(), newFakeClock().now)

	channels, err := benyBot.TextChannels(t.Context(), guildX)
	if err != nil {
		t.Fatal(err)
	}
	expected := []core.DiscordChannel{
		{ID: channelXGeneral, Name: "general"},
		{ID: channelXDesk, Name: "dm-desk"},
		{ID: channelXNews, Name: "news"},
	}
	if !slices.Equal(channels, expected) {
		t.Errorf("channels = %+v, want %+v", channels, expected)
	}

	for _, guildID := range []string{guildZ, ""} {
		channels, err := benyBot.TextChannels(t.Context(), guildID)
		if err != nil || channels == nil || len(channels) != 0 {
			t.Errorf("guild %q: channels = %#v, %v; want an empty list", guildID, channels, err)
		}
	}
}

// A server the gateway doesn't know right now has an unknown list, not an empty one.
func TestTextChannelsOfAServerTheGatewayCantSeeAreUnavailable(t *testing.T) {
	state := newFakeGatewayState()
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, newFakeDiscordClient(), state, newFakeClock().now)
	state.markUnavailable(guildY)
	for _, guildID := range []string{"100000000000000077", guildY} {
		if channels, err := benyBot.TextChannels(t.Context(), guildID); !errors.Is(err, core.ErrDiscordListUnavailable) {
			t.Errorf("guild %q: channels = %#v, %v; want core.ErrDiscordListUnavailable", guildID, channels, err)
		}
	}
}

func TestChannelInGuildChecksTheGatewaysServer(t *testing.T) {
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, newFakeDiscordClient(), newFakeGatewayState(), newFakeClock().now)
	for _, testCase := range []struct {
		channelID string
		guildID   string
		expected  bool
	}{
		{channelXDesk, guildX, true},
		{channelXNews, guildX, true},
		{channelXDesk, guildY, false},
		{channelYDesk, guildX, false},
		{channelXVoice, guildX, false},
		{"300000000000000077", guildX, false},
		{"", guildX, false},
	} {
		if actual := benyBot.channelInGuild(testCase.channelID, testCase.guildID); actual != testCase.expected {
			t.Errorf("channelInGuild(%q, %q) = %v, want %v", testCase.channelID, testCase.guildID, actual, testCase.expected)
		}
	}
}

// Right after a restart, before the linked server's GuildCreate arrives, the DM can still save the rest
// of Settings with the stored channels; the picker is empty (and says why), and only a change waits.
func TestSettingsKeepTheStoredChannelsWhileTheGatewayCantSeeTheServer(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.setDmChannel(test.dmA, channelXDesk)
	test.state.forgetGuild(guildX)

	options, err := test.service.DiscordChannelOptions(test.context, test.dmA)
	if err != nil || options == nil || len(options) != 0 {
		t.Fatalf("the picker while the server is unknown = %#v, %v; want an empty list", options, err)
	}
	settings, err := test.service.GetSettings(test.context, test.dmA)
	if err != nil {
		t.Fatal(err)
	}
	input := core.SettingsInput{
		CampaignName:       settings.CampaignName,
		ColonyName:         settings.ColonyName,
		Timezone:           "Europe/Paris",
		DiscordDmChannelID: settings.DiscordDmChannelID,
	}
	saved, err := test.service.UpdateSettings(test.context, test.dmA, input)
	if err != nil || saved.Timezone != "Europe/Paris" || saved.DiscordDmChannelID == nil || *saved.DiscordDmChannelID != channelXDesk {
		t.Fatalf("saving with the stored channel unchanged = %+v, %v", saved, err)
	}

	changed := channelXGeneral
	input.DiscordAnnounceChannelID = &changed
	_, err = test.service.UpdateSettings(test.context, test.dmA, input)
	operationError, typed := core.AsError(err)
	if !typed || operationError.Kind != core.KindInvalid || len(operationError.Fields) != 1 ||
		operationError.Fields[0].Field != "discordAnnounceChannelId" ||
		operationError.Fields[0].Message != "Discord's channel list couldn't be read right now. Try again in a moment." {
		t.Fatalf("a new channel while the server is unknown: %+v", err)
	}
}
