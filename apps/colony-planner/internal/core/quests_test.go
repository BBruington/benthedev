package core_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

type requestRecorder struct {
	mutex    sync.Mutex
	prompts  []core.RequestPrompt
	outcomes []core.RequestOutcome
	// postsSlowly leaves storing the message ID to the test, as a post still on its way would.
	postsSlowly bool
}

// RequestCreated records the prompt and stores a message ID, as Beny Bot does after posting.
func (recorder *requestRecorder) RequestCreated(requestContext context.Context, prompt core.RequestPrompt) {
	recorder.mutex.Lock()
	recorder.prompts = append(recorder.prompts, prompt)
	postsSlowly := recorder.postsSlowly
	recorder.mutex.Unlock()
	if postsSlowly {
		return
	}
	if _, err := prompt.RecordMessage(requestContext, "message-"+prompt.RequestID.String()); err != nil {
		panic(err)
	}
}

func (recorder *requestRecorder) RequestDecided(_ context.Context, outcome core.RequestOutcome) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	recorder.outcomes = append(recorder.outcomes, outcome)
}

func (recorder *requestRecorder) lastOutcome() core.RequestOutcome {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	if len(recorder.outcomes) == 0 {
		return core.RequestOutcome{}
	}
	return recorder.outcomes[len(recorder.outcomes)-1]
}

type questTest struct {
	*colonyTest
	recorder *requestRecorder
}

func newQuestTest(t *testing.T) *questTest {
	t.Helper()
	colony := newColonyTest(t)
	recorder := &requestRecorder{}
	colony.service.SetRequestNotifier(recorder)
	thorin := "Thorin"
	if _, err := colony.service.SetCharacterName(colony.context, colony.dm, colony.player.MemberID(), core.CharacterNameInput{CharacterName: &thorin}); err != nil {
		t.Fatal(err)
	}
	return &questTest{colonyTest: colony, recorder: recorder}
}

func (test *questTest) patron(name string, revealed bool) core.Patron {
	test.t.Helper()
	page, err := test.service.CreatePatron(test.context, test.dm, core.NewPatronInput{Revealed: revealed, PatronInput: core.PatronInput{Name: name}})
	if err != nil {
		test.t.Fatalf("create patron %s: %v", name, err)
	}
	return page.Patron
}

func (test *questTest) setFavor(patronID uuid.UUID, favor int64) core.ColonyChange {
	test.t.Helper()
	return test.must(test.service.AdjustFavor(test.context, test.dm, patronID, core.FavorAdjustInput{ExpectedVersion: test.board().Version, SetTo: &favor}))
}

func (test *questTest) favor(patronID uuid.UUID) int64 {
	test.t.Helper()
	page, err := test.service.GetPatron(test.context, test.dm, patronID)
	if err != nil {
		test.t.Fatalf("read patron: %v", err)
	}
	return page.Patron.Favor
}

func (test *questTest) action(patronID uuid.UUID, name string, favorCost int64, effects core.Effects, revealed bool) core.PatronAction {
	test.t.Helper()
	action, err := test.service.CreatePatronAction(test.context, test.dm, core.NewPatronActionInput{
		PatronID:          patronID,
		Revealed:          revealed,
		PatronActionInput: core.PatronActionInput{Name: name, FavorCost: favorCost, Effects: effects},
	})
	if err != nil {
		test.t.Fatalf("create action %s: %v", name, err)
	}
	return action
}

func (test *questTest) quest(input core.NewQuestInput) core.Quest {
	test.t.Helper()
	quest, err := test.service.CreateQuest(test.context, test.dm, input)
	if err != nil {
		test.t.Fatalf("create quest %s: %v", input.Title, err)
	}
	return quest
}

func (test *questTest) entries(viewer core.Viewer, kind core.TimelineKind) []core.TimelineEntry {
	test.t.Helper()
	var matching []core.TimelineEntry
	for _, entry := range test.timeline(viewer, 0, test.board().Day) {
		if entry.Kind == kind {
			matching = append(matching, entry)
		}
	}
	return matching
}

func (test *questTest) dmAsPlayer() core.Viewer {
	test.t.Helper()
	caller := test.dmCaller
	caller.ViewAsPlayer = true
	return test.enter(caller, test.campaignID)
}

func requireMessage(t *testing.T, err error, kind core.ErrorKind, message string) {
	t.Helper()
	operationError, typed := core.AsError(err)
	if !typed || operationError.Kind != kind || operationError.Message != message {
		t.Fatalf("got %v; want kind %d %q", err, kind, message)
	}
}

func effects(resources map[string]int64, colonists int64) core.Effects {
	return core.Effects{Resources: resources, Colonists: colonists}
}

// SPEC §5.12 Example F, end to end through core.
func TestExampleFCompletingAQuest(t *testing.T) {
	test := newQuestTest(t)
	test.setHeadcounts(map[string]int32{"FARMER": 4, "BUILDER": 5, "SOLDIER": 4, "HEALER": 3, "UNASSIGNED": 0})
	test.correctDay(23)
	test.setTo(map[string]int64{"food": 0, "morale": 98})
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 1)
	wolfDen := test.quest(core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{
		Title:         "Clear the Wolf Den",
		PatronID:      &vex.ID,
		RewardEffects: effects(map[string]int64{"food": 20, "morale": 5}, 3),
		RewardFavor:   2,
		Loot:          []core.LootItem{{Name: "Wolf pelts", Quantity: 6}},
	}})
	if test.board().Colonists != 16 {
		t.Fatalf("before: %d colonists, want 16", test.board().Colonists)
	}

	preview, err := test.service.PreviewCompleteQuest(test.context, test.dm, wolfDen.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary != "Food +20 · Morale 98 → 100 (capped) · 3 colonists (Unassigned) · Lady Vex favor 1 → 3" {
		t.Fatalf("preview = %q", preview.Summary)
	}

	completed := test.must(test.service.CompleteQuest(test.context, test.dm, wolfDen.ID))
	board := test.board()
	if test.amount("food") != 20 || test.amount("morale") != 100 || test.favor(vex.ID) != 3 || board.Unassigned != 3 || board.Colonists != 19 {
		t.Fatalf("after: food %d, morale %d, favor %d, Unassigned %d, colonists %d",
			test.amount("food"), test.amount("morale"), test.favor(vex.ID), board.Unassigned, board.Colonists)
	}
	entry := completed.Entries[0]
	if entry.Kind != core.TimelineKindQuestCompleted || entry.Body != "Food +20 · Morale +2 · 3 colonists (Unassigned) · Lady Vex favor +2" ||
		entry.Dm.Visibility != core.TimelineVisibilityPublic || *entry.QuestID != wolfDen.ID || *entry.PatronID != vex.ID {
		t.Fatalf("QUEST_COMPLETED = %+v", entry)
	}
	if seen := test.entries(test.player, core.TimelineKindQuestCompleted); len(seen) != 1 {
		t.Fatalf("the player sees %d QUEST_COMPLETED entries, want 1", len(seen))
	}

	_, err = test.service.CompleteQuest(test.context, test.dm, wolfDen.ID)
	requireMessage(t, err, core.KindConflict, "Already resolved")
	_, err = test.service.FailQuest(test.context, test.dm, wolfDen.ID)
	requireMessage(t, err, core.KindConflict, "Already resolved")
	if test.amount("food") != 20 || test.favor(vex.ID) != 3 {
		t.Fatal("a second click paid again")
	}

	test.undo(completed)
	if test.amount("food") != 0 || test.amount("morale") != 98 || test.favor(vex.ID) != 1 || test.board().Colonists != 16 {
		t.Fatalf("undo: food %d, morale %d, favor %d", test.amount("food"), test.amount("morale"), test.favor(vex.ID))
	}
	reopened, err := test.service.GetQuest(test.context, test.dm, wolfDen.ID)
	if err != nil || reopened.Status != core.QuestStatusActive || reopened.ResolvedDay != nil {
		t.Fatalf("undo left the quest %+v, %v", reopened, err)
	}
}

