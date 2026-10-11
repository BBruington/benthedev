package bot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const testDmChannelID = "500000000000000001"

func waitingAna() core.WaitingMember {
	return core.WaitingMember{MemberID: uuid.New(), DisplayName: "Ana", DiscordUsername: "ana"}
}

func TestMemberWaitingPostsToTheDmChannel(t *testing.T) {
	benyBot, client, service, _ := newRunningTestBot(t)
	service.setDmChannel(testDmChannelID)

	benyBot.MemberWaiting(t.Context(), waitingAna())
	benyBot.posts.Wait()

	messages := client.recordedMessages()
	if len(messages) != 1 {
		t.Fatalf("posted %d messages, want 1", len(messages))
	}
	if messages[0].channelID != testDmChannelID {
		t.Errorf("posted to %q, want %q", messages[0].channelID, testDmChannelID)
	}
	expected := "Ana (@ana) is waiting to join: http://localhost:5173/players"
	if messages[0].message.Content != expected {
		t.Errorf("content = %q, want %q", messages[0].message.Content, expected)
	}
	encoded, err := json.Marshal(messages[0].message.AllowedMentions)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"parse":[]`) {
		t.Errorf("allowed mentions encode as %s, want an empty parse list", encoded)
	}
	if messages[0].message.Flags&discordgo.MessageFlagsSuppressEmbeds == 0 {
		t.Error("the ping lets Discord show a link preview under it")
	}
}

func TestMemberWaitingOutlivesTheSignInRequest(t *testing.T) {
	benyBot, client, service, _ := newRunningTestBot(t)
	service.setDmChannel(testDmChannelID)

	requestContext, cancelRequest := context.WithCancel(t.Context())
	benyBot.MemberWaiting(requestContext, waitingAna())
	cancelRequest()
	benyBot.posts.Wait()

	if messages := client.recordedMessages(); len(messages) != 1 {
		t.Fatalf("posted %d messages, want 1", len(messages))
	}
}

func TestMemberWaitingSkipsQuietlyWithoutADmChannel(t *testing.T) {
	benyBot, client, service, _ := newRunningTestBot(t)

	benyBot.MemberWaiting(t.Context(), waitingAna())
	benyBot.posts.Wait()
	if messages := client.recordedMessages(); len(messages) != 0 {
		t.Fatalf("posted %d messages, want none", len(messages))
	}

	// Skipped pings don't use up the hourly allowance.
	service.setDmChannel(testDmChannelID)
	for range waitingPingLimit {
		benyBot.MemberWaiting(t.Context(), waitingAna())
	}
	benyBot.posts.Wait()
	if messages := client.recordedMessages(); len(messages) != waitingPingLimit {
		t.Fatalf("posted %d messages, want %d", len(messages), waitingPingLimit)
	}
}

func TestMemberWaitingSendsAtMostFivePingsAnHour(t *testing.T) {
	benyBot, client, service, clock := newRunningTestBot(t)
	service.setDmChannel(testDmChannelID)

	for range waitingPingLimit + 2 {
		benyBot.MemberWaiting(t.Context(), waitingAna())
		benyBot.posts.Wait()
		clock.advance(time.Minute)
	}
	if messages := client.recordedMessages(); len(messages) != waitingPingLimit {
		t.Fatalf("posted %d messages in the first hour, want %d", len(messages), waitingPingLimit)
	}

	// The first ping was sent 7 minutes ago; once it is an hour old, one more fits.
	clock.advance(waitingPingWindow - 7*time.Minute)
	benyBot.MemberWaiting(t.Context(), waitingAna())
	benyBot.MemberWaiting(t.Context(), waitingAna())
	benyBot.posts.Wait()
	if messages := client.recordedMessages(); len(messages) != waitingPingLimit+1 {
		t.Fatalf("posted %d messages, want %d", len(messages), waitingPingLimit+1)
	}
}

func TestMemberWaitingBeforeRunOrAfterShutdownPostsNothing(t *testing.T) {
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", GuildID: testGuildID, PublicURL: testPublicURL, Logger: quietLogger()}, client, newFakeClock().now)
	benyBot.MemberWaiting(t.Context(), waitingAna())

	service := newFakeColonyService()
	service.setDmChannel(testDmChannelID)
	if err := benyBot.start(service); err != nil {
		t.Fatal(err)
	}
	benyBot.stop()
	benyBot.MemberWaiting(t.Context(), waitingAna())
	benyBot.posts.Wait()

	if messages := client.recordedMessages(); len(messages) != 0 {
		t.Fatalf("posted %d messages, want none", len(messages))
	}
}

func TestMemberWaitingSurvivesFailures(t *testing.T) {
	benyBot, client, service, _ := newRunningTestBot(t)
	service.channelsError = errors.New("connection refused")
	benyBot.MemberWaiting(t.Context(), waitingAna())
	benyBot.posts.Wait()
	if messages := client.recordedMessages(); len(messages) != 0 {
		t.Fatalf("posted %d messages without settings, want none", len(messages))
	}

	service.channelsError = nil
	service.setDmChannel(testDmChannelID)
	client.sendError = errors.New("HTTP 403 Forbidden")
	benyBot.MemberWaiting(t.Context(), waitingAna())
	benyBot.posts.Wait()
	if messages := client.recordedMessages(); len(messages) != 1 {
		t.Fatalf("attempted %d posts, want 1", len(messages))
	}
}

func TestWaitingMessageShowsNamesLiterally(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		member   core.WaitingMember
		expected string
	}{
		{
			"plain names",
			core.WaitingMember{DisplayName: "Thorin", DiscordUsername: "thorin.oak"},
			"Thorin (@thorin.oak) is waiting to join: http://localhost:5173/players",
		},
		{
			"underscores in usernames are not italics",
			core.WaitingMember{DisplayName: "Ana", DiscordUsername: "ana_the_bold"},
			`Ana (@ana\_the\_bold) is waiting to join: http://localhost:5173/players`,
		},
		{
			"markdown, masked links and mentions are escaped",
			core.WaitingMember{DisplayName: "# **[click](https://example.com)** <@&1>", DiscordUsername: "mallory"},
			`\# \*\*\[click\]\(https\://example.com\)\*\* \<@&1\> (@mallory) is waiting to join: http://localhost:5173/players`,
		},
		{
			"a bare link in a name is not clickable",
			core.WaitingMember{DisplayName: "https://evil.example/players", DiscordUsername: "mallory"},
			`https\://evil.example/players (@mallory) is waiting to join: http://localhost:5173/players`,
		},
		{
			"emoji codes are shown as typed",
			core.WaitingMember{DisplayName: "Ana :crown:", DiscordUsername: "ana"},
			`Ana \:crown\: (@ana) is waiting to join: http://localhost:5173/players`,
		},
		{
			"a missing display name falls back to the username",
			core.WaitingMember{DiscordUsername: "brannoc"},
			"brannoc (@brannoc) is waiting to join: http://localhost:5173/players",
		},
		{
			"line breaks stay on one line",
			core.WaitingMember{DisplayName: "Ana\n> quoted", DiscordUsername: "ana"},
			`Ana \> quoted (@ana) is waiting to join: http://localhost:5173/players`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if actual := waitingMessage(testCase.member, testPublicURL); actual != testCase.expected {
				t.Errorf("got  %q\nwant %q", actual, testCase.expected)
			}
			if strings.Contains(waitingMessage(testCase.member, testPublicURL), "\n") {
				t.Error("message spans several lines")
			}
		})
	}
}
