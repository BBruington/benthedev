package bot

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const (
	grainBargePrompt = "Thorin requests **Grain Barge** from Lady Vex (3 favor → Food +7). Favor now 5."
	alreadyDecided   = "That request was already decided."
	requestNotFound  = "That request doesn't exist."
)

// newRequestWorld links server X to campaign A and server Y to campaign B, each with its DM channel.
func newRequestWorld(t *testing.T) *world {
	t.Helper()
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.link(guildY, test.dmB)
	test.setDmChannel(test.dmA, channelXDesk)
	test.setDmChannel(test.dmB, channelYDesk)
	return test
}

// grainBarge gives the DM's campaign Lady Vex at the given favor and her Grain Barge (3 favor → Food
// +7), both revealed, and returns the patron and the action.
func (test *world) grainBarge(dm core.Viewer, favor int64) (uuid.UUID, uuid.UUID) {
	test.t.Helper()
	page, err := test.service.CreatePatron(test.context, dm, core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{Name: "Lady Vex"}})
	if err != nil {
		test.t.Fatalf("create patron: %v", err)
	}
	test.setFavor(dm, page.Patron.ID, favor)
	action, err := test.service.CreatePatronAction(test.context, dm, core.NewPatronActionInput{
		PatronID:          page.Patron.ID,
		Revealed:          true,
		PatronActionInput: core.PatronActionInput{Name: "Grain Barge", FavorCost: 3, Effects: core.Effects{Resources: map[string]int64{"food": 7}}},
	})
	if err != nil {
		test.t.Fatalf("create action: %v", err)
	}
	return page.Patron.ID, action.ID
}

func (test *world) setFavor(dm core.Viewer, patronID uuid.UUID, favor int64) {
	test.t.Helper()
	board, err := test.service.GetColonyBoard(test.context, dm)
	if err != nil {
		test.t.Fatalf("board: %v", err)
	}
	if _, err := test.service.AdjustFavor(test.context, dm, patronID, core.FavorAdjustInput{ExpectedVersion: board.Version, SetTo: &favor}); err != nil {
		test.t.Fatalf("set favor: %v", err)
	}
}

func (test *world) favor(dm core.Viewer, patronID uuid.UUID) int64 {
	test.t.Helper()
	page, err := test.service.GetPatron(test.context, dm, patronID)
	if err != nil {
		test.t.Fatalf("read patron: %v", err)
	}
	return page.Patron.Favor
}

// thorin is Ana in the DM's campaign, whose DM named her character Thorin.
func (test *world) thorin(dm core.Viewer) core.Viewer {
	test.t.Helper()
	ana := test.enter(test.signIn(activeUserID, "ana"), dm.CampaignID())
	name := "Thorin"
	if _, err := test.service.SetCharacterName(test.context, dm, ana.MemberID(), core.CharacterNameInput{CharacterName: &name}); err != nil {
		test.t.Fatalf("name Thorin: %v", err)
	}
	return ana
}

// ask makes the request and waits until Beny Bot has posted its prompt.
func (test *world) ask(player core.Viewer, actionID uuid.UUID) uuid.UUID {
	test.t.Helper()
	request, err := test.service.CreateRequest(test.context, player, core.RequestInput{ActionID: actionID})
	if err != nil {
		test.t.Fatalf("request: %v", err)
	}
	test.bot.posts.Wait()
	return request.ID
}

func (test *world) requestStatus(viewer core.Viewer, requestID uuid.UUID) core.RequestStatus {
	test.t.Helper()
	request, err := test.service.GetRequest(test.context, viewer, requestID)
	if err != nil {
		test.t.Fatalf("read request: %v", err)
	}
	return request.Status
}

// prompt is the one message in the channel, as Discord shows it now.
func (test *world) prompt(channelID string) *discordgo.Message {
	test.t.Helper()
	messages := test.client.messagesTo(channelID)
	if len(messages) != 1 {
		test.t.Fatalf("channel %s has %d messages, want 1", channelID, len(messages))
	}
	test.client.mutex.Lock()
	defer test.client.mutex.Unlock()
	for _, posted := range test.client.posted {
		if posted.ChannelID == channelID {
			copied := *posted
			return &copied
		}
	}
	test.t.Fatalf("channel %s has no posted message", channelID)
	return nil
}