func TestCompletingTwiceAtOncePaysOnce(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	quest := test.quest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{Title: "Escort", PatronID: &vex.ID, RewardFavor: 4,
		RewardEffects: effects(map[string]int64{"gold": 10}, 0)}})
	goldBefore := test.amount("gold")
	var group sync.WaitGroup
	results := make([]error, 4)
	for index := range results {
		group.Go(func() {
			_, results[index] = test.service.CompleteQuest(test.context, test.dm, quest.ID)
		})
	}
	group.Wait()
	succeeded := 0
	for _, err := range results {
		switch {
		case err == nil:
			succeeded++
		case !isKind(err, core.KindConflict):
			t.Fatalf("a concurrent Complete failed with %v", err)
		}
	}
	if succeeded != 1 || test.favor(vex.ID) != 4 || test.amount("gold") != goldBefore+10 {
		t.Fatalf("%d completions, favor %d, gold %d", succeeded, test.favor(vex.ID), test.amount("gold"))
	}
}

func TestFailingAQuestAppliesItsCost(t *testing.T) {
	test := newQuestTest(t)
	test.setTo(map[string]int64{"food": 10})
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 2)
	const failureCanary = "failure-cost-dm-notes"
	ledger := test.quest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{
		Title:          "Recover the Ledger",
		PatronID:       &vex.ID,
		FailureEffects: effects(map[string]int64{"food": -50}, 0),
		FailureFavor:   5,
		DmNotes:        failureCanary,
	}})
	// The failure cost is DM-only until it happens.
	seen, err := test.service.GetQuest(test.context, test.player, ledger.ID)
	if err != nil || seen.Dm != nil {
		t.Fatalf("the player's quest carries %+v, %v", seen.Dm, err)
	}
	encoded, _ := json.Marshal(seen)
	if strings.Contains(string(encoded), failureCanary) || strings.Contains(string(encoded), "failure") {
		t.Fatalf("the player's quest = %s", encoded)
	}

	preview, err := test.service.PreviewFailQuest(test.context, test.dm, ledger.ID)
	if err != nil || preview.Summary != "Food 10 → 0 (floored) · Lady Vex favor 2 → −3" {
		t.Fatalf("preview = %q, %v", preview.Summary, err)
	}
	failed := test.must(test.service.FailQuest(test.context, test.dm, ledger.ID))
	if test.amount("food") != 0 || test.favor(vex.ID) != -3 {
		t.Fatalf("after Fail: food %d, favor %d; want CLAMP to 0 and favor below 0", test.amount("food"), test.favor(vex.ID))
	}
	if entry := failed.Entries[0]; entry.Kind != core.TimelineKindQuestFailed || entry.Body != "Food −10 · Lady Vex favor −5" || entry.Dm.Visibility != core.TimelineVisibilityPublic {
		t.Fatalf("QUEST_FAILED = %+v", entry)
	}
	_, err = test.service.FailQuest(test.context, test.dm, ledger.ID)
	requireMessage(t, err, core.KindConflict, "Already resolved")

	// Undo reverses a Fail while it is the latest change.
	test.undo(failed)
	if test.amount("food") != 10 || test.favor(vex.ID) != 2 {
		t.Fatalf("undo: food %d, favor %d", test.amount("food"), test.favor(vex.ID))
	}

	// Reopening a failed quest is status only; a completed quest doesn't reopen.
	test.must(test.service.FailQuest(test.context, test.dm, ledger.ID))
	test.must(test.service.ReopenQuest(test.context, test.dm, ledger.ID))
	reopened, err := test.service.GetQuest(test.context, test.dm, ledger.ID)
	if err != nil || reopened.Status != core.QuestStatusActive || test.amount("food") != 0 || test.favor(vex.ID) != -3 {
		t.Fatalf("reopen: %+v, food %d, favor %d, %v", reopened.Status, test.amount("food"), test.favor(vex.ID), err)
	}
	if reopenedEntries := test.entries(test.player, core.TimelineKindQuestReopened); len(reopenedEntries) != 1 {
		t.Fatalf("QUEST_REOPENED entries = %d", len(reopenedEntries))
	}
	test.must(test.service.CompleteQuest(test.context, test.dm, ledger.ID))
	_, err = test.service.ReopenQuest(test.context, test.dm, ledger.ID)
	if !isKind(err, core.KindConflict) {
		t.Fatalf("reopening a completed quest = %v, want 409", err)
	}

	// A hidden patron's favor line stays out of PUBLIC text and in the DM's note.
	hollow := test.patron("The Hollow Court", false)
	secret := test.quest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{Title: "Carry the Letter", PatronID: &hollow.ID, FailureFavor: 1}})
	secretFailure := test.must(test.service.FailQuest(test.context, test.dm, secret.ID))
	entry := secretFailure.Entries[0]
	if strings.Contains(entry.Title+entry.Body, "Hollow") || entry.PatronID != nil || entry.Dm.Note != "The Hollow Court favor −1" || test.favor(hollow.ID) != -1 {
		t.Fatalf("hidden patron's QUEST_FAILED = %+v (note %q)", entry, entry.Dm.Note)
	}
}

