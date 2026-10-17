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

// setDmChannel picks the campaign's DM channel on Settings, as the DM would; core accepts only a text
// channel of the linked server, from Beny Bot's directory.
func (test *world) setDmChannel(dm core.Viewer, channelID string) {
	test.t.Helper()
	settings, err := test.service.GetSettings(test.context, dm)
	if err != nil {
		test.t.Fatalf("settings: %v", err)
	}
	if _, err := test.service.UpdateSettings(test.context, dm, core.SettingsInput{
		CampaignName:       settings.CampaignName,
		ColonyName:         settings.ColonyName,
		Timezone:           settings.Timezone,
		DiscordDmChannelID: &channelID,
	}); err != nil {
		test.t.Fatalf("set the DM channel: %v", err)
	}
}

// waiting is a new joiner of the DM's campaign, as core announces one.
func waiting(dm core.Viewer, slug string, displayName string) core.WaitingMember {
	return core.WaitingMember{CampaignID: dm.CampaignID(), CampaignSlug: slug, MemberID: uuid.New(), DisplayName: displayName, DiscordUsername: strings.ToLower(displayName)}
}

func TestAJoinerIsAnnouncedInTheirCampaignsDmChannel(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.link(guildY, test.dmB)
	test.setDmChannel(test.dmA, channelXDesk)
	test.setDmChannel(test.dmB, channelYDesk)

	// The real path: a new person opens A's invite link.
	test.join(test.signIn("200000000000000010", "nico"), test.dmA, false)
	test.bot.posts.Wait()

	messages := test.client.messagesTo(channelXDesk)
	if len(messages) != 1 {
		t.Fatalf("posted %d messages to A's DM channel, want 1", len(messages))
	}
	if expected := "nico (@nico) is waiting to join: http://localhost:5173/c/sample-colony/players"; messages[0].message.Content != expected {
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
	if others := test.client.messagesTo(channelYDesk); len(others) != 0 {
		t.Errorf("B's DM channel got %d messages about A's joiner", len(others))
	}
}

func TestPostsGoOnlyToAChannelOfTheLinkedServer(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.setDmChannel(test.dmA, channelXDesk)

	// The stored channel now belongs to another server (the gateway says so): no post.
	test.state.moveChannel(channelXDesk, guildZ)
	test.bot.MemberWaiting(test.context, waiting(test.dmA, campaignASlug, "Ana"))
	test.bot.posts.Wait()
	if messages := test.client.messagesTo(channelXDesk); len(messages) != 0 {
		t.Fatalf("posted %d messages into another server's channel", len(messages))
	}

	// Skipped posts don't use up the hourly allowance.
	test.state.moveChannel(channelXDesk, guildX)
	for range core.WaitingPingsPerHour {
		test.bot.MemberWaiting(test.context, waiting(test.dmA, campaignASlug, "Ana"))
	}
	test.bot.posts.Wait()
	if messages := test.client.messagesTo(channelXDesk); len(messages) != core.WaitingPingsPerHour {
		t.Fatalf("posted %d messages, want %d", len(messages), core.WaitingPingsPerHour)
	}
}

func TestPostsSkipUnlinkedAndSuspendedCampaigns(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.setDmChannel(test.dmA, channelXDesk)

	if _, err := test.service.SuspendCampaign(test.context, test.owner, test.dmA.CampaignID()); err != nil {
		t.Fatal(err)
	}
	test.bot.MemberWaiting(test.context, waiting(test.dmA, campaignASlug, "Ana"))
	// B is not linked at all.
	test.bot.MemberWaiting(test.context, waiting(test.dmB, campaignBSlug, "Iko"))
	test.bot.posts.Wait()
	if messages := test.client.messagesTo(channelXDesk); len(messages) != 0 {
		t.Fatalf("a suspended campaign posted %d messages", len(messages))
	}
	if _, err := test.service.UnsuspendCampaign(test.context, test.owner, test.dmA.CampaignID()); err != nil {
		t.Fatal(err)
	}
	test.bot.MemberWaiting(test.context, waiting(test.dmA, campaignASlug, "Ana"))
	test.bot.posts.Wait()
	if messages := test.client.messagesTo(channelXDesk); len(messages) != 1 {
		t.Fatalf("after unsuspending, posted %d messages, want 1", len(messages))
	}
}

func TestWaitingPingsAreLimitedPerCampaign(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.link(guildY, test.dmB)
	test.setDmChannel(test.dmA, channelXDesk)
	test.setDmChannel(test.dmB, channelYDesk)

	for range core.WaitingPingsPerHour + 2 {
		test.bot.MemberWaiting(test.context, waiting(test.dmA, campaignASlug, "Ana"))
		test.bot.posts.Wait()
		test.clock.advance(time.Minute)
	}
	if messages := test.client.messagesTo(channelXDesk); len(messages) != core.WaitingPingsPerHour {
		t.Fatalf("A posted %d messages in the first hour, want %d", len(messages), core.WaitingPingsPerHour)
	}
	// One busy campaign never silences another.
	test.bot.MemberWaiting(test.context, waiting(test.dmB, campaignBSlug, "Iko"))
	test.bot.posts.Wait()
	if messages := test.client.messagesTo(channelYDesk); len(messages) != 1 {
		t.Fatalf("B posted %d messages, want 1", len(messages))
	}

	// A's first ping was sent 7 minutes ago; once it is an hour old, one more fits.
	test.clock.advance(pingWindow - 7*time.Minute)
	test.bot.MemberWaiting(test.context, waiting(test.dmA, campaignASlug, "Ana"))
	test.bot.MemberWaiting(test.context, waiting(test.dmA, campaignASlug, "Ana"))
	test.bot.posts.Wait()
	if messages := test.client.messagesTo(channelXDesk); len(messages) != core.WaitingPingsPerHour+1 {
		t.Fatalf("A posted %d messages, want %d", len(messages), core.WaitingPingsPerHour+1)
	}
}

func TestMemberWaitingOutlivesTheJoinRequest(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.setDmChannel(test.dmA, channelXDesk)

	requestContext, cancelRequest := context.WithCancel(test.context)
	test.bot.MemberWaiting(requestContext, waiting(test.dmA, campaignASlug, "Ana"))
	cancelRequest()
	test.bot.posts.Wait()
	if messages := test.client.messagesTo(channelXDesk); len(messages) != 1 {
		t.Fatalf("posted %d messages, want 1", len(messages))
	}
}

func TestPostsBeforeRunOrAfterShutdownSendNothing(t *testing.T) {
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", SiteOwnerDiscordID: ownerDiscordID, PublicURL: testPublicURL, Logger: quietLogger()}, client, newFakeGatewayState(), newFakeClock().now)
	member := core.WaitingMember{CampaignID: uuid.New(), CampaignSlug: campaignASlug, MemberID: uuid.New(), DisplayName: "Ana", DiscordUsername: "ana"}
	request := core.RequestedCampaign{CampaignRequestID: uuid.New(), CampaignName: "Sky Isles", DisplayName: "Mira", DiscordUsername: "mira"}
	benyBot.MemberWaiting(t.Context(), member)
	benyBot.CampaignRequested(t.Context(), request)

	if err := benyBot.start(failingService{}); err != nil {
		t.Fatal(err)
	}
	benyBot.stop()
	benyBot.MemberWaiting(t.Context(), member)
	benyBot.CampaignRequested(t.Context(), request)
	benyBot.posts.Wait()

	client.mutex.Lock()
	defer client.mutex.Unlock()
	if len(client.messages) != 0 || len(client.directChannels) != 0 {
		t.Fatalf("sent %d messages and opened %d direct messages, want none", len(client.messages), len(client.directChannels))
	}
}

func TestPostsSurviveFailures(t *testing.T) {
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, client, newFakeGatewayState(), newFakeClock().now)
	if err := benyBot.start(failingService{}); err != nil {
		t.Fatal(err)
	}
	benyBot.MemberWaiting(t.Context(), core.WaitingMember{CampaignID: uuid.New(), CampaignSlug: campaignASlug, MemberID: uuid.New()})
	benyBot.posts.Wait()
	if messages := client.messagesTo(channelXDesk); len(messages) != 0 {
		t.Fatalf("posted %d messages without the campaign's server, want none", len(messages))
	}

	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.setDmChannel(test.dmA, channelXDesk)
	test.client.mutex.Lock()
	test.client.sendError = errors.New("HTTP 403 Forbidden")
	test.client.mutex.Unlock()
	test.bot.MemberWaiting(test.context, waiting(test.dmA, campaignASlug, "Ana"))
	test.bot.posts.Wait()
	if messages := test.client.messagesTo(channelXDesk); len(messages) != 1 {
		t.Fatalf("attempted %d posts, want 1", len(messages))
	}
}

