package core_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

func TestCampaignRequestsAndOwnerApproval(t *testing.T) {
	test := newFixture(t)
	mira := test.caller(discordID(1), "mira")
	if _, err := test.service.ListCampaignRequests(test.context, mira); !isKind(err, core.KindForbidden) {
		t.Fatalf("a non-owner listed requests: %v", err)
	}
	for _, input := range []core.CampaignRequestInput{
		{CampaignName: "", Slug: "sky-isles"},
		{CampaignName: "Sky Isles", Slug: "Sky Isles"},
		{CampaignName: "Sky Isles", Slug: "ab"},
		{CampaignName: "Sky Isles", Slug: "sky--isles"},
		{CampaignName: "Sky Isles", Slug: "-sky"},
	} {
		if _, err := test.service.RequestCampaign(test.context, mira, input); !isKind(err, core.KindInvalid) {
			t.Errorf("request %+v: %v", input, err)
		}
	}
	request, err := test.service.RequestCampaign(test.context, mira, core.CampaignRequestInput{CampaignName: "Sky Isles", Slug: "sky-isles", Note: "Hi!"})
	if err != nil || request.Status != core.CampaignRequestStatusPending {
		t.Fatalf("request: %+v, %v", request, err)
	}
	if len(test.notifier.requests) != 1 || test.notifier.requests[0].CampaignRequestID != request.ID {
		t.Fatalf("the owner was not pinged: %+v", test.notifier.requests)
	}
	if _, err := test.service.RequestCampaign(test.context, mira, core.CampaignRequestInput{CampaignName: "Other", Slug: "other"}); !isKind(err, core.KindConflict) {
		t.Fatalf("a second open request: %v", err)
	}
	wren := test.caller(discordID(2), "wren")
	if _, err := test.service.RequestCampaign(test.context, wren, core.CampaignRequestInput{CampaignName: "Sky", Slug: "sky-isles"}); !isKind(err, core.KindInvalid) {
		t.Fatalf("a slug taken by an open request: %v", err)
	}
	wrenRequest, err := test.service.RequestCampaign(test.context, wren, core.CampaignRequestInput{CampaignName: "Sample Sky Isles", Slug: "sample-sky-isles"})
	if err != nil {
		t.Fatal(err)
	}

	// Deny; she may ask again.
	if _, err := test.service.DenyCampaignRequest(test.context, mira, wrenRequest.ID); !isKind(err, core.KindForbidden) {
		t.Fatalf("a non-owner denied: %v", err)
	}
	if _, err := test.service.DenyCampaignRequest(test.context, test.owner, wrenRequest.ID); err != nil {
		t.Fatal(err)
	}
	mine, err := test.service.MyCampaignRequest(test.context, wren)
	if err != nil || mine.Request == nil || mine.Request.Status != core.CampaignRequestStatusDenied {
		t.Fatalf("Wren's request = %+v, %v", mine, err)
	}
	if _, err := test.service.RequestCampaign(test.context, wren, core.CampaignRequestInput{CampaignName: "Again", Slug: "sample-sky-isles"}); err != nil {
		t.Fatalf("asking again after a denial: %v", err)
	}

	// Approval with an adjusted address creates the whole campaign.
	adjusted := "skies"
	approved, err := test.service.ApproveCampaignRequest(test.context, test.owner, request.ID, core.ApproveCampaignRequestInput{Slug: &adjusted})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Slug != "skies" || approved.Name != "Sky Isles" || approved.DmDiscordUsername != "mira" || approved.PlayerCount != 0 || approved.DiscordLinked {
		t.Fatalf("approved = %+v", approved)
	}
	if _, err := test.service.ApproveCampaignRequest(test.context, test.owner, request.ID, core.ApproveCampaignRequestInput{}); !isKind(err, core.KindConflict) {
		t.Fatalf("approving twice: %v", err)
	}
	dm := test.enter(mira, approved.ID)
	board, err := test.service.GetColonyBoard(test.context, dm)
	if err != nil || board.Day != 0 || board.ColonyName != "The Colony" || len(board.Resources) != 4 || len(board.Roles) != 9 || board.CampaignName != "Sky Isles" {
		t.Fatalf("a new campaign's board = %+v, %v", board, err)
	}
	mine, err = test.service.MyCampaignRequest(test.context, mira)
	if err != nil || mine.Request.Status != core.CampaignRequestStatusApproved || mine.Request.Campaign == nil || mine.Request.Campaign.Slug != "skies" {
		t.Fatalf("Mira's request after approval = %+v, %v", mine, err)
	}

	// The owner's own request is approved on submit; owning the site gives nothing in Mira's campaign.
	ownRequest, err := test.service.RequestCampaign(test.context, test.owner, core.CampaignRequestInput{CampaignName: "Owner's", Slug: "owners"})
	if err != nil || ownRequest.Status != core.CampaignRequestStatusApproved {
		t.Fatalf("the owner's request = %+v, %v", ownRequest, err)
	}
	if _, err := test.service.EnterCampaign(test.context, test.owner, approved.ID); !errors.Is(err, core.ErrNoSuchCampaign) {
		t.Fatalf("the owner entered a campaign they aren't in: %v", err)
	}
	if _, err := test.service.EnterCampaign(test.context, test.owner, uuid.New()); !errors.Is(err, core.ErrNoSuchCampaign) {
		t.Fatalf("an unknown campaign: %v", err)
	}
	list, err := test.service.ListCampaigns(test.context, test.owner)
	if err != nil || len(list.Campaigns) != 2 {
		t.Fatalf("owner's list = %+v, %v", list, err)
	}

	// Withdraw.
	withdrawn, err := test.service.RequestCampaign(test.context, test.caller(discordID(3), "nico"), core.CampaignRequestInput{CampaignName: "N", Slug: "nico"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.WithdrawCampaignRequest(test.context, mira, withdrawn.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("someone else withdrew Nico's request: %v", err)
	}
}

func TestInviteLinksAndJoining(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "Sample", "sample")
	code := test.inviteCode(dm)
	ana := test.caller(discordID(2), "ana")
	info, err := test.service.InviteInfo(test.context, ana, code)
	if err != nil || info.CampaignName != "Sample" || info.DmDisplayName != "dm" || info.Membership != nil || info.CampaignStatus != core.CampaignStatusActive {
		t.Fatalf("invite info = %+v, %v", info, err)
	}
	joined, err := test.service.JoinByInvite(test.context, ana, code)
	if err != nil || joined.Status != core.MemberStatusPending || joined.Role != core.CampaignRolePlayer {
		t.Fatalf("join = %+v, %v", joined, err)
	}
	if test.notifier.pings() != 1 || test.notifier.waiting[0].CampaignID != dm.CampaignID() {
		t.Fatalf("pings = %+v", test.notifier.waiting)
	}
	// The waiting page names who lets them in (docs/design/multi-campaign.md §5.1).
	waiting, err := test.service.CampaignMe(test.context, test.enter(ana, dm.CampaignID()))
	if err != nil || waiting.Status != core.MemberStatusPending || waiting.DmDisplayName != "dm" || waiting.Campaign.Name != "Sample" {
		t.Fatalf("the waiting member's me = %+v, %v", waiting, err)
	}
	again, err := test.service.JoinByInvite(test.context, ana, code)
	if err != nil || again.MemberID != joined.MemberID || test.notifier.pings() != 1 {
		t.Fatalf("joining twice: %+v, %v, pings %d", again, err, test.notifier.pings())
	}

	// A removed member opening the link stays removed and pings nobody.
	if _, err := test.service.DenyMember(test.context, dm, joined.MemberID); err != nil {
		t.Fatal(err)
	}
	removed, err := test.service.JoinByInvite(test.context, ana, code)
	if err != nil || removed.Status != core.MemberStatusRemoved || test.notifier.pings() != 1 {
		t.Fatalf("a removed member's join = %+v, %v, pings %d", removed, err, test.notifier.pings())
	}

	// Reset kills the old link at once.
	reset, err := test.service.ResetInviteLink(test.context, dm)
	if err != nil || reset.InviteCode == code {
		t.Fatalf("reset = %+v, %v", reset, err)
	}
	for _, dead := range []string{code, "not a code", ""} {
		if _, err := test.service.InviteInfo(test.context, ana, dead); !errors.Is(err, core.ErrInviteNotWorking) {
			t.Errorf("invite %q: %v", dead, err)
		}
		if _, err := test.service.JoinByInvite(test.context, ana, dead); !errors.Is(err, core.ErrInviteNotWorking) {
			t.Errorf("join %q: %v", dead, err)
		}
	}

	// A suspended campaign's link reads paused and refuses joins.
	if _, err := test.service.SuspendCampaign(test.context, test.owner, dm.CampaignID()); err != nil {
		t.Fatal(err)
	}
	quinn := test.caller(discordID(3), "quinn")
	if info, err := test.service.InviteInfo(test.context, quinn, reset.InviteCode); err != nil || info.CampaignStatus != core.CampaignStatusSuspended {
		t.Fatalf("a paused campaign's invite = %+v, %v", info, err)
	}
	if _, err := test.service.JoinByInvite(test.context, quinn, reset.InviteCode); !isKind(err, core.KindConflict) {
		t.Fatalf("joining a paused campaign: %v", err)
	}
	if campaigns, _ := test.service.MyCampaigns(test.context, quinn); len(campaigns.Campaigns) != 0 {
		t.Fatal("a refused join created a membership")
	}
}

// Decision C5: a player can leave, and only themselves.
func TestLeavingACampaign(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "Sample", "sample")
	anaCaller, ana := test.player(dm, discordID(2), "ana", true)
	_, bram := test.player(dm, discordID(3), "bram", false)

	if _, err := test.service.LeaveCampaign(test.context, dm); !isKind(err, core.KindForbidden) {
		t.Fatalf("the DM left: %v", err)
	}
	left, err := test.service.LeaveCampaign(test.context, ana)
	if err != nil || left.Status != core.MemberStatusRemoved || left.Member.Dm == nil || removalKind(left.Member) != core.RemovalKindLeft {
		t.Fatalf("leave from ACTIVE = %+v, %v", left, err)
	}
	if _, err := test.service.LeaveCampaign(test.context, bram); err != nil {
		t.Fatalf("leave from PENDING: %v", err)
	}
	if _, err := test.service.LeaveCampaign(test.context, test.enter(anaCaller, dm.CampaignID())); !isKind(err, core.KindConflict) {
		t.Fatalf("leave from REMOVED: %v", err)
	}
	if _, err := test.service.GetColonyBoard(test.context, test.enter(anaCaller, dm.CampaignID())); !isKind(err, core.KindForbidden) {
		t.Fatalf("a member who left still reads the board: %v", err)
	}
	pings := test.notifier.pings()
	rejoined, err := test.service.JoinByInvite(test.context, anaCaller, test.inviteCode(dm))
	if err != nil || rejoined.Status != core.MemberStatusRemoved || test.notifier.pings() != pings {
		t.Fatalf("the invite link after leaving = %+v, %v", rejoined, err)
	}
	groups, err := test.service.ListMembers(test.context, dm)
	if err != nil || len(groups.Removed) != 2 || removalKind(groups.Removed[0]) != core.RemovalKindLeft {
		t.Fatalf("players = %+v, %v", groups, err)
	}
	readmitted, err := test.service.ReadmitMember(test.context, dm, ana.MemberID())
	if err != nil || readmitted.Dm.Status != core.MemberStatusActive || readmitted.Dm.RemovalKind != nil {
		t.Fatalf("re-admit = %+v, %v", readmitted, err)
	}
}