func TestQuestVisibilityAndPosting(t *testing.T) {
	test := newQuestTest(t)
	hiddenNpc, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{NpcInput: core.NpcInput{Name: "Edric"}})
	if err != nil {
		t.Fatal(err)
	}
	hiddenPatron := test.patron("The Hollow Court", false)
	visible := test.quest(core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{
		Title: "Clear the Wolf Den", GiverNpcID: &hiddenNpc.ID, PatronID: &hiddenPatron.ID, RewardFavor: 2,
	}})
	quiet := test.quest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{Title: "Mend the Nets"}})
	hidden := test.quest(core.NewQuestInput{PostToTimeline: true, QuestInput: core.QuestInput{Title: "Find the Cove", DmNotes: "quest-dm-canary"}})
	if posted := test.entries(test.player, core.TimelineKindQuestPosted); len(posted) != 1 || *posted[0].QuestID != visible.ID || posted[0].Title != "New quest: Clear the Wolf Den" {
		t.Fatalf("QUEST_POSTED = %+v", posted)
	}

	list, err := test.service.ListQuests(test.context, test.player)
	if err != nil || len(list.Quests) != 2 {
		t.Fatalf("the player lists %d quests, want the 2 visible ones (%v)", len(list.Quests), err)
	}
	for _, quest := range list.Quests {
		if quest.GiverNpc != nil || quest.Patron != nil || quest.RewardFavor != 0 || quest.Dm != nil {
			t.Fatalf("a hidden giver or patron reached the player: %+v", quest)
		}
	}
	_, hiddenErr := test.service.GetQuest(test.context, test.player, hidden.ID)
	_, missingErr := test.service.GetQuest(test.context, test.player, uuid.New())
	if !isKind(hiddenErr, core.KindNotFound) || hiddenErr.Error() != missingErr.Error() {
		t.Fatalf("a hidden quest = %v; a missing one = %v", hiddenErr, missingErr)
	}
	for _, viewer := range []core.Viewer{test.player, test.dmAsPlayer()} {
		if seen, _ := test.service.ListQuests(test.context, viewer); len(seen.Quests) != 2 {
			t.Fatalf("view as player lists %d quests", len(seen.Quests))
		}
	}

	// Edits never carry visibility; the switch posts when the quest becomes visible.
	if _, err := test.service.UpdateQuest(test.context, test.dm, hidden.ID, core.QuestInput{Title: "Find the Smugglers' Cove"}); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.GetQuest(test.context, test.player, hidden.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("an edit revealed the quest: %v", err)
	}
	if _, err := test.service.SetQuestRevealed(test.context, test.dm, hidden.ID, core.QuestRevealInput{Revealed: true, PostToTimeline: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetQuestRevealed(test.context, test.dm, quiet.ID, core.QuestRevealInput{Revealed: true, PostToTimeline: true}); err != nil {
		t.Fatal(err)
	}
	if posted := test.entries(test.player, core.TimelineKindQuestPosted); len(posted) != 2 {
		t.Fatalf("after revealing: %d QUEST_POSTED entries, want 2 (an already visible quest posts nothing)", len(posted))
	}

	// Hiding a quest takes its entries off the players' timeline; deleting it then keeps them there.
	if _, err := test.service.SetQuestRevealed(test.context, test.dm, visible.ID, core.QuestRevealInput{}); err != nil {
		t.Fatal(err)
	}
	for _, entry := range test.entries(test.player, core.TimelineKindQuestPosted) {
		if entry.QuestID != nil && *entry.QuestID == visible.ID {
			t.Fatal("a hidden quest's posting reached the player")
		}
	}
	if err := test.service.DeleteQuest(test.context, test.dm, visible.ID); err != nil {
		t.Fatal(err)
	}
	if posted := test.entries(test.player, core.TimelineKindQuestPosted); len(posted) != 1 {
		t.Fatalf("after deleting the hidden quest the player sees %d postings, want 1", len(posted))
	}
	if dmPosted := test.entries(test.dm, core.TimelineKindQuestPosted); len(dmPosted) != 2 {
		t.Fatalf("the DM keeps %d postings, want 2", len(dmPosted))
	}

	// Favor needs a patron giver.
	_, err = test.service.CreateQuest(test.context, test.dm, core.NewQuestInput{QuestInput: core.QuestInput{Title: "Unpaid", FailureFavor: 1}})
	if !isKind(err, core.KindInvalid) {
		t.Fatalf("favor without a patron = %v", err)
	}
}

// SPEC §5.12 Example G, end to end through core.
func TestExampleGRequestAndApproval(t *testing.T) {
	test := newQuestTest(t)
	test.setHeadcounts(map[string]int32{"UNASSIGNED": 4})
	test.correctDay(12)
	test.setTo(map[string]int64{"food": 20})
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 5)
	grainBarge := test.action(vex.ID, "Grain Barge", 3, effects(map[string]int64{"food": 7}, 0), true)

	asked, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID, Note: "We're hungry"})
	if err != nil || asked.Status != core.RequestStatusPending || asked.Title != "Grain Barge" || asked.FavorCost != 3 || asked.RequestedBy.Name != "Thorin" {
		t.Fatalf("request = %+v, %v", asked, err)
	}
	if prompt := test.recorder.prompts[0]; prompt.RequesterName != "Thorin" || prompt.Title != "Grain Barge" || prompt.PatronName != "Lady Vex" ||
		prompt.Terms != "3 favor → Food +7" || prompt.FavorNow != "5" {
		t.Fatalf("Beny Bot's prompt = %+v", prompt)
	}

	granted := test.must(test.service.ApproveRequest(test.context, test.dm, asked.ID, core.ApproveRequestInput{}))
	if test.favor(vex.ID) != 2 || test.amount("food") != 27 {
		t.Fatalf("after Grant: favor %d, food %d", test.favor(vex.ID), test.amount("food"))
	}
	entry := granted.Entries[0]
	if entry.Kind != core.TimelineKindPatronAction || entry.Title != "Thorin called in “Grain Barge” from Lady Vex" || entry.Body != "Food +7 (favor −3)" ||
		entry.Dm.Visibility != core.TimelineVisibilityPublic || *entry.RequestID != asked.ID {
		t.Fatalf("PATRON_ACTION = %+v", entry)
	}
	if outcome := test.recorder.lastOutcome(); outcome.Text != "Granted · Food +7 · favor 5 → 2" || outcome.DiscordMessageID != "message-"+asked.ID.String() {
		t.Fatalf("Beny Bot's edit = %+v", outcome)
	}
	_, err = test.service.ApproveRequest(test.context, test.dm, asked.ID, core.ApproveRequestInput{})
	requireMessage(t, err, core.KindConflict, "That request was already decided.")

	_, err = test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
	requireMessage(t, err, core.KindInvalid, "Not enough favor: Lady Vex has 2, needs 3.")

	// The variant: two requests made while favor is 5; the second approval is refused, then declined.
	test.setFavor(vex.ID, 5)
	first, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
	if err != nil {
		t.Fatal(err)
	}
	second, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
	if err != nil {
		t.Fatal(err)
	}
	test.must(test.service.ApproveRequest(test.context, test.dm, first.ID, core.ApproveRequestInput{}))
	_, err = test.service.ApproveRequest(test.context, test.dm, second.ID, core.ApproveRequestInput{})
	requireMessage(t, err, core.KindConflict, "Not enough favor: Lady Vex has 2, needs 3.")
	if test.favor(vex.ID) != 2 {
		t.Fatalf("a refused grant changed favor to %d", test.favor(vex.ID))
	}
	declined, err := test.service.DeclineRequest(test.context, test.dm, second.ID, core.DeclineRequestInput{Note: "Vex is busy this week."})
	if err != nil || declined.Status != core.RequestStatusDeclined || declined.DecisionNote == nil || *declined.DecisionNote != "Vex is busy this week." {
		t.Fatalf("decline = %+v, %v", declined, err)
	}
	if outcome := test.recorder.lastOutcome(); outcome.Text != "Declined" || outcome.RequestID != second.ID {
		t.Fatalf("Beny Bot's edit = %+v", outcome)
	}
}

func TestRequestRulesTweaksAndUndo(t *testing.T) {
	test := newQuestTest(t)
	test.setTo(map[string]int64{"food": 4})
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 10)
	hidden := test.action(vex.ID, "Hire Mercenaries", 2, effects(map[string]int64{"gold": 5}, 0), false)
	feast := test.action(vex.ID, "Feast", 0, effects(map[string]int64{"food": -10}, 0), true)
	grainBarge := test.action(vex.ID, "Grain Barge", 3, effects(map[string]int64{"food": 7}, 0), true)

	// STRICT on resources too, with the shortfall named; hidden and archived actions are refused.
	_, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: feast.ID})
	requireMessage(t, err, core.KindInvalid, "Not enough food: the colony has 4, needs 10.")
	if _, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: hidden.ID}); !isKind(err, core.KindInvalid) {
		t.Fatalf("asking for a hidden action = %v", err)
	}
	if _, err := test.service.SetPatronActionArchived(test.context, test.dm, feast.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: feast.ID}); !isKind(err, core.KindInvalid) {
		t.Fatalf("asking for an archived action = %v", err)
	}
	actions, err := test.service.ListPatronActions(test.context, test.player)
	if err != nil || len(actions) != 1 || actions[0].ID != grainBarge.ID || actions[0].Dm != nil {
		t.Fatalf("the player's actions = %+v, %v", actions, err)
	}

	// A tweak applies to this grant only; the request still shows what was asked.
	asked, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
	if err != nil {
		t.Fatal(err)
	}
	tweak := effects(map[string]int64{"food": 10}, 0)
	preview, err := test.service.PreviewApproveRequest(test.context, test.dm, asked.ID, core.ApproveRequestInput{AppliedEffects: &tweak})
	if err != nil || !preview.Affordable || preview.Summary != "Lady Vex favor 10 → 7 · Food +10" {
		t.Fatalf("approval preview = %+v, %v", preview, err)
	}
	granted := test.must(test.service.ApproveRequest(test.context, test.dm, asked.ID, core.ApproveRequestInput{AppliedEffects: &tweak, Note: "Extra from the docks"}))
	request, err := test.service.GetRequest(test.context, test.player, asked.ID)
	if err != nil || request.Effects.Resources["food"] != 7 || request.AppliedEffects == nil || request.AppliedEffects.Resources["food"] != 10 || test.amount("food") != 14 {
		t.Fatalf("tweaked grant = %+v, food %d, %v", request, test.amount("food"), err)
	}

	// Undo of a grant: favor and stock come back and the request waits again; Beny Bot marks it.
	test.undo(granted)
	request, err = test.service.GetRequest(test.context, test.dm, asked.ID)
	if err != nil || request.Status != core.RequestStatusPending || request.AppliedEffects != nil || test.favor(vex.ID) != 10 || test.amount("food") != 4 {
		t.Fatalf("after undo: %+v, favor %d, food %d, %v", request.Status, test.favor(vex.ID), test.amount("food"), err)
	}
	if outcome := test.recorder.lastOutcome(); outcome.Text != "Undone" || !outcome.Undone || outcome.RequestID != asked.ID {
		t.Fatalf("Beny Bot's edit after undo = %+v", outcome)
	}

	// Withdraw is the requester's own; anyone else gets not found.
	_, otherPlayer := test.fixture.player(test.dm, "seed:bram", "bram", true)
	if _, err := test.service.WithdrawRequest(test.context, otherPlayer, asked.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("someone else's withdraw = %v", err)
	}
	withdrawn, err := test.service.WithdrawRequest(test.context, test.player, asked.ID)
	if err != nil || withdrawn.Status != core.RequestStatusWithdrawn {
		t.Fatalf("withdraw = %+v, %v", withdrawn, err)
	}
	if _, err := test.service.ApproveRequest(test.context, test.dm, asked.ID, core.ApproveRequestInput{}); !isKind(err, core.KindConflict) {
		t.Fatalf("approving a withdrawn request = %v", err)
	}

	// Use action now: on behalf of Thorin, a hidden action credits him in a DM-only entry; undo removes it.
	used := test.must(test.service.UseActionNow(test.context, test.dm, core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: hidden.ID, OnBehalfOf: ptr(test.player.MemberID())}))
	entry := used.Entries[0]
	if entry.Title != "Thorin called in “Hire Mercenaries” from Lady Vex" || entry.Dm.Visibility != core.TimelineVisibilityDm || test.favor(vex.ID) != 8 {
		t.Fatalf("use action now = %+v, favor %d", entry, test.favor(vex.ID))
	}
	usedRequest := *entry.RequestID
	if _, err := test.service.GetRequest(test.context, test.player, usedRequest); !isKind(err, core.KindNotFound) {
		t.Fatalf("a request for a hidden action reached the player: %v", err)
	}
	test.undo(used)
	if _, err := test.service.GetRequest(test.context, test.dm, usedRequest); !isKind(err, core.KindNotFound) || test.favor(vex.ID) != 10 {
		t.Fatalf("undoing use action now kept the request (%v) or favor %d", err, test.favor(vex.ID))
	}
	visibleUse := test.must(test.service.UseActionNow(test.context, test.dm, core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: grainBarge.ID, OnBehalfOf: ptr(test.player.MemberID())}))
	if entry := visibleUse.Entries[0]; entry.Title != "Thorin called in “Grain Barge” from Lady Vex" || entry.Dm.Visibility != core.TimelineVisibilityPublic {
		t.Fatalf("use action now, visible = %+v", entry)
	}
}