func TestACampaignRequestMessagesTheSiteOwner(t *testing.T) {
	test := newWorld(t)
	ownerChannel := directChannelID(ownerDiscordID)
	before := len(test.client.messagesTo(ownerChannel))

	// The real path: a new account asks to run a campaign.
	mira := test.signIn("200000000000000011", "mira")
	if _, err := test.service.RequestCampaign(test.context, mira, core.CampaignRequestInput{CampaignName: "Sky *Isles*", Slug: "sky-isles"}); err != nil {
		t.Fatal(err)
	}
	test.bot.posts.Wait()
	messages := test.client.messagesTo(ownerChannel)[before:]
	if len(messages) != 1 {
		t.Fatalf("sent the owner %d messages, want 1", len(messages))
	}
	if expected := `mira (@mira) asks to run Sky \*Isles\*: http://localhost:5173/admin`; messages[0].message.Content != expected {
		t.Errorf("content = %q, want %q", messages[0].message.Content, expected)
	}
	if messages[0].message.AllowedMentions == nil || len(messages[0].message.AllowedMentions.Parse) != 0 {
		t.Error("the owner's message allows mentions")
	}
}

func TestOwnerPingsAreLimitedAndNeedARealDiscordID(t *testing.T) {
	clock := newFakeClock()
	client := newFakeDiscordClient()
	benyBot := newBot(Options{Token: "test-token", SiteOwnerDiscordID: ownerDiscordID, PublicURL: testPublicURL, Logger: quietLogger()}, client, newFakeGatewayState(), clock.now)
	if err := benyBot.start(failingService{}); err != nil {
		t.Fatal(err)
	}
	request := core.RequestedCampaign{CampaignRequestID: uuid.New(), CampaignName: "Sky Isles", DisplayName: "Mira", DiscordUsername: "mira"}
	for range core.OwnerRequestPingsPerHour + 2 {
		benyBot.CampaignRequested(t.Context(), request)
		benyBot.posts.Wait()
	}
	if messages := client.messagesTo(directChannelID(ownerDiscordID)); len(messages) != core.OwnerRequestPingsPerHour {
		t.Fatalf("sent the owner %d messages in an hour, want %d", len(messages), core.OwnerRequestPingsPerHour)
	}
	clock.advance(pingWindow)
	benyBot.CampaignRequested(t.Context(), request)
	benyBot.posts.Wait()
	if messages := client.messagesTo(directChannelID(ownerDiscordID)); len(messages) != core.OwnerRequestPingsPerHour+1 {
		t.Fatalf("an hour later, sent the owner %d messages, want %d", len(messages), core.OwnerRequestPingsPerHour+1)
	}

	// In devauth builds without SITE_OWNER_DISCORD_ID the owner is seed:dm, who has no Discord account.
	seedOwnerClient := newFakeDiscordClient()
	seedOwnerBot := newBot(Options{Token: "test-token", SiteOwnerDiscordID: core.SeedDmDiscordID, Logger: quietLogger()}, seedOwnerClient, newFakeGatewayState(), clock.now)
	if err := seedOwnerBot.start(failingService{}); err != nil {
		t.Fatal(err)
	}
	seedOwnerBot.CampaignRequested(t.Context(), request)
	seedOwnerBot.posts.Wait()
	seedOwnerClient.mutex.Lock()
	defer seedOwnerClient.mutex.Unlock()
	if len(seedOwnerClient.directChannels) != 0 || len(seedOwnerClient.messages) != 0 {
		t.Error("Beny Bot tried to message seed:dm")
	}
}