func (test *world) followUps() []string {
	test.client.mutex.Lock()
	defer test.client.mutex.Unlock()
	contents := make([]string, 0, len(test.client.followUps))
	for _, followUp := range test.client.followUps {
		if followUp.Flags&discordgo.MessageFlagsEphemeral == 0 {
			test.t.Errorf("follow-up %q is not ephemeral", followUp.Content)
		}
		contents = append(contents, followUp.Content)
	}
	return contents
}

var interactionCounter atomic.Int64

// buttonClick is a click on a button of message, from a member of the server.
func buttonClick(guildID string, discordUserID string, message *discordgo.Message, customID string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID:        fmt.Sprintf("interaction-%d", interactionCounter.Add(1)),
		Type:      discordgo.InteractionMessageComponent,
		GuildID:   guildID,
		ChannelID: message.ChannelID,
		Message:   message,
		Member:    &discordgo.Member{User: &discordgo.User{ID: discordUserID}},
		Data:      discordgo.MessageComponentInteractionData{CustomID: customID, ComponentType: discordgo.ButtonComponent},
	}}
}

// click hands Beny Bot one click and waits for the posts it started; it returns the click's one answer.
func (test *world) click(event *discordgo.InteractionCreate) *discordgo.InteractionResponse {
	test.t.Helper()
	test.clock.advance(commandRefillInterval)
	before := len(test.client.recordedResponses())
	test.bot.handleInteraction(test.context, event)
	test.bot.posts.Wait()
	responses := test.client.recordedResponses()
	if len(responses) != before+1 {
		test.t.Fatalf("got %d answers, want 1", len(responses)-before)
	}
	return responses[before]
}

func requireDeferredUpdate(t *testing.T, response *discordgo.InteractionResponse) {
	t.Helper()
	if response.Type != discordgo.InteractionResponseDeferredMessageUpdate {
		t.Fatalf("answer type = %v, want a deferred update", response.Type)
	}
}

// buttonIDs lists the prompt's button custom IDs, or none once it is decided.
func buttonIDs(message *discordgo.Message) []string {
	var customIDs []string
	for _, component := range message.Components {
		row, isRow := component.(discordgo.ActionsRow)
		if !isRow {
			continue
		}
		for _, inner := range row.Components {
			if button, isButton := inner.(discordgo.Button); isButton && button.Style != discordgo.LinkButton {
				customIDs = append(customIDs, button.CustomID)
			}
		}
	}
	return customIDs
}

// reviewLinks lists the prompt's link buttons' URLs.
func reviewLinks(message *discordgo.Message) []string {
	var links []string
	for _, component := range message.Components {
		row, isRow := component.(discordgo.ActionsRow)
		if !isRow {
			continue
		}
		for _, inner := range row.Components {
			if button, isButton := inner.(discordgo.Button); isButton && button.Style == discordgo.LinkButton {
				links = append(links, button.URL)
			}
		}
	}
	return links
}

// requireButtons checks Grant, Decline and the link to the request's approval sheet in its campaign.
func requireButtons(t *testing.T, message *discordgo.Message, campaignSlug string, requestID uuid.UUID) {
	t.Helper()
	customIDs := buttonIDs(message)
	expected := []string{"req:" + requestID.String() + ":grant", "req:" + requestID.String() + ":decline"}
	if strings.Join(customIDs, " ") != strings.Join(expected, " ") {
		t.Errorf("buttons = %v, want %v", customIDs, expected)
	}
	expectedLink := testPublicURL + "/c/" + campaignSlug + "/requests?review=" + requestID.String()
	if links := reviewLinks(message); len(links) != 1 || links[0] != expectedLink {
		t.Errorf("links = %v, want [%s]", links, expectedLink)
	}
}

func requireNoButtons(t *testing.T, message *discordgo.Message) {
	t.Helper()
	if customIDs := buttonIDs(message); len(customIDs) != 0 {
		t.Errorf("the decided prompt still has buttons %v", customIDs)
	}
	if links := reviewLinks(message); len(links) != 0 {
		t.Errorf("the decided prompt still links to its sheet: %v", links)
	}
}

