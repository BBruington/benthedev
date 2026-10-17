package core_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/BBruington/colony-planner/internal/core"
)

func TestConcurrentJoinsCannotPassThePendingCap(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "Sample", "sample")
	for index := range core.PendingMemberLimit - 1 {
		test.player(dm, discordID(100+index), fmt.Sprintf("waiting%d", index), false)
	}
	pingsBefore := test.notifier.pings()
	inviteCode := test.inviteCode(dm)
	racers := []core.Caller{test.caller(discordID(200), "racer0"), test.caller(discordID(201), "racer1")}
	results := make(chan error, len(racers))
	for _, racer := range racers {
		go func() {
			_, err := test.service.JoinByInvite(test.context, racer, inviteCode)
			results <- err
		}()
	}
	var admitted, refused int
	for range racers {
		err := <-results
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, core.ErrJoinsFull):
			refused++
		default:
			t.Fatalf("join: %v", err)
		}
	}
	if admitted != 1 || refused != 1 {
		t.Fatalf("admitted %d and refused %d; the cap should let exactly one in", admitted, refused)
	}
	if test.notifier.pings()-pingsBefore != 1 {
		t.Fatalf("pings = %d, want 1", test.notifier.pings()-pingsBefore)
	}
	// The cap is per campaign: another campaign still takes joiners.
	_, otherDm := test.campaign(test.caller(discordID(2), "other-dm"), "Other", "other")
	test.player(otherDm, discordID(300), "elsewhere", false)
}

func TestSignInOnlyCreatesTheAccount(t *testing.T) {
	test := newFixture(t)
	if _, err := test.service.SignIn(test.context, core.DiscordIdentity{UserID: " ", Username: "x"}); err == nil {
		t.Fatal("a sign-in without a Discord ID was accepted")
	}
	first := test.caller(discordID(1), "ana")
	again := test.caller(discordID(1), "ana-renamed")
	if first.AccountID != again.AccountID {
		t.Fatal("signing in again made a second account")
	}
	campaigns, err := test.service.MyCampaigns(test.context, first)
	if err != nil || len(campaigns.Campaigns) != 0 {
		t.Fatalf("a first sign-in joined something: %+v, %v", campaigns, err)
	}
	if test.notifier.pings() != 0 {
		t.Fatal("a sign-in pinged someone")
	}
	me, err := test.service.AccountMe(test.context, again)
	if err != nil || me.DiscordUsername != "ana-renamed" || me.IsSiteOwner {
		t.Fatalf("account me = %+v, %v", me, err)
	}
	ownerMe, err := test.service.AccountMe(test.context, test.owner)
	if err != nil || !ownerMe.IsSiteOwner {
		t.Fatalf("the owner's account me = %+v, %v", ownerMe, err)
	}
}

func TestOperationsRefusePlayersAndViewAsPlayer(t *testing.T) {
	test := newFixture(t)
	dmCaller := test.caller(discordID(1), "dm")
	campaignID, dm := test.campaign(dmCaller, "Sample", "sample")
	if !dm.IsDM() || !dm.IsActive() || dm.CampaignID() != campaignID {
		t.Fatalf("the DM's viewer is wrong: dm=%v active=%v", dm.IsDM(), dm.IsActive())
	}
	viewingAsPlayer := dmCaller
	viewingAsPlayer.ViewAsPlayer = true
	dmAsPlayer := test.enter(viewingAsPlayer, campaignID)
	_, pending := test.player(dm, discordID(2), "ana", false)

	for name, viewer := range map[string]core.Viewer{"a pending member": pending, "the DM viewing as player": dmAsPlayer, "the zero Viewer": {}} {
		if _, err := test.service.ListMembers(test.context, viewer); err == nil {
			t.Errorf("%s listed members", name)
		}
		if _, err := test.service.ApproveMember(test.context, viewer, pending.MemberID()); err == nil {
			t.Errorf("%s approved a member", name)
		}
		if _, err := test.service.GetSettings(test.context, viewer); err == nil {
			t.Errorf("%s read settings", name)
		}
		if _, err := test.service.Invite(test.context, viewer); err == nil {
			t.Errorf("%s read the invite code", name)
		}
	}
	if _, err := test.service.UpdateProfile(test.context, pending, core.ProfileInput{}); !isKind(err, core.KindForbidden) {
		t.Errorf("a pending member edited their profile: %v", err)
	}
	if _, err := test.service.ApproveMember(test.context, dm, pending.MemberID()); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := test.service.ApproveMember(test.context, dm, pending.MemberID()); err != nil {
		t.Fatalf("approving twice: %v", err)
	}
	if _, err := test.service.RemoveMember(test.context, dm, dm.MemberID()); !isKind(err, core.KindForbidden) {
		t.Fatalf("the DM removed themselves: %v", err)
	}
	if len(test.evictor.accounts) != 0 {
		t.Fatalf("membership changes evicted sessions: %v", test.evictor.accounts)
	}
}

