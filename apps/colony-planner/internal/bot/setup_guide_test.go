package bot

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// The setup guide tells the author which permissions to grant when inviting Beny Bot Dev (spec §7.1).
// This keeps its number in step with the permissions Beny Bot actually needs.
func TestSetupGuideNamesBenyBotsInvite(t *testing.T) {
	guide, err := os.ReadFile("../../docs/discord-dev-setup.md")
	if err != nil {
		t.Fatalf("read the setup guide: %v", err)
	}
	for _, expected := range []string{
		"permissions=" + strconv.FormatInt(invitePermissions, 10),
		"scope=bot+applications.commands",
		"http://localhost:5173/auth/discord/callback",
	} {
		if !strings.Contains(string(guide), expected) {
			t.Errorf("docs/discord-dev-setup.md doesn't contain %q", expected)
		}
	}
}

// The link on /settings must match the one the setup guide tells the author to paste.
func TestInviteURLMatchesTheSetupGuide(t *testing.T) {
	if InviteURL("") != "" {
		t.Fatal("an invite link without a client ID")
	}
	guide, err := os.ReadFile("../../docs/discord-dev-setup.md")
	if err != nil {
		t.Fatalf("read the setup guide: %v", err)
	}
	documented := InviteURL("YOUR_CLIENT_ID")
	if !strings.Contains(string(guide), documented) {
		t.Fatalf("docs/discord-dev-setup.md doesn't show the invite link %q", documented)
	}
	if expected := "https://discord.com/oauth2/authorize?client_id=123456789012345678&scope=bot+applications.commands&permissions=8589954048"; InviteURL("123456789012345678") != expected {
		t.Fatalf("InviteURL = %q, want %q", InviteURL("123456789012345678"), expected)
	}
}