func TestARequestIsPromptedInItsCampaignsDmChannelWithButtons(t *testing.T) {
	test := newRequestWorld(t)
	_, barge := test.grainBarge(test.dmA, 5)
	requestID := test.ask(test.thorin(test.dmA), barge)

	prompt := test.prompt(channelXDesk)
	if prompt.Content != grainBargePrompt {
		t.Errorf("prompt = %q, want %q", prompt.Content, grainBargePrompt)
	}
	requireButtons(t, prompt, campaignASlug, requestID)
	sent := test.client.messagesTo(channelXDesk)[0].message
	if sent.AllowedMentions == nil || len(sent.AllowedMentions.Parse) != 0 || sent.AllowedMentions.Parse == nil {
		t.Errorf("the prompt allows mentions: %+v", sent.AllowedMentions)
	}
	if sent.Flags&discordgo.MessageFlagsSuppressEmbeds == 0 {
		t.Error("the prompt lets Discord show a link preview under it")
	}
	if others := test.client.messagesTo(channelYDesk); len(others) != 0 {
		t.Errorf("B's DM channel got %d messages about A's request", len(others))
	}

	// The prompt's message ID was stored: a decision on the website edits it.
	if _, err := test.service.DeclineRequest(test.context, test.dmA, requestID, core.DeclineRequestInput{Note: "Not this season."}); err != nil {
		t.Fatalf("decline: %v", err)
	}
	test.bot.posts.Wait()
	decided := test.prompt(channelXDesk)
	if decided.Content != grainBargePrompt+"\nDeclined" {
		t.Errorf("declined prompt = %q", decided.Content)
	}
	requireNoButtons(t, decided)
}

func TestARequestPromptEscapesAPlayersCharacterName(t *testing.T) {
	test := newRequestWorld(t)
	_, barge := test.grainBarge(test.dmA, 5)
	ana := test.enter(test.signIn(activeUserID, "ana"), test.dmA.CampaignID())
	name := "*Sly* Pip"
	if _, err := test.service.SetCharacterName(test.context, test.dmA, ana.MemberID(), core.CharacterNameInput{CharacterName: &name}); err != nil {
		t.Fatalf("name the character: %v", err)
	}
	test.ask(ana, barge)

	want := `\*Sly\* Pip requests **Grain Barge** from Lady Vex (3 favor → Food +7). Favor now 5.`
	if prompt := test.prompt(channelXDesk); prompt.Content != want {
		t.Errorf("prompt = %q, want %q", prompt.Content, want)
	}
}

func TestARequestWithoutADmChannelPostsNothing(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	_, barge := test.grainBarge(test.dmA, 5)
	requestID := test.ask(test.thorin(test.dmA), barge)
	if len(test.client.messagesTo(channelXDesk)) != 0 || len(test.client.messagesTo(channelXGeneral)) != 0 {
		t.Fatal("a request without a DM channel was posted")
	}
	if test.requestStatus(test.dmA, requestID) != core.RequestStatusPending {
		t.Error("the request no longer waits")
	}
}

func TestTheDmGrantsFromDiscord(t *testing.T) {
	test := newRequestWorld(t)
	vex, barge := test.grainBarge(test.dmA, 5)
	requestID := test.ask(test.thorin(test.dmA), barge)

	requireDeferredUpdate(t, test.click(buttonClick(guildX, dmUserID, test.prompt(channelXDesk), "req:"+requestID.String()+":grant")))
	granted := test.prompt(channelXDesk)
	if expected := grainBargePrompt + "\nGranted · Food +7 · favor 5 → 2"; granted.Content != expected {
		t.Errorf("granted prompt = %q, want %q", granted.Content, expected)
	}
	requireNoButtons(t, granted)
	if favor := test.favor(test.dmA, vex); favor != 2 {
		t.Errorf("favor = %d, want 2", favor)
	}
	if status := test.requestStatus(test.dmA, requestID); status != core.RequestStatusApproved {
		t.Errorf("status = %s, want APPROVED", status)
	}
	if followUps := test.followUps(); len(followUps) != 0 {
		t.Errorf("a successful grant followed up with %v", followUps)
	}

	// A second click from a client still showing the buttons is a harmless no-op: no second grant, and
	// no edit puts the buttons back.
	stale := &discordgo.Message{ID: granted.ID, ChannelID: channelXDesk, Content: grainBargePrompt}
	requireDeferredUpdate(t, test.click(buttonClick(guildX, dmUserID, stale, "req:"+requestID.String()+":grant")))
	if favor := test.favor(test.dmA, vex); favor != 2 {
		t.Errorf("favor after a second click = %d, want 2", favor)
	}
	if after := test.prompt(channelXDesk); after.Content != granted.Content || len(buttonIDs(after)) != 0 {
		t.Errorf("a second click changed the prompt to %q with buttons %v", after.Content, buttonIDs(after))
	}
	if followUps := test.followUps(); len(followUps) != 1 || followUps[0] != alreadyDecided {
		t.Errorf("follow-ups = %v, want only %q", followUps, alreadyDecided)
	}
}