// Requests are the players'; a grant that can't happen previews no after; a request made while its patron
// or action was hidden, and an undone Use action now, never name a hidden row to players later.
func TestRequestsNeverNameWhatPlayersCouldNotSee(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 2)
	grainBarge := test.action(vex.ID, "Grain Barge", 3, effects(map[string]int64{"food": 7}, 0), true)

	for _, dm := range []core.Viewer{test.dm, test.dmAsPlayer()} {
		_, err := test.service.CreateRequest(test.context, dm, core.RequestInput{ActionID: grainBarge.ID})
		requireMessage(t, err, core.KindForbidden, "Use action now to use an action yourself.")
	}

	test.setFavor(vex.ID, 5)
	asked, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
	if err != nil {
		t.Fatal(err)
	}
	test.setFavor(vex.ID, 2)
	preview, err := test.service.PreviewApproveRequest(test.context, test.dm, asked.ID, core.ApproveRequestInput{})
	if err != nil || preview.Affordable || preview.Shortfall != "Not enough favor: Lady Vex has 2, needs 3" || preview.Favor != nil ||
		preview.Summary != "" || len(preview.Resources) != 0 {
		t.Fatalf("an unaffordable preview = %+v, %v", preview, err)
	}

	// Used while hidden, then renamed and revealed: players never learn the old names.
	court := test.patron("The Hollow Court", false)
	test.setFavor(court.ID, 5)
	whispers := test.action(court.ID, "Whisper Network", 2, effects(map[string]int64{"gold": 1}, 0), false)
	used := test.must(test.service.UseActionNow(test.context, test.dm, core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: whispers.ID}))
	usedRequest := *used.Entries[0].RequestID
	if _, err := test.service.UpdatePatron(test.context, test.dm, court.ID, core.PatronInput{Name: "The Merchant Guild"}); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.UpdatePatronAction(test.context, test.dm, whispers.ID, core.PatronActionInput{Name: "Courier Service", FavorCost: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetPatronRevealed(test.context, test.dm, court.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetPatronActionRevealed(test.context, test.dm, whispers.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, player := range []core.Viewer{test.player, test.dmAsPlayer()} {
		if _, err := test.service.GetRequest(test.context, player, usedRequest); !isKind(err, core.KindNotFound) {
			t.Fatalf("a request made while hidden reached a player: %v", err)
		}
		list, err := test.service.ListRequests(test.context, player, core.RequestFilter{PatronID: &court.ID})
		if err != nil || len(list.Requests) != 0 {
			t.Fatalf("the revealed patron's requests for a player = %+v, %v", list.Requests, err)
		}
	}
	if request, err := test.service.GetRequest(test.context, test.dm, usedRequest); err != nil || request.Title != "Whisper Network" {
		t.Fatalf("the DM's request = %+v, %v", request, err)
	}

	// Undoing Use action now deletes its request, so the UNDO entry can't follow the action once it is
	// hidden: it doesn't name it.
	test.setFavor(vex.ID, 5)
	rowboat := test.action(vex.ID, "Rowboat", 1, effects(map[string]int64{"food": 1}, 0), true)
	undone := test.undo(test.must(test.service.UseActionNow(test.context, test.dm, core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: rowboat.ID, OnBehalfOf: ptr(test.player.MemberID())})))
	if entry := undone.Entries[0]; entry.Title != "The DM rewound a change to the colony" || entry.Dm.Visibility != core.TimelineVisibilityPublic ||
		entry.RequestID != nil || entry.PatronID == nil || *entry.PatronID != vex.ID {
		t.Fatalf("UNDO of Use action now = %+v", entry)
	}
	if _, err := test.service.SetPatronActionRevealed(test.context, test.dm, rowboat.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, entry := range test.timeline(test.player, 0, test.board().Day) {
		if strings.Contains(entry.Title, "Rowboat") || strings.Contains(entry.Body, "Rowboat") {
			t.Fatalf("the player's timeline names a hidden action: %+v", entry)
		}
	}
}

// A request decided while its prompt was still being posted had no message to edit, so storing the
// message ID hands back that outcome and the prompt loses its buttons.
func TestAPromptPostedAfterItsDecisionShowsTheOutcome(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 10)
	grainBarge := test.action(vex.ID, "Grain Barge", 3, effects(map[string]int64{"food": 7}, 0), true)
	test.recorder.postsSlowly = true
	decide := map[string]func(requestID uuid.UUID) error{
		"Withdrawn": func(requestID uuid.UUID) error {
			_, err := test.service.WithdrawRequest(test.context, test.player, requestID)
			return err
		},
		"Declined": func(requestID uuid.UUID) error {
			_, err := test.service.DeclineRequest(test.context, test.dm, requestID, core.DeclineRequestInput{})
			return err
		},
		"Granted": func(requestID uuid.UUID) error {
			_, err := test.service.ApproveRequest(test.context, test.dm, requestID, core.ApproveRequestInput{})
			return err
		},
	}
	for text, decideIt := range decide {
		asked, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
		if err != nil {
			t.Fatal(err)
		}
		prompt := test.recorder.prompts[len(test.recorder.prompts)-1]
		if err := decideIt(asked.ID); err != nil {
			t.Fatal(err)
		}
		outcome, err := prompt.RecordMessage(test.context, "late-message")
		if err != nil || outcome == nil || outcome.Text != text || outcome.DiscordMessageID != "late-message" || outcome.RequestID != asked.ID {
			t.Fatalf("storing a prompt after %s = %+v, %v", text, outcome, err)
		}
	}
	waiting, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := test.recorder.prompts[len(test.recorder.prompts)-1].RecordMessage(test.context, "on-time"); err != nil || outcome != nil {
		t.Fatalf("storing a waiting request's prompt = %+v, %v", outcome, err)
	}
	if waiting.Status != core.RequestStatusPending {
		t.Fatalf("status = %s", waiting.Status)
	}
}

// Use action now makes a new request each time, so the board version is what stops a repeated submit
// (a second tab, a retry) from spending favor twice.
func TestUseActionNowRefusesARepeat(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 10)
	grainBarge := test.action(vex.ID, "Grain Barge", 3, effects(map[string]int64{"food": 7}, 0), true)
	input := core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: grainBarge.ID}
	test.must(test.service.UseActionNow(test.context, test.dm, input))
	_, err := test.service.UseActionNow(test.context, test.dm, input)
	if !isKind(err, core.KindConflict) || test.favor(vex.ID) != 7 {
		t.Fatalf("a repeated Use action now = %v, favor %d", err, test.favor(vex.ID))
	}
}

