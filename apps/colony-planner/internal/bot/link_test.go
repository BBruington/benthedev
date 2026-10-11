package bot

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

const administrator = int64(discordgo.PermissionAdministrator)

// linkedServer is the DM's Settings card for the campaign: the linked server's ID and name, or empty.
func (test *world) linkedServer(dm core.Viewer) (string, string) {
	test.t.Helper()
	settings, err := test.service.GetSettings(test.context, dm)
	if err != nil {
		test.t.Fatalf("settings: %v", err)
	}
	if settings.DiscordServer == nil {
		return "", ""
	}
	return settings.DiscordServer.GuildID, settings.DiscordServer.GuildName
}

func TestLinkConnectsTheServerToTheCodesCampaign(t *testing.T) {
	test := newWorld(t)
	code := test.linkCode(test.dmA)
	// The code works however it is typed: lower case, spaced, without its BENY- prefix.
	typed := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(code, "BENY-"), "-", " "))

	reply := test.run(serverCommand(guildX, strangerUserID, "link", manageServer, codeOption(typed)))
	if expected := "Linked this server to **Sample Colony**. Pick channels in the campaign's Settings."; reply != expected {
		t.Fatalf("reply = %q, want %q", reply, expected)
	}
	if guildID, guildName := test.linkedServer(test.dmA); guildID != guildX || guildName != "Colony Planner Test" {
		t.Errorf("Settings shows %q (%q), want guild X by its gateway name", guildID, guildName)
	}
	if reply := test.run(slashCommand(guildX, activeUserID, "beny")); reply != "Boy!" {
		t.Errorf("after /link, /beny answered %q", reply)
	}
}

func TestLinkNeedsManageServer(t *testing.T) {
	test := newWorld(t)
	code := test.linkCode(test.dmA)

	// The DM of the campaign is no exception: the server must consent too.
	for _, permissions := range []int64{0, int64(discordgo.PermissionManageChannels | discordgo.PermissionManageMessages)} {
		if reply := test.run(serverCommand(guildX, dmUserID, "link", permissions, codeOption(code))); reply != manageServerMessage {
			t.Errorf("permissions %d: reply = %q, want %q", permissions, reply, manageServerMessage)
		}
	}
	if guildID, _ := test.linkedServer(test.dmA); guildID != "" {
		t.Fatalf("a refused /link linked %q", guildID)
	}
	// The refusals didn't use the code up. Administrator implies Manage Server.
	if reply := test.run(serverCommand(guildX, dmUserID, "link", administrator, codeOption(code))); !strings.HasPrefix(reply, "Linked this server to ") {
		t.Errorf("an administrator's /link answered %q", reply)
	}
	if reply := test.run(serverCommand("", dmUserID, "link", 0, codeOption(code))); reply != useInServerMessage {
		t.Errorf("/link in a direct message answered %q", reply)
	}
}

func TestLinkRefusesBadCodes(t *testing.T) {
	test := newWorld(t)

	if reply := test.run(serverCommand(guildX, dmUserID, "link", manageServer, codeOption("BENY-22222-22222"))); reply != core.ErrLinkCodeInvalid.Message {
		t.Errorf("a mistyped code: %q", reply)
	}
	if reply := test.run(serverCommand(guildX, dmUserID, "link", manageServer)); reply != core.ErrLinkCodeInvalid.Message {
		t.Errorf("no code: %q", reply)
	}

	expired := test.linkCode(test.dmA)
	test.clock.advance(core.LinkCodeLifetime)
	if reply := test.run(serverCommand(guildX, dmUserID, "link", manageServer, codeOption(expired))); reply != core.ErrLinkCodeInvalid.Message {
		t.Errorf("an expired code: %q", reply)
	}

	used := test.linkCode(test.dmA)
	if reply := test.run(serverCommand(guildX, dmUserID, "link", manageServer, codeOption(used))); !strings.HasPrefix(reply, "Linked") {
		t.Fatalf("a fresh code: %q", reply)
	}
	if reply := test.run(serverCommand(guildZ, dmUserID, "link", manageServer, codeOption(used))); reply != core.ErrLinkCodeInvalid.Message {
		t.Errorf("a used code in another server: %q", reply)
	}

	suspendedCode := test.linkCode(test.dmB)
	if _, err := test.service.SuspendCampaign(test.context, test.owner, test.dmB.CampaignID()); err != nil {
		t.Fatal(err)
	}
	if reply := test.run(serverCommand(guildY, otherDmUserID, "link", manageServer, codeOption(suspendedCode))); reply != core.ErrLinkCodeInvalid.Message {
		t.Errorf("a suspended campaign's code: %q", reply)
	}
	if reply := test.run(slashCommand(guildY, activeUserID, "beny")); reply != core.ErrGuildNotLinked.Message {
		t.Errorf("after refused links, server Y answered %q", reply)
	}
}