func TestTheDmDeclinesFromDiscord(t *testing.T) {
	test := newRequestWorld(t)
	vex, barge := test.grainBarge(test.dmA, 5)
	requestID := test.ask(test.thorin(test.dmA), barge)

	requireDeferredUpdate(t, test.click(buttonClick(guildX, dmUserID, test.prompt(channelXDesk), "req:"+requestID.String()+":decline")))
	declined := test.prompt(channelXDesk)
	if declined.Content != grainBargePrompt+"\nDeclined" {
		t.Errorf("declined prompt = %q", declined.Content)
	}
	requireNoButtons(t, declined)
	if favor := test.favor(test.dmA, vex); favor != 5 {
		t.Errorf("favor = %d, want 5", favor)
	}
	if status := test.requestStatus(test.dmA, requestID); status != core.RequestStatusDeclined {
		t.Errorf("status = %s, want DECLINED", status)
	}
}

func TestAShortfallKeepsTheButtons(t *testing.T) {
	test := newRequestWorld(t)
	vex, barge := test.grainBarge(test.dmA, 5)
	requestID := test.ask(test.thorin(test.dmA), barge)
	test.setFavor(test.dmA, vex, 2)

	requireDeferredUpdate(t, test.click(buttonClick(guildX, dmUserID, test.prompt(channelXDesk), "req:"+requestID.String()+":grant")))
	short := test.prompt(channelXDesk)
	if expected := grainBargePrompt + "\nNot enough favor: Lady Vex has 2, needs 3. Open in app."; short.Content != expected {
		t.Errorf("prompt = %q, want %q", short.Content, expected)
	}
	requireButtons(t, short, campaignASlug, requestID)
	if status := test.requestStatus(test.dmA, requestID); status != core.RequestStatusPending {
		t.Errorf("status = %s, want PENDING", status)
	}

	// Topped up, the same buttons grant, and the shortfall line gives way to the outcome.
	test.setFavor(test.dmA, vex, 4)
	requireDeferredUpdate(t, test.click(buttonClick(guildX, dmUserID, short, "req:"+requestID.String()+":grant")))
	if granted := test.prompt(channelXDesk); granted.Content != grainBargePrompt+"\nGranted · Food +7 · favor 4 → 1" {
		t.Errorf("granted prompt = %q", granted.Content)
	}
}

func TestWebsiteDecisionsEditThePrompt(t *testing.T) {
	test := newRequestWorld(t)
	_, barge := test.grainBarge(test.dmA, 5)
	thorin := test.thorin(test.dmA)

	requestID := test.ask(thorin, barge)
	change, err := test.service.ApproveRequest(test.context, test.dmA, requestID, core.ApproveRequestInput{})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	test.bot.posts.Wait()
	granted := test.prompt(channelXDesk)
	if granted.Content != grainBargePrompt+"\nGranted · Food +7 · favor 5 → 2" {
		t.Errorf("granted prompt = %q", granted.Content)
	}
	requireNoButtons(t, granted)

	// Undoing the grant leaves the request waiting, so the buttons come back.
	if _, err := test.service.UndoLastChange(test.context, test.dmA, core.UndoInput{GroupID: change.GroupID}); err != nil {
		t.Fatalf("undo: %v", err)
	}
	test.bot.posts.Wait()
	undone := test.prompt(channelXDesk)
	if undone.Content != grainBargePrompt+"\nUndone" {
		t.Errorf("undone prompt = %q", undone.Content)
	}
	requireButtons(t, undone, campaignASlug, requestID)

	if _, err := test.service.WithdrawRequest(test.context, thorin, requestID); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	test.bot.posts.Wait()
	withdrawn := test.prompt(channelXDesk)
	if withdrawn.Content != grainBargePrompt+"\nWithdrawn" {
		t.Errorf("withdrawn prompt = %q", withdrawn.Content)
	}
	requireNoButtons(t, withdrawn)
}