// A refused /link leaves the code working; only the link that is made uses it up, and only once.
func TestRefusedLinksKeepTheCode(t *testing.T) {
	test := newFixture(t)
	_, dmA := test.campaign(test.caller(discordID(1), "dm-a"), "A", "campaign-a")
	_, dmB := test.campaign(test.caller(discordID(2), "dm-b"), "B", "campaign-b")
	const guildX = "900000000000000001"
	const guildY = "900000000000000002"
	const guildZ = "900000000000000003"
	link := func(code string, guildID string) error {
		_, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: code, GuildID: guildID, GuildName: "Test", DiscordUserID: discordID(1)})
		return err
	}
	linkedGuild := func(dm core.Viewer) string {
		t.Helper()
		settings, err := test.service.GetSettings(test.context, dm)
		if err != nil {
			t.Fatal(err)
		}
		if settings.DiscordServer == nil {
			return ""
		}
		return settings.DiscordServer.GuildID
	}
	codeB, _ := test.service.CreateLinkCode(test.context, dmB)
	if err := link(codeB.Code, guildX); err != nil {
		t.Fatal(err)
	}

	code, _ := test.service.CreateLinkCode(test.context, dmA)
	if err := link(code.Code, guildX); !errors.Is(err, core.ErrGuildAlreadyLinked) {
		t.Fatalf("a server linked elsewhere: %v", err)
	}
	if err := link(code.Code, "not a server"); !isKind(err, core.KindInvalid) {
		t.Fatalf("a malformed server: %v", err)
	}
	if err := link(code.Code, guildY); err != nil {
		t.Fatalf("the code after two refusals: %v", err)
	}
	if linkedGuild(dmA) != guildY || linkedGuild(dmB) != guildX {
		t.Fatalf("A is on %q and B on %q", linkedGuild(dmA), linkedGuild(dmB))
	}
	// Used once: unlinking doesn't bring the code back.
	if err := test.service.UnlinkGuild(test.context, dmA); err != nil {
		t.Fatal(err)
	}
	if err := link(code.Code, guildY); !errors.Is(err, core.ErrLinkCodeInvalid) {
		t.Fatalf("a used code worked again: %v", err)
	}

	// The campaign already linked: unlink the other server, then the same code works.
	if err := link(mustLinkCode(t, test, dmA), guildY); err != nil {
		t.Fatal(err)
	}
	second := mustLinkCode(t, test, dmA)
	if err := link(second, guildZ); !errors.Is(err, core.ErrCampaignAlreadyLinked) {
		t.Fatalf("a second server: %v", err)
	}
	if err := test.service.UnlinkGuild(test.context, dmA); err != nil {
		t.Fatal(err)
	}
	if err := link(second, guildZ); err != nil || linkedGuild(dmA) != guildZ {
		t.Fatalf("the same code after unlinking the other server: %v (linked to %q)", err, linkedGuild(dmA))
	}
	// A refused code still expires.
	third := mustLinkCode(t, test, dmA)
	if err := link(third, guildY); !errors.Is(err, core.ErrCampaignAlreadyLinked) {
		t.Fatal(err)
	}
	if err := test.service.UnlinkGuild(test.context, dmA); err != nil {
		t.Fatal(err)
	}
	test.clock.advance(core.LinkCodeLifetime)
	if err := link(third, guildY); !errors.Is(err, core.ErrLinkCodeInvalid) {
		t.Fatalf("an expired code: %v", err)
	}
}