func TestLinkIsOneServerToOneCampaign(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)

	// B's DM can't take over A's server, and the answer doesn't name A.
	reply := test.run(serverCommand(guildX, otherDmUserID, "link", manageServer, codeOption(test.linkCode(test.dmB))))
	if reply != core.ErrGuildAlreadyLinked.Message {
		t.Errorf("linking a linked server: %q", reply)
	}
	if strings.Contains(reply, campaignAName) {
		t.Errorf("the refusal names the other campaign: %q", reply)
	}
	// A can't link a second server without unlinking the first.
	if reply := test.run(serverCommand(guildZ, dmUserID, "link", manageServer, codeOption(test.linkCode(test.dmA)))); reply != core.ErrCampaignAlreadyLinked.Message {
		t.Errorf("linking a second server: %q", reply)
	}
	if guildID, _ := test.linkedServer(test.dmA); guildID != guildX {
		t.Errorf("A is linked to %q, want X", guildID)
	}
	if guildID, _ := test.linkedServer(test.dmB); guildID != "" {
		t.Errorf("B is linked to %q, want nothing", guildID)
	}
}

// The troubleshooting answer in docs/discord-dev-setup.md: unlink the other server, then run /link
// again with the same code.
func TestARefusedLinkKeepsTheCode(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)

	code := test.linkCode(test.dmA)
	if reply := test.run(serverCommand(guildZ, dmUserID, "link", manageServer, codeOption(code))); reply != core.ErrCampaignAlreadyLinked.Message {
		t.Fatalf("linking a second server: %q", reply)
	}
	if reply := test.run(serverCommand(guildY, otherDmUserID, "link", manageServer, codeOption(test.linkCode(test.dmB)))); !strings.HasPrefix(reply, "Linked") {
		t.Fatalf("B's link: %q", reply)
	}
	if reply := test.run(serverCommand(guildY, dmUserID, "link", manageServer, codeOption(code))); reply != core.ErrGuildAlreadyLinked.Message {
		t.Fatalf("linking B's server: %q", reply)
	}
	if reply := test.run(serverCommand(guildX, dmUserID, "unlink", manageServer)); !strings.HasPrefix(reply, "Unlinked this server") {
		t.Fatalf("/unlink: %q", reply)
	}
	if reply := test.run(serverCommand(guildZ, dmUserID, "link", manageServer, codeOption(code))); !strings.HasPrefix(reply, "Linked") {
		t.Fatalf("the same code after the refusals: %q", reply)
	}
	if guildID, _ := test.linkedServer(test.dmA); guildID != guildZ {
		t.Errorf("A is linked to %q, want Z", guildID)
	}
	if reply := test.run(serverCommand(guildX, dmUserID, "link", manageServer, codeOption(code))); reply != core.ErrLinkCodeInvalid.Message {
		t.Errorf("the code after it linked: %q", reply)
	}
}