func TestAPromptBenyBotCantReadShowsTheOutcomeAlone(t *testing.T) {
	test := newRequestWorld(t)
	_, barge := test.grainBarge(test.dmA, 5)
	requestID := test.ask(test.thorin(test.dmA), barge)
	test.client.mutex.Lock()
	test.client.readError = errors.New("HTTP 403 Forbidden: Missing Access")
	test.client.mutex.Unlock()

	if _, err := test.service.DeclineRequest(test.context, test.dmA, requestID, core.DeclineRequestInput{}); err != nil {
		t.Fatalf("decline: %v", err)
	}
	test.bot.posts.Wait()
	declined := test.prompt(channelXDesk)
	if declined.Content != "Declined" {
		t.Errorf("prompt = %q, want the outcome alone", declined.Content)
	}
	requireNoButtons(t, declined)
}

func TestOnlyTheDmCanDecide(t *testing.T) {
	test := newRequestWorld(t)
	vex, barge := test.grainBarge(test.dmA, 5)
	requestID := test.ask(test.thorin(test.dmA), barge)
	grant := "req:" + requestID.String() + ":grant"

	for _, refusal := range []struct {
		guildID       string
		discordUserID string
		expected      string
	}{
		{guildX, activeUserID, onlyDmDecidesMessage},
		{guildX, pendingUserID, core.ErrNotLetIn.Message},
		{guildX, removedUserID, core.ErrNotLetIn.Message},
		{guildX, strangerUserID, core.ErrNotInCampaign.Message},
		{guildX, unknownUserID, core.ErrNotInCampaign.Message},
		// B's DM is no DM in A's server.
		{guildX, otherDmUserID, core.ErrNotInCampaign.Message},
		{"", dmUserID, useInServerMessage},
	} {
		content := requireEphemeral(t, test.click(buttonClick(refusal.guildID, refusal.discordUserID, test.prompt(channelXDesk), grant)))
		if content != refusal.expected {
			t.Errorf("%s in %q was answered %q, want %q", refusal.discordUserID, refusal.guildID, content, refusal.expected)
		}
	}
	if content := requireEphemeral(t, test.click(buttonClick(guildZ, dmUserID, test.prompt(channelXDesk), grant))); content != core.ErrGuildNotLinked.Message {
		t.Errorf("an unlinked server answered %q", content)
	}
	if favor := test.favor(test.dmA, vex); favor != 5 {
		t.Errorf("favor = %d, want 5", favor)
	}
	if status := test.requestStatus(test.dmA, requestID); status != core.RequestStatusPending {
		t.Errorf("status = %s, want PENDING", status)
	}
	if prompt := test.prompt(channelXDesk); prompt.Content != grainBargePrompt {
		t.Errorf("a refused click edited the prompt to %q", prompt.Content)
	}
}

func TestAButtonNeverReachesAnotherCampaignsRequest(t *testing.T) {
	test := newRequestWorld(t)
	vexA, bargeA := test.grainBarge(test.dmA, 5)
	requestA := test.ask(test.thorin(test.dmA), bargeA)
	_, bargeB := test.grainBarge(test.dmB, 5)
	requestB := test.ask(test.thorin(test.dmB), bargeB)
	promptB := test.prompt(channelYDesk)
	requireButtons(t, promptB, campaignBSlug, requestB)

	// B's DM, in B's server, on B's prompt, with A's request ID: B's campaign has no such request.
	requireDeferredUpdate(t, test.click(buttonClick(guildY, otherDmUserID, promptB, "req:"+requestA.String()+":grant")))
	if followUps := test.followUps(); len(followUps) != 1 || followUps[0] != requestNotFound {
		t.Errorf("follow-ups = %v, want only %q", followUps, requestNotFound)
	}
	// A's DM is no member of B, so B's server refuses them even with A's own request ID.
	if content := requireEphemeral(t, test.click(buttonClick(guildY, dmUserID, promptB, "req:"+requestA.String()+":grant"))); content != core.ErrNotInCampaign.Message {
		t.Errorf("A's DM in B's server was answered %q", content)
	}
	if favor := test.favor(test.dmA, vexA); favor != 5 {
		t.Errorf("A's favor = %d, want 5", favor)
	}
	if status := test.requestStatus(test.dmA, requestA); status != core.RequestStatusPending {
		t.Errorf("A's request is %s, want PENDING", status)
	}
	if prompt := test.prompt(channelXDesk); prompt.Content != grainBargePrompt {
		t.Errorf("A's prompt became %q", prompt.Content)
	}
	if prompt := test.prompt(channelYDesk); prompt.Content != promptB.Content {
		t.Errorf("B's prompt became %q", prompt.Content)
	}
}