// Deleting a quest waits for the colony lock that undo and the Visible switch hold, so it neither
// deadlocks with an undo of that quest's change nor reads a revealed flag about to change.
func TestDeletingAQuestWaitsForTheColonyLock(t *testing.T) {
	test := newQuestTest(t)
	cove := test.quest(core.NewQuestInput{QuestInput: core.QuestInput{Title: "Find the Smugglers' Cove"}})
	transaction, err := test.database.Pool.Begin(test.context)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = transaction.Rollback(test.context) }()
	if _, err := store.New(transaction).GetColonyForUpdate(test.context, test.campaignID); err != nil {
		t.Fatalf("lock the colony: %v", err)
	}
	deleted := make(chan error, 1)
	go func() { deleted <- test.service.DeleteQuest(test.context, test.dm, cove.ID) }()
	select {
	case err := <-deleted:
		t.Fatalf("the delete ran while another change held the colony lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := transaction.Rollback(test.context); err != nil {
		t.Fatalf("release the lock: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Fatalf("delete after the lock: %v", err)
	}
}

// A DM-only UNDO repeats the label of what it undid, so it carries the same quest and patron links and
// never names a used action: the DM making it PUBLIC later can't show players a hidden row.
func TestADmOnlyUndoFollowsHiddenRows(t *testing.T) {
	test := newQuestTest(t)
	court := test.patron("The Hollow Court", false)
	favorUndo := test.undo(test.setFavor(court.ID, 5))
	test.setFavor(court.ID, 5)
	whispers := test.action(court.ID, "Whisper Network", 2, effects(map[string]int64{"gold": 1}, 0), false)
	actionUndo := test.undo(test.must(test.service.UseActionNow(test.context, test.dm, core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: whispers.ID})))
	cove := test.quest(core.NewQuestInput{Revealed: false, QuestInput: core.QuestInput{Title: "Find the Smugglers' Cove"}})
	questUndo := test.undo(test.must(test.service.CompleteQuest(test.context, test.dm, cove.ID)))

	if entry := actionUndo.Entries[0]; entry.Title != "The DM rewound a change to the colony" || entry.PatronID == nil || *entry.PatronID != court.ID {
		t.Fatalf("a DM-only UNDO of Use action now = %+v", entry)
	}
	if entry := questUndo.Entries[0]; entry.QuestID == nil || *entry.QuestID != cove.ID {
		t.Fatalf("a DM-only UNDO of a hidden quest's completion = %+v", entry)
	}
	for _, undo := range []core.ColonyChange{favorUndo, actionUndo, questUndo} {
		entry := undo.Entries[0]
		if entry.Dm.Visibility != core.TimelineVisibilityDm {
			t.Fatalf("an UNDO of a hidden row's change is %s", entry.Dm.Visibility)
		}
		input := core.TimelineEntryInput{Day: entry.Day, Title: entry.Title, Body: entry.Body, Visibility: core.TimelineVisibilityPublic}
		if _, err := test.service.UpdateTimelineEntry(test.context, test.dm, entry.ID, input); err != nil {
			t.Fatal(err)
		}
	}
	for _, player := range []core.Viewer{test.player, test.dmAsPlayer()} {
		for _, entry := range test.timeline(player, 0, test.board().Day) {
			for _, secret := range []string{"Hollow Court", "Whisper Network", "Smugglers"} {
				if strings.Contains(entry.Title, secret) || strings.Contains(entry.Body, secret) {
					t.Fatalf("a player's timeline names %s: %+v", secret, entry)
				}
			}
		}
	}
}

// A favor change's patron lock leaves the patron's key alone, so a quest edit naming that patron (its
// foreign key check) neither waits for it nor deadlocks with an undo, which locks patrons before quests.
func TestAQuestEditDoesNotWaitForAFavorLock(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	quest := test.quest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{Title: "Clear the Wolf Den"}})
	transaction, err := test.database.Pool.Begin(test.context)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = transaction.Rollback(test.context) }()
	if _, err := store.New(transaction).GetPatronForUpdate(test.context, store.GetPatronForUpdateParams{CampaignID: test.campaignID, ID: vex.ID}); err != nil {
		t.Fatal(err)
	}
	editContext, cancel := context.WithTimeout(test.context, 3*time.Second)
	defer cancel()
	if _, err := test.service.UpdateQuest(editContext, test.dm, quest.ID, core.QuestInput{Title: "Clear the Wolf Den", PatronID: &vex.ID}); err != nil {
		t.Fatalf("a quest edit naming a patron whose favor is locked: %v", err)
	}
}

func TestDecisionNoteReachesOnlyTheRequesterAndTheDm(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 9)
	grainBarge := test.action(vex.ID, "Grain Barge", 3, effects(map[string]int64{"food": 7}, 0), true)
	_, otherPlayer := test.fixture.player(test.dm, "seed:bram", "bram", true)
	asked, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
	if err != nil {
		t.Fatal(err)
	}
	const noteCanary = "decline-note-canary"
	if _, err := test.service.DeclineRequest(test.context, test.dm, asked.ID, core.DeclineRequestInput{Note: noteCanary}); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		name     string
		viewer   core.Viewer
		readable bool
	}{{"requester", test.player, true}, {"DM", test.dm, true}, {"another player", otherPlayer, false}, {"DM viewing as player", test.dmAsPlayer(), false}} {
		list, err := test.service.ListRequests(test.context, check.viewer, core.RequestFilter{})
		if err != nil || len(list.Requests) != 1 || list.Requests[0].Status != core.RequestStatusDeclined {
			t.Fatalf("%s: requests %+v, %v", check.name, list, err)
		}
		single, err := test.service.GetRequest(test.context, check.viewer, asked.ID)
		if err != nil {
			t.Fatal(err)
		}
		page, err := test.service.GetPatron(test.context, check.viewer, vex.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, request := range []core.Request{list.Requests[0], single, page.Requests[0]} {
			encoded, _ := json.Marshal(request)
			if strings.Contains(string(encoded), noteCanary) != check.readable {
				t.Fatalf("%s reads %s", check.name, encoded)
			}
		}
	}
	mine, err := test.service.ListRequests(test.context, otherPlayer, core.RequestFilter{Mine: true})
	if err != nil || len(mine.Requests) != 0 {
		t.Fatalf("another player's own requests = %+v, %v", mine, err)
	}
}

