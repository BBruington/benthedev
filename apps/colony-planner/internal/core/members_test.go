package core_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

const testDmDiscordID = "500000000000000005"

type countingNotifier struct {
	mutex sync.Mutex
	pings int
}

func (notifier *countingNotifier) MemberWaiting(context.Context, core.WaitingMember) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.pings++
}

type recordingEvictor struct {
	mutex   sync.Mutex
	members []uuid.UUID
}

func (evictor *recordingEvictor) EvictSession([]byte) {}

func (evictor *recordingEvictor) EvictAll() {}

func (evictor *recordingEvictor) EvictMember(memberID uuid.UUID) {
	evictor.mutex.Lock()
	defer evictor.mutex.Unlock()
	evictor.members = append(evictor.members, memberID)
}

func newService(t *testing.T) (*core.Service, *dbtest.Database, *countingNotifier, *recordingEvictor) {
	t.Helper()
	database := dbtest.New(t)
	notifier := &countingNotifier{}
	evictor := &recordingEvictor{}
	service := core.New(core.Options{
		Pool:        database.Pool,
		Notifier:    notifier,
		Logger:      slog.New(slog.DiscardHandler),
		DmDiscordID: testDmDiscordID,
		Sessions:    evictor,
	})
	return service, database, notifier, evictor
}

func createPending(t *testing.T, database *dbtest.Database, count int) {
	t.Helper()
	for index := range count {
		if _, err := store.New(database.Pool).CreateMember(context.Background(), store.CreateMemberParams{
			DiscordUserID:   fmt.Sprintf("seed:waiting-%d", index),
			DiscordUsername: fmt.Sprintf("waiting%d", index),
			DisplayName:     fmt.Sprintf("Waiting %d", index),
			Status:          store.MemberStatusPENDING,
		}); err != nil {
			t.Fatalf("create pending member: %v", err)
		}
	}
}

func TestConcurrentSignInsCannotPassThePendingCap(t *testing.T) {
	service, database, notifier, _ := newService(t)
	createPending(t, database, core.PendingMemberLimit-1)

	results := make(chan error, 2)
	for index := range 2 {
		go func() {
			_, err := service.SignIn(context.Background(), core.DiscordIdentity{
				UserID:   fmt.Sprintf("60000000000000000%d", index),
				Username: fmt.Sprintf("racer%d", index),
			})
			results <- err
		}()
	}
	var admitted, refused int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, core.ErrSignUpsFull):
			refused++
		default:
			t.Fatalf("sign in: %v", err)
		}
	}
	if admitted != 1 || refused != 1 {
		t.Fatalf("admitted %d and refused %d; the cap should let exactly one in", admitted, refused)
	}
	if notifier.pings != 1 {
		t.Fatalf("pings = %d, want 1", notifier.pings)
	}
}

func TestSignInValidatesTheIdentity(t *testing.T) {
	service, _, _, _ := newService(t)
	if _, err := service.SignIn(context.Background(), core.DiscordIdentity{UserID: " ", Username: "x"}); err == nil {
		t.Fatal("a sign-in without a Discord ID was accepted")
	}
}