func TestADecisionsEditStaysInTheLinkedServer(t *testing.T) {
	test := newRequestWorld(t)
	_, barge := test.grainBarge(test.dmA, 5)
	requestID := test.ask(test.thorin(test.dmA), barge)
	editsBefore := len(test.client.edits)

	// The gateway now reports the DM channel as someone else's: Beny Bot edits nothing there.
	test.state.moveChannel(channelXDesk, guildZ)
	if _, err := test.service.DeclineRequest(test.context, test.dmA, requestID, core.DeclineRequestInput{}); err != nil {
		t.Fatalf("decline: %v", err)
	}
	test.bot.posts.Wait()
	test.client.mutex.Lock()
	edits := len(test.client.edits) - editsBefore
	test.client.mutex.Unlock()
	if edits != 0 {
		t.Errorf("edited %d messages in a channel outside the linked server", edits)
	}
}

func TestTwoProcessesDecideAClickOnce(t *testing.T) {
	test := newRequestWorld(t)
	vex, barge := test.grainBarge(test.dmA, 9)
	requestID := test.ask(test.thorin(test.dmA), barge)
	click := buttonClick(guildX, dmUserID, test.prompt(channelXDesk), "req:"+requestID.String()+":grant")

	// A deploy overlap: a second process, connected with the same token, receives the same click.
	answers := &discordAnswers{answered: map[string]bool{}}
	test.client.answers = answers
	secondClient := newFakeDiscordClient()
	secondClient.answers = answers
	second := newBot(Options{Token: "test-token", Logger: quietLogger()}, secondClient, test.state, test.clock.now)
	if err := second.start(test.service); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.stop)

	answeredBefore := len(test.client.recordedResponses())
	var clicks sync.WaitGroup
	clicks.Go(func() { test.bot.handleInteraction(test.context, click) })
	clicks.Go(func() { second.handleInteraction(test.context, click) })
	clicks.Wait()
	test.bot.posts.Wait()

	if favor := test.favor(test.dmA, vex); favor != 6 {
		t.Errorf("favor = %d, want 6 (one grant)", favor)
	}
	if answered := len(test.client.recordedResponses()) - answeredBefore + len(secondClient.recordedResponses()); answered != 1 {
		t.Errorf("the click was answered %d times, want 1", answered)
	}
}

func TestADoubleClickGrantsOnce(t *testing.T) {
	test := newRequestWorld(t)
	vex, barge := test.grainBarge(test.dmA, 9)
	requestID := test.ask(test.thorin(test.dmA), barge)
	prompt := test.prompt(channelXDesk)

	var clicks sync.WaitGroup
	for range 2 {
		clicks.Go(func() {
			test.bot.handleInteraction(test.context, buttonClick(guildX, dmUserID, prompt, "req:"+requestID.String()+":grant"))
		})
	}
	clicks.Wait()
	test.bot.posts.Wait()
	if favor := test.favor(test.dmA, vex); favor != 6 {
		t.Errorf("favor after a double click = %d, want 6 (one grant)", favor)
	}
	if granted := test.prompt(channelXDesk); granted.Content != "Thorin requests **Grain Barge** from Lady Vex (3 favor → Food +7). Favor now 9.\nGranted · Food +7 · favor 9 → 6" {
		t.Errorf("granted prompt = %q", granted.Content)
	}
	if followUps := test.followUps(); len(followUps) != 1 || followUps[0] != alreadyDecided {
		t.Errorf("follow-ups = %v, want only %q", followUps, alreadyDecided)
	}
}

func TestUnknownButtonsAreAnswered(t *testing.T) {
	test := newRequestWorld(t)
	message := &discordgo.Message{ID: "499999999999999999", ChannelID: channelXDesk}
	for _, customID := range []string{"req:1:grant", "req:" + uuid.NewString() + ":approve", "npc:" + uuid.NewString() + ":grant", "req:" + uuid.NewString()} {
		if content := requireEphemeral(t, test.click(buttonClick(guildX, dmUserID, message, customID))); content != unknownButtonMessage {
			t.Errorf("%q was answered %q", customID, content)
		}
	}
}