func mustLinkCode(t *testing.T, test *fixture, dm core.Viewer) string {
	t.Helper()
	code, err := test.service.CreateLinkCode(test.context, dm)
	if err != nil {
		t.Fatal(err)
	}
	return code.Code
}

// Several /link runs with one code at once: exactly one links, whatever the servers.
func TestConcurrentLinksWithOneCode(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "A", "campaign-a")
	for round, sameServer := range []bool{false, true} {
		code := mustLinkCode(t, test, dm)
		const attempts = 6
		start := make(chan struct{})
		results := make(chan error, attempts)
		var waitGroup sync.WaitGroup
		for attempt := range attempts {
			guildID := fmt.Sprintf("9000000000000%02d%03d", round, attempt)
			if sameServer {
				guildID = fmt.Sprintf("9000000000000%02d999", round)
			}
			waitGroup.Go(func() {
				<-start
				_, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: code, GuildID: guildID, GuildName: "Test", DiscordUserID: discordID(1)})
				results <- err
			})
		}
		close(start)
		waitGroup.Wait()
		close(results)
		succeeded := 0
		for err := range results {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, core.ErrCampaignAlreadyLinked), errors.Is(err, core.ErrGuildAlreadyLinked), errors.Is(err, core.ErrLinkCodeInvalid):
			default:
				t.Errorf("round %d: an unexpected refusal: %v", round, err)
			}
		}
		if succeeded != 1 {
			t.Fatalf("round %d: %d links succeeded, want 1", round, succeeded)
		}
		if err := test.service.UnlinkGuild(test.context, dm); err != nil {
			t.Fatal(err)
		}
		if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: code, GuildID: "900000000000000777", GuildName: "Test"}); !errors.Is(err, core.ErrLinkCodeInvalid) {
			t.Fatalf("round %d: the code outlived its link: %v", round, err)
		}
	}
}