func TestUnlinkNeedsManageServerAndClearsTheLink(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)

	if reply := test.run(serverCommand(guildX, activeUserID, "unlink", 0)); reply != manageServerMessage {
		t.Errorf("a player's /unlink: %q", reply)
	}
	if guildID, _ := test.linkedServer(test.dmA); guildID != guildX {
		t.Fatal("a refused /unlink unlinked the server")
	}
	// A server admin needn't be in the campaign; the answer doesn't name it.
	reply := test.run(serverCommand(guildX, unknownUserID, "unlink", manageServer))
	if !strings.HasPrefix(reply, "Unlinked this server") || strings.Contains(reply, campaignAName) {
		t.Errorf("/unlink answered %q", reply)
	}
	if guildID, _ := test.linkedServer(test.dmA); guildID != "" {
		t.Errorf("A is still linked to %q", guildID)
	}
	if reply := test.run(slashCommand(guildX, activeUserID, "beny")); reply != core.ErrGuildNotLinked.Message {
		t.Errorf("after /unlink, /beny answered %q", reply)
	}
	if reply := test.run(serverCommand(guildX, unknownUserID, "unlink", manageServer)); reply != "This server isn't linked to a campaign." {
		t.Errorf("a second /unlink answered %q", reply)
	}
	// The server can be linked again with a new code.
	test.link(guildX, test.dmA)
}

func TestRemovingBenyBotUnlinksItsServer(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.link(guildY, test.dmB)

	// An outage is not a removal.
	test.bot.handleGuildDelete(test.context, &discordgo.GuildDelete{Guild: &discordgo.Guild{ID: guildX, Unavailable: true}})
	if guildID, _ := test.linkedServer(test.dmA); guildID != guildX {
		t.Fatal("an unavailable server was unlinked")
	}
	test.bot.handleGuildDelete(test.context, &discordgo.GuildDelete{Guild: &discordgo.Guild{ID: guildX}})
	if guildID, _ := test.linkedServer(test.dmA); guildID != "" {
		t.Errorf("the server that removed Beny Bot is still linked: %q", guildID)
	}
	if guildID, _ := test.linkedServer(test.dmB); guildID != guildY {
		t.Errorf("removing Beny Bot from X unlinked B's server: %q", guildID)
	}
	test.bot.handleGuildDelete(test.context, nil)
	test.bot.handleGuildDelete(test.context, &discordgo.GuildDelete{})
}

func TestReadyUnlinksServersThatRemovedBenyBotWhileItWasOff(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.link(guildY, test.dmB)

	// Ready lists X (unavailable for now, which still means present) and Z, but not Y.
	test.bot.handleReady(test.context, &discordgo.Ready{
		User:   &discordgo.User{ID: "500000000000000001"},
		Guilds: []*discordgo.Guild{{ID: guildX, Unavailable: true}, {ID: guildZ}},
	})
	if guildID, _ := test.linkedServer(test.dmA); guildID != guildX {
		t.Errorf("a server still in Ready's list was unlinked: %q", guildID)
	}
	if guildID, _ := test.linkedServer(test.dmB); guildID != "" {
		t.Errorf("a server missing from Ready's list is still linked: %q", guildID)
	}
	if reply := test.run(slashCommand(guildY, activeUserID, "beny")); reply != core.ErrGuildNotLinked.Message {
		t.Errorf("after reconciling, server Y answered %q", reply)
	}
}

// A Ready with no servers is taken as incomplete: it unlinks nothing.
func TestAnEmptyReadyUnlinksNothing(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.link(guildY, test.dmB)

	for _, guilds := range [][]*discordgo.Guild{nil, {}, {nil, {ID: ""}}} {
		test.bot.handleReady(test.context, &discordgo.Ready{User: &discordgo.User{ID: "500000000000000001"}, Guilds: guilds})
	}
	if guildID, _ := test.linkedServer(test.dmA); guildID != guildX {
		t.Errorf("an empty Ready unlinked A's server: %q", guildID)
	}
	if guildID, _ := test.linkedServer(test.dmB); guildID != guildY {
		t.Errorf("an empty Ready unlinked B's server: %q", guildID)
	}
	if reply := test.run(slashCommand(guildX, activeUserID, "beny")); reply != "Boy!" {
		t.Errorf("after an empty Ready, /beny answered %q", reply)
	}
}