func TestOperationsRefusePlayersAndViewAsPlayer(t *testing.T) {
	service, _, _, evictor := newService(t)
	testContext := context.Background()
	dmID, err := service.SignIn(testContext, core.DiscordIdentity{UserID: testDmDiscordID, Username: "dm"})
	if err != nil {
		t.Fatalf("sign in the DM: %v", err)
	}
	playerID, err := service.SignIn(testContext, core.DiscordIdentity{UserID: "700000000000000007", Username: "ana"})
	if err != nil {
		t.Fatalf("sign in a player: %v", err)
	}
	dm := service.NewViewer(core.ViewerIdentity{MemberID: dmID, DiscordUserID: testDmDiscordID, Status: store.MemberStatusPENDING})
	if !dm.IsDM || !dm.IsActive() {
		t.Fatalf("the DM's viewer = %+v; the DM is always ACTIVE", dm)
	}
	dmAsPlayer := dm
	dmAsPlayer.ViewAsPlayer = true
	pending := service.NewViewer(core.ViewerIdentity{MemberID: playerID, DiscordUserID: "700000000000000007", Status: store.MemberStatusPENDING})

	for name, viewer := range map[string]core.Viewer{"a pending member": pending, "the DM viewing as player": dmAsPlayer} {
		if _, err := service.ListMembers(testContext, viewer); !isKind(err, core.KindForbidden) {
			t.Errorf("%s listed members: %v", name, err)
		}
		if _, err := service.ApproveMember(testContext, viewer, playerID); !isKind(err, core.KindForbidden) {
			t.Errorf("%s approved a member: %v", name, err)
		}
		if _, err := service.GetSettings(testContext, viewer); !isKind(err, core.KindForbidden) {
			t.Errorf("%s read settings: %v", name, err)
		}
	}
	if _, err := service.UpdateProfile(testContext, pending, core.ProfileInput{}); !isKind(err, core.KindForbidden) {
		t.Errorf("a pending member edited their profile: %v", err)
	}

	if _, err := service.ApproveMember(testContext, dm, playerID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if len(evictor.members) != 1 || evictor.members[0] != playerID {
		t.Fatalf("approval evicted %v; want exactly the approved member", evictor.members)
	}
	if _, err := service.ApproveMember(testContext, dm, playerID); err != nil {
		t.Fatalf("approving twice: %v", err)
	}
	if len(evictor.members) != 1 {
		t.Fatal("a no-op approval evicted sessions again")
	}
}

func TestUpdateSettingsValidates(t *testing.T) {
	service, _, _, _ := newService(t)
	testContext := context.Background()
	dmID, err := service.SignIn(testContext, core.DiscordIdentity{UserID: testDmDiscordID, Username: "dm"})
	if err != nil {
		t.Fatalf("sign in the DM: %v", err)
	}
	dm := service.NewViewer(core.ViewerIdentity{MemberID: dmID, DiscordUserID: testDmDiscordID})

	for _, timezone := range []string{"Mars/Olympus", "Local", ""} {
		_, err := service.UpdateSettings(testContext, dm, core.SettingsInput{CampaignName: "Sample", Timezone: timezone})
		operationError, typed := core.AsError(err)
		if !typed || operationError.Kind != core.KindInvalid || len(operationError.Fields) != 1 || operationError.Fields[0].Field != "timezone" {
			t.Errorf("timezone %q: %v", timezone, err)
		}
	}

	blank := "  "
	channel := "123456789012345678"
	settings, err := service.UpdateSettings(testContext, dm, core.SettingsInput{
		CampaignName:             "  Sample Colony ",
		Timezone:                 "America/New_York",
		DiscordAnnounceChannelID: &blank,
		DiscordDmChannelID:       &channel,
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if settings.CampaignName != "Sample Colony" || settings.DiscordAnnounceChannelID != nil || settings.DiscordDmChannelID == nil {
		t.Fatalf("settings = %+v", settings)
	}
	channels, err := service.DiscordChannels(testContext)
	if err != nil || channels.DmChannelID == nil || *channels.DmChannelID != channel {
		t.Fatalf("Beny Bot's channels = %+v, %v", channels, err)
	}
}

func TestSettingsShowBenyBotsInviteLinkOnlyWhenConfigured(t *testing.T) {
	database := dbtest.New(t)
	testContext := context.Background()
	inviteURL := "https://discord.com/oauth2/authorize?client_id=123456789012345678&scope=bot+applications.commands&permissions=8590019584"
	for _, configured := range []string{"", inviteURL} {
		service := core.New(core.Options{Pool: database.Pool, Logger: slog.New(slog.DiscardHandler), DmDiscordID: testDmDiscordID, BotInviteURL: configured})
		dm := service.NewViewer(core.ViewerIdentity{MemberID: uuid.New(), DiscordUserID: testDmDiscordID})
		settings, err := service.GetSettings(testContext, dm)
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

func isKind(err error, kind core.ErrorKind) bool {
	operationError, typed := core.AsError(err)
	return typed && operationError.Kind == kind
}