func TestPatronsFavorAndSecrecy(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	hollow := test.patron("The Hollow Court", false)
	test.action(hollow.ID, "Whisper Network", 1, core.Effects{}, true)
	archived := test.patron("Captain Orrin", true)
	if _, err := test.service.SetPatronArchived(test.context, test.dm, archived.ID, true); err != nil {
		t.Fatal(err)
	}

	// DM adjustments are unbounded, DM-only by default, and PUBLIC only while the patron is revealed.
	below := test.must(test.service.AdjustFavor(test.context, test.dm, vex.ID, core.FavorAdjustInput{ExpectedVersion: test.board().Version, Delta: ptr(int64(-2)), Public: true}))
	if test.favor(vex.ID) != -2 || below.Entries[0].Dm.Visibility != core.TimelineVisibilityPublic || below.Entries[0].Body != "Lady Vex favor 0 → −2" {
		t.Fatalf("adjust = %+v, favor %d", below.Entries[0], test.favor(vex.ID))
	}
	hiddenAdjust := test.must(test.service.AdjustFavor(test.context, test.dm, hollow.ID, core.FavorAdjustInput{ExpectedVersion: test.board().Version, Delta: ptr(int64(4)), Public: true}))
	if hiddenAdjust.Entries[0].Dm.Visibility != core.TimelineVisibilityDm {
		t.Fatal("a hidden patron's adjustment went PUBLIC")
	}
	_, err := test.service.AdjustFavor(test.context, test.dm, vex.ID, core.FavorAdjustInput{ExpectedVersion: test.board().Version - 1, Delta: ptr(int64(1))})
	if !isKind(err, core.KindConflict) {
		t.Fatalf("a stale favor adjustment = %v", err)
	}
	test.undo(hiddenAdjust)
	if test.favor(hollow.ID) != 0 {
		t.Fatalf("undo left favor at %d", test.favor(hollow.ID))
	}

	list, err := test.service.ListPatrons(test.context, test.player)
	if err != nil || len(list.Patrons) != 1 || list.Patrons[0].ID != vex.ID || list.Patrons[0].Favor != -2 || list.Patrons[0].Dm != nil {
		t.Fatalf("the player's patrons = %+v, %v", list, err)
	}
	if _, err := test.service.GetPatron(test.context, test.player, hollow.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("a hidden patron's page = %v", err)
	}
	if page, err := test.service.GetPatron(test.context, test.player, archived.ID); err != nil || !page.Patron.Archived {
		t.Fatalf("an archived patron's page keeps its history: %+v, %v", page, err)
	}
	if actions, _ := test.service.ListPatronActions(test.context, test.player); len(actions) != 0 {
		t.Fatalf("a hidden patron's action reached the player: %+v", actions)
	}

	// Hiding a patron takes the entries that name them off the players' timeline.
	if seen := test.entries(test.player, core.TimelineKindFavorAdjusted); len(seen) != 1 {
		t.Fatalf("the player sees %d favor entries, want 1", len(seen))
	}
	if _, err := test.service.SetPatronRevealed(test.context, test.dm, vex.ID, false); err != nil {
		t.Fatal(err)
	}
	if seen := test.entries(test.player, core.TimelineKindFavorAdjusted); len(seen) != 0 {
		t.Fatal("an entry naming a hidden patron reached the player")
	}
	// Edits never carry visibility.
	page, err := test.service.UpdatePatron(test.context, test.dm, vex.ID, core.PatronInput{Name: "Lady Vex", DmNotes: "patron-dm-canary"})
	if err != nil || page.Patron.Dm.Revealed {
		t.Fatalf("an edit revealed the patron: %+v, %v", page.Patron.Dm, err)
	}
}

func TestQuestsPatronsAndRequestsRefuseAnotherCampaignsIDs(t *testing.T) {
	test := newQuestTest(t)
	otherDmCaller := test.caller("600000000000000099", "other dm")
	_, otherDm := test.campaign(otherDmCaller, "Other", "other-campaign")
	if _, err := test.service.CreateResource(test.context, otherDm, core.NewResourceInput{Label: "Brine", Icon: core.ColonyIconDroplets}); err != nil {
		t.Fatal(err)
	}
	otherPatronPage, err := test.service.CreatePatron(test.context, otherDm, core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{Name: "Ridge Council"}})
	if err != nil {
		t.Fatal(err)
	}
	otherPatron := otherPatronPage.Patron.ID
	otherAction, err := test.service.CreatePatronAction(test.context, otherDm, core.NewPatronActionInput{PatronID: otherPatron, Revealed: true, PatronActionInput: core.PatronActionInput{Name: "Timber Levy"}})
	if err != nil {
		t.Fatal(err)
	}
	otherNpc, err := test.service.CreateNpc(test.context, otherDm, core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Dagny"}})
	if err != nil {
		t.Fatal(err)
	}
	otherQuest, err := test.service.CreateQuest(test.context, otherDm, core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{Title: "Hold the Ridge"}})
	if err != nil {
		t.Fatal(err)
	}
	otherBoard, err := test.service.GetColonyBoard(test.context, otherDm)
	if err != nil {
		t.Fatal(err)
	}
	otherRequest, err := test.service.UseActionNow(test.context, otherDm, core.UseActionInput{ExpectedVersion: otherBoard.Version, ActionID: otherAction.ID})
	if err != nil {
		t.Fatal(err)
	}
	otherRequestID := *otherRequest.Entries[0].RequestID
	otherMember := otherDm.MemberID()
	vex := test.patron("Lady Vex", true)
	action := test.action(vex.ID, "Grain Barge", 0, core.Effects{}, true)
	asked, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: action.ID})
	if err != nil {
		t.Fatal(err)
	}
	version := test.board().Version

	for name, attempt := range map[string]func() error{
		"quest patron": func() error {
			_, err := test.service.CreateQuest(test.context, test.dm, core.NewQuestInput{QuestInput: core.QuestInput{Title: "x", PatronID: &otherPatron}})
			return err
		},
		"quest giver": func() error {
			_, err := test.service.CreateQuest(test.context, test.dm, core.NewQuestInput{QuestInput: core.QuestInput{Title: "x", GiverNpcID: &otherNpc.ID}})
			return err
		},
		"quest reward key": func() error {
			_, err := test.service.CreateQuest(test.context, test.dm, core.NewQuestInput{QuestInput: core.QuestInput{Title: "x", RewardEffects: effects(map[string]int64{"brine": 1}, 0)}})
			return err
		},
		"quest page": func() error { _, err := test.service.GetQuest(test.context, test.dm, otherQuest.ID); return err },
		"complete":   func() error { _, err := test.service.CompleteQuest(test.context, test.dm, otherQuest.ID); return err },
		"fail":       func() error { _, err := test.service.FailQuest(test.context, test.dm, otherQuest.ID); return err },
		"reveal quest": func() error {
			_, err := test.service.SetQuestRevealed(test.context, test.dm, otherQuest.ID, core.QuestRevealInput{Revealed: true})
			return err
		},
		"delete quest": func() error { return test.service.DeleteQuest(test.context, test.dm, otherQuest.ID) },
		"patron page":  func() error { _, err := test.service.GetPatron(test.context, test.dm, otherPatron); return err },
		"adjust favor": func() error {
			_, err := test.service.AdjustFavor(test.context, test.dm, otherPatron, core.FavorAdjustInput{ExpectedVersion: version, Delta: ptr(int64(1))})
			return err
		},
		"archive patron": func() error {
			_, err := test.service.SetPatronArchived(test.context, test.dm, otherPatron, true)
			return err
		},
		"action of patron": func() error {
			_, err := test.service.CreatePatronAction(test.context, test.dm, core.NewPatronActionInput{PatronID: otherPatron, PatronActionInput: core.PatronActionInput{Name: "x"}})
			return err
		},
		"action effects key": func() error {
			_, err := test.service.CreatePatronAction(test.context, test.dm, core.NewPatronActionInput{PatronID: vex.ID, PatronActionInput: core.PatronActionInput{Name: "x", Effects: effects(map[string]int64{"brine": 1}, 0)}})
			return err
		},
		"edit action": func() error {
			_, err := test.service.UpdatePatronAction(test.context, test.dm, otherAction.ID, core.PatronActionInput{Name: "x"})
			return err
		},
		"request action": func() error {
			_, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: otherAction.ID})
			return err
		},
		"approve": func() error {
			_, err := test.service.ApproveRequest(test.context, test.dm, otherRequestID, core.ApproveRequestInput{})
			return err
		},
		"decline": func() error {
			_, err := test.service.DeclineRequest(test.context, test.dm, otherRequestID, core.DeclineRequestInput{})
			return err
		},
		"withdraw": func() error {
			_, err := test.service.WithdrawRequest(test.context, test.player, otherRequestID)
			return err
		},
		"discord grant": func() error {
			_, err := test.service.DecideRequestFromDiscord(test.context, test.dm, otherRequestID, true)
			return err
		},
		"read request": func() error { _, err := test.service.GetRequest(test.context, test.dm, otherRequestID); return err },
		"use action": func() error {
			_, err := test.service.UseActionNow(test.context, test.dm, core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: otherAction.ID})
			return err
		},
		"on behalf of": func() error {
			_, err := test.service.UseActionNow(test.context, test.dm, core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: action.ID, OnBehalfOf: &otherMember})
			return err
		},
		"grant tweak key": func() error {
			_, err := test.service.ApproveRequest(test.context, test.dm, asked.ID, core.ApproveRequestInput{AppliedEffects: ptr(effects(map[string]int64{"brine": 1}, 0))})
			return err
		},
		"preview tweak key": func() error {
			_, err := test.service.PreviewApproveRequest(test.context, test.dm, asked.ID, core.ApproveRequestInput{AppliedEffects: ptr(effects(map[string]int64{"brine": 1}, 0))})
			return err
		},
	} {
		err := attempt()
		if !isKind(err, core.KindNotFound) && !isKind(err, core.KindInvalid) {
			t.Errorf("%s with another campaign's ID or key = %v, want 404 or 422", name, err)
		}
	}
	if test.favor(vex.ID) != 0 {
		t.Fatal("a refused operation changed favor")
	}
	// A player may never decide, on the website or in Discord.
	if _, err := test.service.DecideRequestFromDiscord(test.context, test.player, asked.ID, true); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player's Grant = %v", err)
	}
	outcome, err := test.service.DecideRequestFromDiscord(test.context, test.dm, asked.ID, true)
	if err != nil || outcome.Status != core.RequestStatusApproved || outcome.DiscordMessageID != "message-"+asked.ID.String() {
		t.Fatalf("the DM's Grant = %+v, %v", outcome, err)
	}
}