func TestMessagesShowNamesLiterally(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		member   core.WaitingMember
		expected string
	}{
		{
			"plain names",
			core.WaitingMember{CampaignSlug: "sample-colony", DisplayName: "Thorin", DiscordUsername: "thorin.oak"},
			"Thorin (@thorin.oak) is waiting to join: http://localhost:5173/c/sample-colony/players",
		},
		{
			"underscores in usernames are not italics",
			core.WaitingMember{CampaignSlug: "sample-colony", DisplayName: "Ana", DiscordUsername: "ana_the_bold"},
			`Ana (@ana\_the\_bold) is waiting to join: http://localhost:5173/c/sample-colony/players`,
		},
		{
			"markdown, masked links and mentions are escaped",
			core.WaitingMember{CampaignSlug: "sky-isles", DisplayName: "# **[click](https://example.com)** <@&1>", DiscordUsername: "mallory"},
			`\# \*\*\[click\]\(https\://example.com\)\*\* \<@&1\> (@mallory) is waiting to join: http://localhost:5173/c/sky-isles/players`,
		},
		{
			"a bare link in a name is not clickable",
			core.WaitingMember{CampaignSlug: "sample-colony", DisplayName: "https://evil.example/players", DiscordUsername: "mallory"},
			`https\://evil.example/players (@mallory) is waiting to join: http://localhost:5173/c/sample-colony/players`,
		},
		{
			"emoji codes are shown as typed",
			core.WaitingMember{CampaignSlug: "sample-colony", DisplayName: "Ana :crown:", DiscordUsername: "ana"},
			`Ana \:crown\: (@ana) is waiting to join: http://localhost:5173/c/sample-colony/players`,
		},
		{
			"a missing display name falls back to the username",
			core.WaitingMember{CampaignSlug: "sample-colony", DiscordUsername: "brannoc"},
			"brannoc (@brannoc) is waiting to join: http://localhost:5173/c/sample-colony/players",
		},
		{
			"line breaks stay on one line",
			core.WaitingMember{CampaignSlug: "sample-colony", DisplayName: "Ana\n> quoted", DiscordUsername: "ana"},
			`Ana \> quoted (@ana) is waiting to join: http://localhost:5173/c/sample-colony/players`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			actual := waitingMessage(testCase.member, testPublicURL)
			if actual != testCase.expected {
				t.Errorf("got  %q\nwant %q", actual, testCase.expected)
			}
			if strings.Contains(actual, "\n") {
				t.Error("message spans several lines")
			}
		})
	}
	request := core.RequestedCampaign{CampaignName: "[Sky](https://evil.example)\nIsles", DiscordUsername: "mira_m"}
	if actual, expected := campaignRequestMessage(request, testPublicURL), `mira\_m (@mira\_m) asks to run \[Sky\]\(https\://evil.example\) Isles: http://localhost:5173/admin`; actual != expected {
		t.Errorf("got  %q\nwant %q", actual, expected)
	}
}