// Ready reconciliation never acts on an empty guild list; an explicit list still unlinks.
func TestReconcileIgnoresAnEmptyReady(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "A", "campaign-a")
	if unlinked, err := test.service.ReconcileGuilds(test.context, nil); err != nil || unlinked != 0 {
		t.Fatalf("nothing linked: %d, %v", unlinked, err)
	}
	const guildID = "900000000000000001"
	if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: mustLinkCode(t, test, dm), GuildID: guildID, GuildName: "Test"}); err != nil {
		t.Fatal(err)
	}
	for _, empty := range [][]string{nil, {}} {
		if unlinked, err := test.service.ReconcileGuilds(test.context, empty); err != nil || unlinked != 0 {
			t.Fatalf("an empty Ready (%#v): %d unlinked, %v", empty, unlinked, err)
		}
	}
	if _, err := test.service.ViewerForGuild(test.context, guildID, discordID(1)); err != nil {
		t.Fatalf("an empty Ready unlinked the server: %v", err)
	}
	if unlinked, err := test.service.ReconcileGuilds(test.context, []string{guildID}); err != nil || unlinked != 0 {
		t.Fatalf("a Ready that lists the server: %d, %v", unlinked, err)
	}
	if unlinked, err := test.service.ReconcileGuilds(test.context, []string{"900000000000000002"}); err != nil || unlinked != 1 {
		t.Fatalf("a Ready that lists only another server: %d, %v", unlinked, err)
	}
	if _, err := test.service.ViewerForGuild(test.context, guildID, discordID(1)); !errors.Is(err, core.ErrGuildNotLinked) {
		t.Fatalf("after a Ready without it: %v", err)
	}
}

// When Discord's channel list can't be read, saving Settings keeps the stored channels and refuses
// only a change.
func TestChannelsWhenDiscordCantBeRead(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "A", "campaign-a")
	const guildID = "900000000000000001"
	stored := "910000000000000001"
	other := "910000000000000002"
	test.directory.channels[guildID] = []core.DiscordChannel{{ID: stored, Name: "announcements"}, {ID: other, Name: "general"}}
	if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: mustLinkCode(t, test, dm), GuildID: guildID, GuildName: "Test"}); err != nil {
		t.Fatal(err)
	}
	input := core.SettingsInput{CampaignName: "A", ColonyName: "A", Timezone: "UTC", DiscordAnnounceChannelID: &stored}
	if _, err := test.service.UpdateSettings(test.context, dm, input); err != nil {
		t.Fatal(err)
	}

	test.directory.unreadable = true
	if options, err := test.service.DiscordChannelOptions(test.context, dm); err != nil || options == nil || len(options) != 0 {
		t.Fatalf("the picker while the list can't be read = %#v, %v; want an empty list", options, err)
	}
	input.Timezone = "Europe/Paris"
	settings, err := test.service.UpdateSettings(test.context, dm, input)
	if err != nil || settings.Timezone != "Europe/Paris" || settings.DiscordAnnounceChannelID == nil || *settings.DiscordAnnounceChannelID != stored {
		t.Fatalf("saving with the stored channel unchanged = %+v, %v", settings, err)
	}
	input.DiscordReminderChannelID = &other
	_, err = test.service.UpdateSettings(test.context, dm, input)
	operationError, typed := core.AsError(err)
	if !typed || operationError.Kind != core.KindInvalid || len(operationError.Fields) != 1 || operationError.Fields[0].Field != "discordReminderChannelId" ||
		operationError.Fields[0].Message != "Discord's channel list couldn't be read right now. Try again in a moment." {
		t.Fatalf("a new channel while the list can't be read: %+v", err)
	}
	// Clearing a channel needs no list.
	input.DiscordReminderChannelID = nil
	input.DiscordAnnounceChannelID = nil
	if settings, err := test.service.UpdateSettings(test.context, dm, input); err != nil || settings.DiscordAnnounceChannelID != nil {
		t.Fatalf("clearing a channel = %+v, %v", settings, err)
	}

	test.directory.unreadable = false
	input.DiscordReminderChannelID = &other
	if settings, err := test.service.UpdateSettings(test.context, dm, input); err != nil || settings.DiscordReminderChannelID == nil {
		t.Fatalf("the change once the list reads again = %+v, %v", settings, err)
	}
}

