package bot

import (
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

	for _, guildID := range []string{guildZ, "100000000000000077", ""} {
		channels, err := benyBot.TextChannels(t.Context(), guildID)
		if err != nil || channels == nil || len(channels) != 0 {
			t.Errorf("guild %q: channels = %#v, %v; want an empty list", guildID, channels, err)
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