func TestUpdateSettingsValidates(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "Sample", "sample")

	for _, timezone := range []string{"Mars/Olympus", "Local", ""} {
		_, err := test.service.UpdateSettings(test.context, dm, core.SettingsInput{CampaignName: "Sample", ColonyName: "Sample", Timezone: timezone})
		operationError, typed := core.AsError(err)
		if !typed || operationError.Kind != core.KindInvalid || len(operationError.Fields) != 1 || operationError.Fields[0].Field != "timezone" {
			t.Errorf("timezone %q: %v", timezone, err)
		}
	}

	blank := "  "
	channel := "123456789012345678"
	_, err := test.service.UpdateSettings(test.context, dm, core.SettingsInput{
		CampaignName: "Sample", ColonyName: "Sample", Timezone: "UTC", DiscordDmChannelID: &channel,
	})
	if operationError, typed := core.AsError(err); !typed || len(operationError.Fields) != 1 || !strings.Contains(operationError.Fields[0].Message, "Link a Discord server first") {
		t.Fatalf("a channel without a linked server: %v", err)
	}
	settings, err := test.service.UpdateSettings(test.context, dm, core.SettingsInput{
		CampaignName:             "  Sample Colony ",
		ColonyName:               " Port Eldari  ",
		Timezone:                 "America/New_York",
		DiscordAnnounceChannelID: &blank,
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if settings.CampaignName != "Sample Colony" || settings.ColonyName != "Port Eldari" || settings.DiscordAnnounceChannelID != nil || settings.DiscordServer != nil {
		t.Fatalf("settings = %+v", settings)
	}
	board, err := test.service.GetColonyBoard(test.context, dm)
	if err != nil || board.CampaignName != "Sample Colony" || board.ColonyName != "Port Eldari" {
		t.Fatalf("the board's names = %+v, %v", board, err)
	}
}

func TestSettingsShowBenyBotsInviteLinkOnlyWhenConfigured(t *testing.T) {
	test := newFixture(t)
	dmCaller := test.caller(discordID(1), "dm")
	campaignID, _ := test.campaign(dmCaller, "Sample", "sample")
	inviteURL := "https://discord.com/oauth2/authorize?client_id=123456789012345678&scope=bot+applications.commands&permissions=8590019584"
	for _, configured := range []string{"", inviteURL} {
		service := core.New(core.Options{Pool: test.database.Pool, SiteOwnerDiscordID: testOwnerDiscordID, BotInviteURL: configured})
		dm, err := service.EnterCampaign(test.context, dmCaller, campaignID)
		if err != nil {
			t.Fatal(err)
		}
		settings, err := service.GetSettings(test.context, dm)
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}
		if configured == "" && settings.BotInviteURL != nil {
			t.Fatalf("an invite link without DISCORD_CLIENT_ID: %q", *settings.BotInviteURL)
		}
		if configured != "" && (settings.BotInviteURL == nil || *settings.BotInviteURL != configured) {
			t.Fatalf("invite link = %v, want %q", settings.BotInviteURL, configured)
		}
	}
}

// Free text is checked in core: a NUL byte would otherwise reach Postgres and come back as a 500, and
// lengths count after trimming.
func TestFreeTextRefusesControlCharactersAndCountsAfterTrimming(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "Sample", "sample")

	expectFieldError := func(label string, err error, field string) {
		t.Helper()
		operationError, typed := core.AsError(err)
		if !typed || operationError.Kind != core.KindInvalid || len(operationError.Fields) != 1 || operationError.Fields[0].Field != field {
			t.Errorf("%s: %v, want one invalid %s field", label, err, field)
		}
	}

	for _, characterName := range []string{"x\x00y", "line\nbreak", "tab\there", strings.Repeat("a", 61)} {
		_, err := test.service.SetCharacterName(test.context, dm, dm.MemberID(), core.CharacterNameInput{CharacterName: &characterName})
		expectFieldError(fmt.Sprintf("character name %q", characterName), err, "characterName")
	}
	padded := "  " + strings.Repeat("a", 60) + "  "
	if member, err := test.service.SetCharacterName(test.context, dm, dm.MemberID(), core.CharacterNameInput{CharacterName: &padded}); err != nil || member.CharacterName == nil || *member.CharacterName != strings.Repeat("a", 60) {
		t.Errorf("a 60-character name with spaces around it: %+v, %v", member, err)
	}

	_, err := test.service.UpdateSettings(test.context, dm, core.SettingsInput{CampaignName: "Bad\x00Name", ColonyName: "Sample", Timezone: "UTC"})
	expectFieldError("campaign name with NUL", err, "campaignName")
	_, err = test.service.UpdateSettings(test.context, dm, core.SettingsInput{CampaignName: "   ", ColonyName: "Sample", Timezone: "UTC"})
	expectFieldError("blank campaign name", err, "campaignName")
	for _, channel := range []string{"12345", "12345678901234567a", "123456789012345678901"} {
		_, err = test.service.UpdateSettings(test.context, dm, core.SettingsInput{CampaignName: "Sample", ColonyName: "Sample", Timezone: "UTC", DiscordReminderChannelID: &channel})
		expectFieldError(fmt.Sprintf("channel %q", channel), err, "discordReminderChannelId")
	}

	_, err = test.service.SignIn(test.context, core.DiscordIdentity{UserID: "100000000000000099", Username: "nul", DisplayName: "Nul\x00Name"})
	expectFieldError("sign-in display name with NUL", err, "displayName")
}