func ptr[Value any](value Value) *Value {
	return &value
}

func requireFieldMessage(t *testing.T, err error, field string, message string) {
	t.Helper()
	operationError, typed := core.AsError(err)
	if !typed || operationError.Kind != core.KindInvalid || len(operationError.Fields) != 1 ||
		operationError.Fields[0].Field != field || operationError.Fields[0].Message != message {
		t.Fatalf("got %v; want the %s field error %q", err, field, message)
	}
}

// Deleting a quest after completing it must not trap the stock and favor its completion paid out:
// undo puts those back and leaves the deleted quest deleted.
func TestUndoAfterTheQuestWasDeletedRestoresTheRest(t *testing.T) {
	test := newQuestTest(t)
	test.setTo(map[string]int64{"food": 10})
	vex := test.patron("Lady Vex", true)
	escort := test.quest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{
		Title: "Escort", PatronID: &vex.ID, RewardFavor: 2, RewardEffects: effects(map[string]int64{"food": 5}, 0),
	}})
	completed := test.must(test.service.CompleteQuest(test.context, test.dm, escort.ID))
	if err := test.service.DeleteQuest(test.context, test.dm, escort.ID); err != nil {
		t.Fatal(err)
	}
	undone := test.undo(completed)
	if test.amount("food") != 10 || test.favor(vex.ID) != 0 || len(undone.Entries) != 1 || undone.Entries[0].Kind != core.TimelineKindUndo {
		t.Fatalf("undo: food %d, favor %d, entries %+v", test.amount("food"), test.favor(vex.ID), undone.Entries)
	}
	if _, err := test.service.GetQuest(test.context, test.dm, escort.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("undo brought the deleted quest back: %v", err)
	}
}

func TestUndoingAReopenRestoresTheFailure(t *testing.T) {
	test := newQuestTest(t)
	test.correctDay(5)
	ledger := test.quest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{Title: "Recover the Ledger"}})
	test.must(test.service.FailQuest(test.context, test.dm, ledger.ID))
	test.correctDay(9)
	reopened := test.must(test.service.ReopenQuest(test.context, test.dm, ledger.ID))
	test.undo(reopened)
	quest, err := test.service.GetQuest(test.context, test.dm, ledger.ID)
	if err != nil || quest.Status != core.QuestStatusFailed || quest.ResolvedDay == nil || *quest.ResolvedDay != 5 || quest.ResolvedAt == nil {
		t.Fatalf("after undoing Reopen: %+v, %v", quest, err)
	}
	if test.board().Day != 9 || len(test.entries(test.player, core.TimelineKindQuestReopened)) != 0 {
		t.Fatalf("undoing Reopen: day %d, the player still sees the reopening", test.board().Day)
	}
}

func TestAFavorAdjustmentChangesExactlyOneWay(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 3)
	for _, input := range []core.FavorAdjustInput{{Delta: ptr(int64(1)), SetTo: ptr(int64(4))}, {}} {
		input.ExpectedVersion = test.board().Version
		_, err := test.service.AdjustFavor(test.context, test.dm, vex.ID, input)
		requireFieldMessage(t, err, "delta", "Give either a change or a new amount.")
	}
	for _, input := range []core.FavorAdjustInput{{SetTo: ptr(int64(3))}, {Delta: ptr(int64(0))}} {
		input.ExpectedVersion = test.board().Version
		_, err := test.service.AdjustFavor(test.context, test.dm, vex.ID, input)
		requireMessage(t, err, core.KindInvalid, "Nothing would change: Lady Vex already has 3 favor.")
	}
	if test.favor(vex.ID) != 3 {
		t.Fatalf("a refused adjustment left favor %d", test.favor(vex.ID))
	}
}

func TestQuestLootIsChecked(t *testing.T) {
	test := newQuestTest(t)
	tooMany := make([]core.LootItem, 51)
	for index := range tooMany {
		tooMany[index] = core.LootItem{Name: "Coin", Quantity: 1}
	}
	for _, check := range []struct {
		loot    []core.LootItem
		message string
	}{
		{[]core.LootItem{{Name: "  ", Quantity: 1}}, "Name every item."},
		{[]core.LootItem{{Name: "Wolf pelts", Quantity: 0}}, "Each quantity is a whole number from 1 to 1,000,000."},
		{[]core.LootItem{{Name: "Wolf pelts", Quantity: 1_000_001}}, "Each quantity is a whole number from 1 to 1,000,000."},
		{[]core.LootItem{{Name: "Wolf\tpelts", Quantity: 1}}, "Remove the tabs, line breaks or other control characters."},
		{tooMany, "List at most 50 items."},
	} {
		_, err := test.service.CreateQuest(test.context, test.dm, core.NewQuestInput{QuestInput: core.QuestInput{Title: "Hunt", Loot: check.loot}})
		requireFieldMessage(t, err, "loot", check.message)
	}
	quest := test.quest(core.NewQuestInput{QuestInput: core.QuestInput{Title: "Hunt", Loot: []core.LootItem{{Name: " Wolf pelts ", Quantity: 6, Description: " Fresh "}}}})
	if len(quest.Loot) != 1 || quest.Loot[0] != (core.LootItem{Name: "Wolf pelts", Quantity: 6, Description: "Fresh"}) {
		t.Fatalf("loot = %+v", quest.Loot)
	}
}