// removalKind is the label /players shows for a member, or "" when they aren't removed.
func removalKind(member core.Member) core.RemovalKind {
	if member.Dm == nil || member.Dm.RemovalKind == nil {
		return ""
	}
	return *member.Dm.RemovalKind
}

// /players tells a denial from a removal and from leaving: core decides, the web only shows it.
func TestRemovalKinds(t *testing.T) {
	test := newFixture(t)
	_, dm := test.campaign(test.caller(discordID(1), "dm"), "Sample", "sample")
	_, denied := test.player(dm, discordID(2), "denied", false)
	_, removedWhileWaiting := test.player(dm, discordID(3), "removed-while-waiting", false)
	_, removed := test.player(dm, discordID(4), "removed", true)
	_, leaver := test.player(dm, discordID(5), "leaver", false)
	_, deniedThenReadmitted := test.player(dm, discordID(6), "readmitted", false)
	_, playing := test.player(dm, discordID(7), "playing", true)
	_, waiting := test.player(dm, discordID(8), "waiting", false)

	steps := []struct {
		name   string
		action func() (core.Member, error)
	}{
		{"deny", func() (core.Member, error) { return test.service.DenyMember(test.context, dm, denied.MemberID()) }},
		{"remove a waiting member", func() (core.Member, error) {
			return test.service.RemoveMember(test.context, dm, removedWhileWaiting.MemberID())
		}},
		{"remove a player", func() (core.Member, error) { return test.service.RemoveMember(test.context, dm, removed.MemberID()) }},
		{"deny before re-admitting", func() (core.Member, error) {
			return test.service.DenyMember(test.context, dm, deniedThenReadmitted.MemberID())
		}},
		{"re-admit", func() (core.Member, error) {
			return test.service.ReadmitMember(test.context, dm, deniedThenReadmitted.MemberID())
		}},
		{"remove the re-admitted", func() (core.Member, error) {
			return test.service.RemoveMember(test.context, dm, deniedThenReadmitted.MemberID())
		}},
	}
	for _, step := range steps {
		if _, err := step.action(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	if _, err := test.service.LeaveCampaign(test.context, leaver); err != nil {
		t.Fatalf("leave: %v", err)
	}

	groups, err := test.service.ListMembers(test.context, dm)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[uuid.UUID]core.RemovalKind{
		denied.MemberID():               core.RemovalKindDenied,
		removedWhileWaiting.MemberID():  core.RemovalKindDenied,
		removed.MemberID():              core.RemovalKindRemoved,
		leaver.MemberID():               core.RemovalKindLeft,
		deniedThenReadmitted.MemberID(): core.RemovalKindRemoved,
		playing.MemberID():              "",
		waiting.MemberID():              "",
		dm.MemberID():                   "",
	}
	seen := 0
	for _, group := range [][]core.Member{groups.Pending, groups.Active, groups.Removed} {
		for _, member := range group {
			want, listed := expected[member.ID]
			if !listed {
				t.Errorf("an unexpected member %s", member.DisplayName)
				continue
			}
			seen++
			if got := removalKind(member); got != want {
				t.Errorf("%s: removal kind %q, want %q", member.DisplayName, got, want)
			}
		}
	}
	if seen != len(expected) {
		t.Errorf("saw %d members, want %d", seen, len(expected))
	}
}

// Decision C3 and the owner's suspend and delete.
func TestDeletingAndSuspendingCampaigns(t *testing.T) {
	test := newFixture(t)
	dmCaller := test.caller(discordID(1), "dm")
	campaignID, dm := test.campaign(dmCaller, "Sample", "sample")
	_, player := test.player(dm, discordID(2), "ana", true)
	otherID, otherDm := test.campaign(test.caller(discordID(3), "hollis"), "Second", "second")
	if _, err := test.service.Advance(test.context, otherDm, core.AdvanceInput{Days: 3}); err != nil {
		t.Fatal(err)
	}
	stream, _, err := test.service.Subscribe(player, "")
	if err != nil {
		t.Fatal(err)
	}

	// Suspension closes the campaign but keeps the pending-level reads.
	if _, err := test.service.SuspendCampaign(test.context, dmCaller, campaignID); !isKind(err, core.KindForbidden) {
		t.Fatalf("a DM suspended: %v", err)
	}
	if _, err := test.service.SuspendCampaign(test.context, test.owner, campaignID); err != nil {
		t.Fatal(err)
	}
	paused := test.enter(dmCaller, campaignID)
	if _, err := test.service.GetColonyBoard(test.context, paused); !errors.Is(err, core.ErrCampaignSuspended) {
		t.Fatalf("a suspended campaign's board: %v", err)
	}
	if me, err := test.service.CampaignMe(test.context, paused); err != nil || me.Campaign.Status != core.CampaignStatusSuspended {
		t.Fatalf("a suspended campaign's me = %+v, %v", me, err)
	}
	if _, err := test.service.CreateLinkCode(test.context, paused); !errors.Is(err, core.ErrCampaignSuspended) {
		t.Fatalf("a link code for a suspended campaign: %v", err)
	}
	if _, err := test.service.UnsuspendCampaign(test.context, test.owner, campaignID); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.GetColonyBoard(test.context, test.enter(dmCaller, campaignID)); err != nil {
		t.Fatalf("an unsuspended campaign: %v", err)
	}

	// The owner deletes only a suspended campaign; the DM deletes their own at any time.
	if err := test.service.DeleteCampaign(test.context, test.owner, campaignID, core.DeleteCampaignInput{ConfirmSlug: "sample"}); !isKind(err, core.KindConflict) {
		t.Fatalf("the owner deleted an active campaign: %v", err)
	}
	if err := test.service.DeleteOwnCampaign(test.context, player, core.DeleteCampaignInput{ConfirmSlug: "sample"}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player deleted the campaign: %v", err)
	}
	if err := test.service.DeleteOwnCampaign(test.context, otherDm, core.DeleteCampaignInput{ConfirmSlug: "sample"}); !isKind(err, core.KindInvalid) {
		t.Fatalf("another campaign's DM deleted with this slug: %v", err)
	}
	if err := test.service.DeleteOwnCampaign(test.context, dm, core.DeleteCampaignInput{ConfirmSlug: "Sample"}); !isKind(err, core.KindInvalid) {
		t.Fatalf("a wrong confirmation: %v", err)
	}
	if err := test.service.DeleteOwnCampaign(test.context, dm, core.DeleteCampaignInput{ConfirmSlug: "sample"}); err != nil {
		t.Fatalf("the DM's delete: %v", err)
	}
	select {
	case <-stream.Closed():
	default:
		t.Fatal("deleting the campaign left its stream open")
	}
	if _, err := test.service.EnterCampaign(test.context, dmCaller, campaignID); !errors.Is(err, core.ErrNoSuchCampaign) {
		t.Fatalf("a deleted campaign: %v", err)
	}
	if _, err := test.service.GetColonyBoard(test.context, dm); !errors.Is(err, core.ErrNoSuchCampaign) {
		t.Fatalf("a Viewer read before the deletion: %v", err)
	}
	for _, table := range []string{"colony", "settings", "resource", "role", "campaign_member", "timeline_event"} {
		var count int64
		if err := test.database.Pool.QueryRow(test.context, `SELECT count(*) FROM `+table+` WHERE campaign_id = $1`, campaignID).Scan(&count); err != nil || count != 0 {
			t.Errorf("%s kept %d rows of the deleted campaign (%v)", table, count, err)
		}
	}
	board, err := test.service.GetColonyBoard(test.context, otherDm)
	if err != nil || board.Day != 3 {
		t.Fatalf("the other campaign changed: %+v, %v", board, err)
	}
	if _, err := test.service.MyCampaigns(test.context, dmCaller); err != nil {
		t.Fatalf("the DM's account went with the campaign: %v", err)
	}

	// The owner's path: suspend, then delete with the slug.
	if _, err := test.service.SuspendCampaign(test.context, test.owner, otherID); err != nil {
		t.Fatal(err)
	}
	if err := test.service.DeleteCampaign(test.context, test.owner, otherID, core.DeleteCampaignInput{ConfirmSlug: "second"}); err != nil {
		t.Fatalf("the owner's delete: %v", err)
	}
}

// IDs and keys from request bodies and jsonb are loaded with the Viewer's campaign (§2.3 rule 5):
// campaign A's DM using campaign B's real IDs and keys gets a 4xx, and B is unchanged.
func TestBodyIDsAndKeysStayInTheirCampaign(t *testing.T) {
	test := newFixture(t)
	_, dmA := test.campaign(test.caller(discordID(1), "dm-a"), "A", "campaign-a")
	campaignB, dmB := test.campaign(test.caller(discordID(2), "dm-b"), "B", "campaign-b")
	brine, err := test.service.CreateResource(test.context, dmB, core.NewResourceInput{Label: "Brine", Icon: core.ColonyIconDroplets})
	if err != nil {
		t.Fatal(err)
	}
	board := func(dm core.Viewer) core.ColonyBoard {
		t.Helper()
		read, err := test.service.GetColonyBoard(test.context, dm)
		if err != nil {
			t.Fatal(err)
		}
		return read
	}
	if _, err := test.service.SetShipmentSchedule(test.context, dmB, core.ShipmentScheduleInput{ExpectedVersion: board(dmB).Version, IntervalDays: 1}); err != nil {
		t.Fatal(err)
	}
	advanced, err := test.service.Advance(test.context, dmB, core.AdvanceInput{Days: 1, ExpectedVersion: board(dmB).Version})
	if err != nil {
		t.Fatal(err)
	}
	arrivalB := advanced.Entries[0]
	if arrivalB.Kind != core.TimelineKindShipmentArrived {
		t.Fatalf("B's first entry is %s", arrivalB.Kind)
	}
	before := board(dmB)
	timelineBefore, _ := test.service.ListTimeline(test.context, dmB, core.TimelineRangeInput{FromDay: 0, ToDay: 10})

	refusals := map[string]error{}
	_, refusals["cargo on B's arrival"] = test.service.AdjustStock(test.context, dmA, core.AdjustInput{
		ExpectedVersion: board(dmA).Version, Source: core.AdjustSourceShipment, ArrivalEntryID: &arrivalB.ID,
		Deltas: map[string]int64{"food": 1}, SetTo: map[string]int64{},
	})
	_, refusals["B-only resource key in deltas"] = test.service.AdjustStock(test.context, dmA, core.AdjustInput{
		ExpectedVersion: board(dmA).Version, Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{brine.Key: 5}, SetTo: map[string]int64{},
	})
	_, refusals["B-only resource key in setTo"] = test.service.PreviewAdjustStock(test.context, dmA, core.AdjustInput{
		Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{}, SetTo: map[string]int64{brine.Key: 5},
	})
	_, refusals["B's undo group"] = test.service.UndoLastChange(test.context, dmA, core.UndoInput{GroupID: advanced.GroupID})
	_, refusals["B's resource by key"] = test.service.UpdateResource(test.context, dmA, brine.Key, core.ResourceInput{Label: "Mine", Icon: core.ColonyIconDroplets})
	_, refusals["B's timeline entry"] = test.service.UpdateTimelineEntry(test.context, dmA, arrivalB.ID, core.TimelineEntryInput{Day: arrivalB.Day, Title: "Mine", Visibility: core.TimelineVisibilityPublic})
	refusals["delete B's timeline entry"] = test.service.DeleteCustomEntry(test.context, dmA, arrivalB.ID)
	_, refusals["B's member"] = test.service.RemoveMember(test.context, dmA, dmB.MemberID())
	_, refusals["B's member's character"] = test.service.SetCharacterName(test.context, dmA, dmB.MemberID(), core.CharacterNameInput{})
	for name, err := range refusals {
		operationError, typed := core.AsError(err)
		if !typed || (operationError.Kind != core.KindNotFound && operationError.Kind != core.KindConflict && operationError.Kind != core.KindInvalid) {
			t.Errorf("%s: %v, want a 404, 409 or 422", name, err)
		}
	}
	after := board(dmB)
	if after.Version != before.Version || len(after.Resources) != len(before.Resources) || after.Day != before.Day {
		t.Fatalf("B changed: %+v → %+v", before, after)
	}
	timelineAfter, _ := test.service.ListTimeline(test.context, dmB, core.TimelineRangeInput{FromDay: 0, ToDay: 10})
	if len(timelineAfter.Entries) != len(timelineBefore.Entries) || timelineAfter.Entries[0].Dm.UndoneAt != nil {
		t.Fatal("B's timeline changed")
	}
	var memberStatus string
	if err := test.database.Pool.QueryRow(test.context, `SELECT status FROM campaign_member WHERE campaign_id = $1 AND id = $2`, campaignB, dmB.MemberID()).Scan(&memberStatus); err != nil || memberStatus != "ACTIVE" {
		t.Fatalf("B's DM = %s, %v", memberStatus, err)
	}
	// Each campaign has its own gold.
	if _, err := test.service.AdjustStock(test.context, dmA, core.AdjustInput{
		ExpectedVersion: board(dmA).Version, Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"gold": 7}, SetTo: map[string]int64{},
	}); err != nil {
		t.Fatal(err)
	}
	for _, resource := range board(dmB).Resources {
		if resource.Key == "gold" && resource.Amount != 0 {
			t.Fatalf("A's adjustment changed B's gold to %d", resource.Amount)
		}
	}
}

// One person, two campaigns: roles are judged per campaign, and the site owner flag is not a role.
func TestRolesAreJudgedPerCampaign(t *testing.T) {
	test := newFixture(t)
	dmACaller := test.caller(discordID(1), "dm-a")
	_, dmA := test.campaign(dmACaller, "A", "campaign-a")
	dmBCaller := test.caller(discordID(2), "dm-b")
	_, dmB := test.campaign(dmBCaller, "B", "campaign-b")
	dmBInA := test.join(dmBCaller, dmA, true)
	dmAInB := test.join(dmACaller, dmB, true)
	for name, viewer := range map[string]core.Viewer{"B's DM in A": dmBInA, "A's DM in B": dmAInB} {
		if viewer.IsDM() || viewer.SeesDmFields() {
			t.Errorf("%s is a DM there", name)
		}
		if _, err := test.service.ListMembers(test.context, viewer); !isKind(err, core.KindForbidden) {
			t.Errorf("%s listed members: %v", name, err)
		}
		if _, err := test.service.GetColonyBoard(test.context, viewer); err != nil {
			t.Errorf("%s can't read the board: %v", name, err)
		}
	}
	// View as player is per session and applies only where the caller is DM.
	viewing := dmACaller
	viewing.ViewAsPlayer = true
	if test.enter(viewing, dmA.CampaignID()).SeesDmFields() || test.enter(viewing, dmB.CampaignID()).ViewAsPlayer() {
		t.Fatal("view as player leaked across campaigns")
	}
	campaigns, err := test.service.MyCampaigns(test.context, dmACaller)
	if err != nil || len(campaigns.Campaigns) != 2 {
		t.Fatalf("A's DM's campaigns = %+v, %v", campaigns, err)
	}
	// Owner status changes nothing inside a campaign.
	ownerFlagged := dmBCaller
	ownerFlagged.IsSiteOwner = true
	if test.enter(ownerFlagged, dmA.CampaignID()).IsDM() {
		t.Fatal("the owner flag made a player a DM")
	}
}

func TestDiscordLinking(t *testing.T) {
	test := newFixture(t)
	_, dmA := test.campaign(test.caller(discordID(1), "dm-a"), "A", "campaign-a")
	_, dmB := test.campaign(test.caller(discordID(2), "dm-b"), "B", "campaign-b")
	const guildID = "900000000000000001"
	const otherGuildID = "900000000000000002"
	test.directory.channels[guildID] = []core.DiscordChannel{{ID: "910000000000000001", Name: "dm-only"}}

	if _, err := test.service.ViewerForGuild(test.context, guildID, discordID(1)); !errors.Is(err, core.ErrGuildNotLinked) {
		t.Fatalf("an unlinked server: %v", err)
	}
	code, err := test.service.CreateLinkCode(test.context, dmA)
	if err != nil || len(code.Code) != len("BENY-XXXXX-XXXXX") {
		t.Fatalf("link code = %+v, %v", code, err)
	}
	if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: "BENY-22222-22222", GuildID: guildID, GuildName: "Test"}); !errors.Is(err, core.ErrLinkCodeInvalid) {
		t.Fatalf("a wrong code: %v", err)
	}
	linked, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: " beny-" + code.Code[5:], GuildID: guildID, GuildName: "Test", DiscordUserID: discordID(1)})
	if err != nil || linked.CampaignName != "A" {
		t.Fatalf("link = %+v, %v", linked, err)
	}
	if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: code.Code, GuildID: otherGuildID, GuildName: "Test"}); !errors.Is(err, core.ErrLinkCodeInvalid) {
		t.Fatalf("a used code worked again: %v", err)
	}
	codeB, _ := test.service.CreateLinkCode(test.context, dmB)
	if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: codeB.Code, GuildID: guildID, GuildName: "Test"}); !errors.Is(err, core.ErrGuildAlreadyLinked) {
		t.Fatalf("a second campaign linked the same server: %v", err)
	}
	expired, _ := test.service.CreateLinkCode(test.context, dmB)
	test.clock.advance(core.LinkCodeLifetime)
	if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: expired.Code, GuildID: otherGuildID, GuildName: "Test"}); !errors.Is(err, core.ErrLinkCodeInvalid) {
		t.Fatalf("an expired code: %v", err)
	}

	// Routing: members, outsiders and the waiting.
	if viewer, err := test.service.ViewerForGuild(test.context, guildID, discordID(1)); err != nil || !viewer.IsDM() || viewer.CampaignID() != dmA.CampaignID() {
		t.Fatalf("A's DM in A's server: %v", err)
	}
	if _, err := test.service.ViewerForGuild(test.context, guildID, discordID(2)); !errors.Is(err, core.ErrNotInCampaign) {
		t.Fatalf("B's DM in A's server: %v", err)
	}
	if _, err := test.service.ViewerForGuild(test.context, guildID, "999999999999999999"); !errors.Is(err, core.ErrNotInCampaign) {
		t.Fatalf("a stranger: %v", err)
	}
	test.player(dmA, discordID(3), "waiting", false)
	if _, err := test.service.ViewerForGuild(test.context, guildID, discordID(3)); !errors.Is(err, core.ErrNotLetIn) {
		t.Fatalf("a waiting member: %v", err)
	}

	// Channels come from the linked server only.
	foreign := "920000000000000001"
	own := "910000000000000001"
	_, err = test.service.UpdateSettings(test.context, dmA, core.SettingsInput{CampaignName: "A", ColonyName: "A", Timezone: "UTC", DiscordDmChannelID: &foreign})
	if !isKind(err, core.KindInvalid) {
		t.Fatalf("a channel of another server: %v", err)
	}
	settings, err := test.service.UpdateSettings(test.context, dmA, core.SettingsInput{CampaignName: "A", ColonyName: "A", Timezone: "UTC", DiscordDmChannelID: &own})
	if err != nil || settings.DiscordServer == nil || settings.DiscordServer.GuildID != guildID {
		t.Fatalf("settings = %+v, %v", settings, err)
	}
	target, found, err := test.service.DiscordTargetFor(test.context, dmA.CampaignID())
	if err != nil || !found || target.GuildID != guildID || target.DmChannelID == nil || *target.DmChannelID != own {
		t.Fatalf("target = %+v, %v, %v", target, found, err)
	}

	// Suspension pauses the bot there; Ready reconciliation unlinks a server the bot left.
	if _, err := test.service.SuspendCampaign(test.context, test.owner, dmA.CampaignID()); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.ViewerForGuild(test.context, guildID, discordID(1)); !errors.Is(err, core.ErrCampaignSuspended) {
		t.Fatalf("a suspended campaign's server: %v", err)
	}
	if _, found, _ := test.service.DiscordTargetFor(test.context, dmA.CampaignID()); found {
		t.Fatal("a suspended campaign still has a posting target")
	}
	if _, err := test.service.UnsuspendCampaign(test.context, test.owner, dmA.CampaignID()); err != nil {
		t.Fatal(err)
	}
	unlinked, err := test.service.ReconcileGuilds(test.context, []string{otherGuildID})
	if err != nil || unlinked != 1 {
		t.Fatalf("reconcile = %d, %v", unlinked, err)
	}
	settings, err = test.service.GetSettings(test.context, dmA)
	if err != nil || settings.DiscordServer != nil || settings.DiscordDmChannelID != nil {
		t.Fatalf("after the bot left: %+v, %v", settings, err)
	}
	if removed, err := test.service.UnlinkGuildByID(test.context, guildID); err != nil || removed {
		t.Fatalf("unlinking an unlinked server: %v, %v", removed, err)
	}
}