// Quests are deleted, but patrons and actions are only archived, so archived ones don't count toward
// their limits: otherwise the DM could never get back under one.
func TestQuestPatronAndActionLimits(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	plant := func(statement string, arguments ...any) {
		t.Helper()
		if _, err := test.database.Pool.Exec(test.context, statement, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	plant(`INSERT INTO quest (campaign_id, title) SELECT $1, 'Planted ' || n FROM generate_series(1, $2::int) AS n`, test.campaignID, core.QuestLimit)
	plant(`INSERT INTO patron (campaign_id, name) SELECT $1, 'Planted ' || n FROM generate_series(1, $2::int) AS n`, test.campaignID, core.PatronLimit-1)
	plant(`INSERT INTO patron_action (campaign_id, patron_id, name) SELECT $1, $2, 'Planted ' || n FROM generate_series(1, $3::int) AS n`,
		test.campaignID, vex.ID, core.PatronActionLimit)

	_, err := test.service.CreateQuest(test.context, test.dm, core.NewQuestInput{QuestInput: core.QuestInput{Title: "One too many"}})
	requireMessage(t, err, core.KindConflict, "This campaign has 2,000 quests, the most it can hold. Delete one first.")
	_, err = test.service.CreatePatron(test.context, test.dm, core.NewPatronInput{PatronInput: core.PatronInput{Name: "Ridge Council"}})
	requireMessage(t, err, core.KindConflict, "This campaign has 500 patrons, the most it can hold. Archive one first.")
	_, err = test.service.CreatePatronAction(test.context, test.dm, core.NewPatronActionInput{PatronID: vex.ID, PatronActionInput: core.PatronActionInput{Name: "Grain Barge"}})
	requireMessage(t, err, core.KindConflict, "This campaign has 2,000 patron actions, the most it can hold. Archive one first.")

	quests, err := test.service.ListQuests(test.context, test.dm)
	if err != nil {
		t.Fatal(err)
	}
	if err := test.service.DeleteQuest(test.context, test.dm, quests.Quests[0].ID); err != nil {
		t.Fatal(err)
	}
	test.quest(core.NewQuestInput{QuestInput: core.QuestInput{Title: "In its place"}})
	if _, err := test.service.SetPatronArchived(test.context, test.dm, vex.ID, true); err != nil {
		t.Fatal(err)
	}
	ridge := test.patron("Ridge Council", true)
	actions, err := test.service.ListPatronActions(test.context, test.dm)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetPatronActionArchived(test.context, test.dm, actions[0].ID, true); err != nil {
		t.Fatal(err)
	}
	timberLevy := test.action(ridge.ID, "Timber Levy", 0, effects(nil, 0), true)

	// Otherwise archive, create and unarchive would take the active count past the limit.
	_, err = test.service.SetPatronArchived(test.context, test.dm, vex.ID, false)
	requireMessage(t, err, core.KindConflict, "This campaign has 500 patrons, the most it can hold. Archive one first.")
	_, err = test.service.SetPatronActionArchived(test.context, test.dm, actions[0].ID, false)
	requireMessage(t, err, core.KindConflict, "This campaign has 2,000 patron actions, the most it can hold. Archive one first.")
	if page, err := test.service.GetPatron(test.context, test.dm, vex.ID); err != nil || !page.Patron.Archived {
		t.Fatalf("a refused unarchive left Lady Vex archived = %v, %v", page.Patron.Archived, err)
	}
	if _, err := test.service.SetPatronArchived(test.context, test.dm, ridge.ID, false); err != nil {
		t.Fatalf("unarchiving a patron who isn't archived = %v, want no change", err)
	}
	if _, err := test.service.SetPatronActionArchived(test.context, test.dm, timberLevy.ID, false); err != nil {
		t.Fatalf("unarchiving an action that isn't archived = %v, want no change", err)
	}

	// Archived rows still count toward a higher ceiling, so a create-and-archive loop can't grow the tables forever.
	plant(`INSERT INTO patron_action (campaign_id, patron_id, name, archived) SELECT $1, $2, 'Archived ' || n, true FROM generate_series(1, $3::int) AS n`,
		test.campaignID, vex.ID, core.PatronActionLimitIncludingArchived-core.PatronActionLimit-1)
	if _, err := test.service.SetPatronActionArchived(test.context, test.dm, timberLevy.ID, true); err != nil {
		t.Fatal(err)
	}
	_, err = test.service.CreatePatronAction(test.context, test.dm, core.NewPatronActionInput{PatronID: ridge.ID, PatronActionInput: core.PatronActionInput{Name: "Ford Toll"}})
	requireMessage(t, err, core.KindConflict, "This campaign has kept 20,000 patron actions, archived ones included, the most it can hold.")
	plant(`INSERT INTO patron (campaign_id, name, archived) SELECT $1, 'Archived ' || n, true FROM generate_series(1, $2::int) AS n`,
		test.campaignID, core.PatronLimitIncludingArchived-core.PatronLimit-1)
	if _, err := test.service.SetPatronArchived(test.context, test.dm, ridge.ID, true); err != nil {
		t.Fatal(err)
	}
	_, err = test.service.CreatePatron(test.context, test.dm, core.NewPatronInput{PatronInput: core.PatronInput{Name: "Salt Guild"}})
	requireMessage(t, err, core.KindConflict, "This campaign has kept 5,000 patrons, archived ones included, the most it can hold.")
}

func TestDecliningFromDiscord(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	grainBarge := test.action(vex.ID, "Grain Barge", 0, effects(map[string]int64{"food": 7}, 0), true)
	foodBefore := test.amount("food")
	asked, err := test.service.CreateRequest(test.context, test.player, core.RequestInput{ActionID: grainBarge.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.DecideRequestFromDiscord(test.context, test.player, asked.ID, false); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player's Decline = %v, want 403", err)
	}
	outcome, err := test.service.DecideRequestFromDiscord(test.context, test.dm, asked.ID, false)
	if err != nil || outcome.Status != core.RequestStatusDeclined || outcome.Text != "Declined" || outcome.RequestID != asked.ID ||
		outcome.DiscordMessageID != "message-"+asked.ID.String() {
		t.Fatalf("Decline from Discord = %+v, %v", outcome, err)
	}
	if notified := test.recorder.lastOutcome(); notified.Status != core.RequestStatusDeclined || notified.RequestID != asked.ID {
		t.Fatalf("Beny Bot's edit = %+v", notified)
	}
	request, err := test.service.GetRequest(test.context, test.player, asked.ID)
	if err != nil || request.Status != core.RequestStatusDeclined || test.amount("food") != foodBefore ||
		len(test.entries(test.dm, core.TimelineKindPatronAction)) != 0 {
		t.Fatalf("after Decline: %+v, food %d, %v", request.Status, test.amount("food"), err)
	}
	_, err = test.service.DecideRequestFromDiscord(test.context, test.dm, asked.ID, true)
	requireMessage(t, err, core.KindConflict, "That request was already decided.")
}

func TestUseActionNowTweaksAndCreditsOnlyActiveMembers(t *testing.T) {
	test := newQuestTest(t)
	test.setTo(map[string]int64{"food": 4})
	vex := test.patron("Lady Vex", true)
	test.setFavor(vex.ID, 5)
	grainBarge := test.action(vex.ID, "Grain Barge", 3, effects(map[string]int64{"food": 7}, 0), true)
	_, pendingMember := test.fixture.player(test.dm, "seed:bram", "bram", false)
	for _, onBehalfOf := range []uuid.UUID{pendingMember.MemberID(), uuid.New()} {
		_, err := test.service.UseActionNow(test.context, test.dm, core.UseActionInput{ExpectedVersion: test.board().Version, ActionID: grainBarge.ID, OnBehalfOf: &onBehalfOf})
		requireFieldMessage(t, err, "onBehalfOf", "Pick an active member of this campaign.")
	}
	if test.favor(vex.ID) != 5 || test.amount("food") != 4 {
		t.Fatalf("a refused use changed favor to %d and food to %d", test.favor(vex.ID), test.amount("food"))
	}

	tweak := effects(map[string]int64{"food": 2}, 0)
	used := test.must(test.service.UseActionNow(test.context, test.dm, core.UseActionInput{
		ExpectedVersion: test.board().Version, ActionID: grainBarge.ID, OnBehalfOf: ptr(test.player.MemberID()), AppliedEffects: &tweak,
	}))
	entry := used.Entries[0]
	if entry.Title != "Thorin called in “Grain Barge” from Lady Vex" || entry.Body != "Food +2 (favor −3)" || test.amount("food") != 6 || test.favor(vex.ID) != 2 {
		t.Fatalf("tweaked use = %+v, food %d, favor %d", entry, test.amount("food"), test.favor(vex.ID))
	}
	request, err := test.service.GetRequest(test.context, test.dm, *entry.RequestID)
	if err != nil || request.Effects.Resources["food"] != 7 || request.AppliedEffects == nil || request.AppliedEffects.Resources["food"] != 2 {
		t.Fatalf("the used request = %+v, %v", request, err)
	}
}

// A quest keeps the patron it already gives when that patron is archived, but an archived patron
// can't be picked anew, on create or on edit.
func TestAQuestKeepsAnArchivedPatronButCantPickOne(t *testing.T) {
	test := newQuestTest(t)
	vex := test.patron("Lady Vex", true)
	ridge := test.patron("Ridge Council", true)
	escort := test.quest(core.NewQuestInput{Revealed: true, QuestInput: core.QuestInput{Title: "Escort", PatronID: &vex.ID, RewardFavor: 2}})
	for _, patron := range []core.Patron{vex, ridge} {
		if _, err := test.service.SetPatronArchived(test.context, test.dm, patron.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	kept, err := test.service.UpdateQuest(test.context, test.dm, escort.ID, core.QuestInput{Title: "Escort the Barge", PatronID: &vex.ID, RewardFavor: 3})
	if err != nil || kept.Patron == nil || kept.Patron.ID != vex.ID || kept.RewardFavor != 3 {
		t.Fatalf("editing a quest of an archived patron = %+v, %v", kept, err)
	}
	_, err = test.service.UpdateQuest(test.context, test.dm, escort.ID, core.QuestInput{Title: "Escort", PatronID: &ridge.ID})
	requireFieldMessage(t, err, "patronId", "Ridge Council is archived.")
	_, err = test.service.CreateQuest(test.context, test.dm, core.NewQuestInput{QuestInput: core.QuestInput{Title: "Another", PatronID: &vex.ID}})
	requireFieldMessage(t, err, "patronId", "Lady Vex is archived.")
}
