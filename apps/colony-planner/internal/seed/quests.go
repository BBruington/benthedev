package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// The M3 sample patrons, quests and requests (spec §9), played through core so favor, entries and the
// requests' snapshots are exactly what real play produces. Sample Colony's history nets to zero on the
// stockpile (the failed quest's food −7 and the granted Grain Barge's food +7), so every M1 and M2
// number stays as it was; Lady Vex ends on favor 5, Example G's starting point.

// enterAs is how the seed acts as one of a campaign's sample players.
type enterAs func(discordUserID string) (core.Viewer, error)

func seedQuestsAndPatrons(requestContext context.Context, service *core.Service, dm core.Viewer, enter enterAs) error {
	vex, err := createPatron(requestContext, service, dm, core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{
		Name:        "Lady Vex",
		Description: "A mainland shipping magnate who funds the colony's riskier ventures.",
		PlayerNotes: "Pays in grain and favors. Remembers both.",
		DmNotes:     "Wants the north cove for her own ships.",
	}})
	if err != nil {
		return err
	}
	hollowCourt, err := createPatron(requestContext, service, dm, core.NewPatronInput{PatronInput: core.PatronInput{
		Name:        "The Hollow Court",
		Description: "Whispers from the Drowned Chapel.",
		DmNotes:     "The cult's patrons. The party hasn't met them yet.",
		SortOrder:   1,
	}})
	if err != nil {
		return err
	}
	orrin, err := createPatron(requestContext, service, dm, core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{
		Name:        "Captain Orrin",
		Description: "A charter captain who sailed home for good.",
		SortOrder:   2,
	}})
	if err != nil {
		return err
	}

	grainBarge, err := createAction(requestContext, service, dm, vex, "Grain Barge", "A barge of mainland grain, rowed in at dawn.", 3, map[string]int64{"food": 7}, true)
	if err != nil {
		return err
	}
	if _, err := createAction(requestContext, service, dm, vex, "Hire Mercenaries", "Two of Vex's guards, on loan.", 6, map[string]int64{"gold": -20}, false); err != nil {
		return err
	}
	spiceCaravan, err := createAction(requestContext, service, dm, vex, "Spice Caravan", "Sold out for the season.", 2, map[string]int64{"morale": 5}, true)
	if err != nil {
		return err
	}
	if _, err := service.SetPatronActionArchived(requestContext, dm, spiceCaravan, true); err != nil {
		return fmt.Errorf("archive the Spice Caravan: %w", err)
	}
	if _, err := createAction(requestContext, service, dm, hollowCourt, "Whisper Network", "Names, for a price.", 2, nil, false); err != nil {
		return err
	}
	if _, err := createAction(requestContext, service, dm, orrin, "Charted Passage", "Orrin's old charts of the reef.", 1, map[string]int64{"materials": 10}, true); err != nil {
		return err
	}
	if _, err := service.SetPatronArchived(requestContext, dm, orrin, true); err != nil {
		return fmt.Errorf("archive Captain Orrin: %w", err)
	}

	escort, err := createQuest(requestContext, service, dm, core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{
		Title:       "Escort the Surveyors",
		Description: "Lady Vex's surveyors want the coast mapped as far as the reef.",
		Kind:        core.QuestKindColony,
		PatronID:    &vex,
		RewardFavor: 4,
		Loot:        []core.LootItem{{Name: "Surveyor's chart", Quantity: 1, Description: "The coast as far as the reef."}},
	}})
	if err != nil {
		return err
	}
	if _, err := service.CompleteQuest(requestContext, dm, escort); err != nil {
		return fmt.Errorf("complete the escort: %w", err)
	}
	ledger, err := createQuest(requestContext, service, dm, core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{
		Title:          "Recover the Lost Ledger",
		Description:    "Vex's ledger went down with a skiff off the north cove.",
		Kind:           core.QuestKindMainland,
		PatronID:       &vex,
		FailureEffects: core.Effects{Resources: map[string]int64{"food": -7}},
		FailureFavor:   1,
		DmNotes:        "The smugglers have it.",
	}})
	if err != nil {
		return err
	}
	if _, err := service.FailQuest(requestContext, dm, ledger); err != nil {
		return fmt.Errorf("fail the ledger: %w", err)
	}
	favorDelta := int64(5)
	board, err := service.GetColonyBoard(requestContext, dm)
	if err != nil {
		return err
	}
	if _, err := service.AdjustFavor(requestContext, dm, vex, core.FavorAdjustInput{
		ExpectedVersion: board.Version,
		Delta:           &favorDelta,
		Reason:          "Thanks for the warning about the smugglers",
	}); err != nil {
		return fmt.Errorf("adjust Lady Vex's favor: %w", err)
	}

	// Requests: Thorin's is granted, Sela's declined with a note only she (and the DM) reads, Pip's
	// withdrawn, and Maren's waits for the DM.
	thorin, err := requestAs(requestContext, service, enter, "seed:ana", grainBarge, "The stores are thin after the ledger business.")
	if err != nil {
		return err
	}
	sela, err := requestAs(requestContext, service, enter, "seed:bram", grainBarge, "")
	if err != nil {
		return err
	}
	pip, err := requestAs(requestContext, service, enter, "seed:cass", grainBarge, "Never mind, the nets came in full.")
	if err != nil {
		return err
	}
	if _, err := service.ApproveRequest(requestContext, dm, thorin, core.ApproveRequestInput{}); err != nil {
		return fmt.Errorf("grant Thorin's request: %w", err)
	}
	if _, err := service.DeclineRequest(requestContext, dm, sela, core.DeclineRequestInput{Note: "Vex is busy this week."}); err != nil {
		return fmt.Errorf("decline Sela's request: %w", err)
	}
	cass, err := enter("seed:cass")
	if err != nil {
		return err
	}
	if _, err := service.WithdrawRequest(requestContext, cass, pip); err != nil {
		return fmt.Errorf("withdraw Pip's request: %w", err)
	}
	if _, err := requestAs(requestContext, service, enter, "seed:dov", grainBarge, "For the harvest feast?"); err != nil {
		return err
	}

	if _, err := createQuest(requestContext, service, dm, core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{
		Title:          "Clear the Wolf Den",
		Description:    "Wolves from the hollow north of the fields have taken two goats this week.",
		Kind:           core.QuestKindColony,
		PatronID:       &vex,
		RewardEffects:  core.Effects{Resources: map[string]int64{"food": 20, "morale": 5}, Colonists: 3},
		RewardFavor:    2,
		FailureEffects: core.Effects{Resources: map[string]int64{"morale": -10}},
		FailureFavor:   2,
		Loot:           []core.LootItem{{Name: "Wolf pelts", Quantity: 6}},
		DmNotes:        "The pack leader is larger than any wolf should be.",
	}}); err != nil {
		return err
	}
	if _, err := createQuest(requestContext, service, dm, core.NewQuestInput{PostToTimeline: true, QuestInput: core.QuestInput{
		Title:       "Find the Smugglers' Cove",
		Description: "Someone is landing cargo north of the colony at night.",
		Kind:        core.QuestKindColony,
		DmNotes:     "Reveal once the party talks to the smuggler again.",
	}}); err != nil {
		return err
	}
	return nil
}

// seedSecondQuestsAndPatrons is Second Sample's smaller set: the Ridge Council, one action, a quest
// completed for favor only (so Ridgehold's numbers stay as they were), one active quest and a pending
// request from Tamsin Vale.
func seedSecondQuestsAndPatrons(requestContext context.Context, service *core.Service, dm core.Viewer, enter enterAs) error {
	council, err := createPatron(requestContext, service, dm, core.NewPatronInput{Revealed: true, PatronInput: core.PatronInput{
		Name:        "The Ridge Council",
		Description: "The mining families who still hold the old claims.",
	}})
	if err != nil {
		return err
	}
	timberLevy, err := createAction(requestContext, service, dm, council, "Timber Levy", "Logs hauled up from the valley.", 2, map[string]int64{"timber": 5}, true)
	if err != nil {
		return err
	}
	highPass, err := createQuest(requestContext, service, dm, core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{
		Title:       "Map the High Pass",
		PatronID:    &council,
		RewardFavor: 3,
	}})
	if err != nil {
		return err
	}
	if _, err := service.CompleteQuest(requestContext, dm, highPass); err != nil {
		return fmt.Errorf("complete the high pass: %w", err)
	}
	if _, err := createQuest(requestContext, service, dm, core.NewQuestInput{Revealed: true, PostToTimeline: true, QuestInput: core.QuestInput{
		Title:         "Hold the Ridge",
		Kind:          core.QuestKindColony,
		RewardEffects: core.Effects{Resources: map[string]int64{"timber": 10}},
	}}); err != nil {
		return err
	}
	_, err = requestAs(requestContext, service, enter, "seed:iko", timberLevy, "For the palisade.")
	return err
}

func createPatron(requestContext context.Context, service *core.Service, dm core.Viewer, input core.NewPatronInput) (uuid.UUID, error) {
	page, err := service.CreatePatron(requestContext, dm, input)
	if err != nil {
		return uuid.Nil, fmt.Errorf("patron %s: %w", input.Name, err)
	}
	return page.Patron.ID, nil
}

func createAction(requestContext context.Context, service *core.Service, dm core.Viewer, patronID uuid.UUID, name string, description string,
	favorCost int64, resources map[string]int64, revealed bool,
) (uuid.UUID, error) {
	action, err := service.CreatePatronAction(requestContext, dm, core.NewPatronActionInput{
		PatronID: patronID,
		Revealed: revealed,
		PatronActionInput: core.PatronActionInput{
			Name:        name,
			Description: description,
			FavorCost:   favorCost,
			Effects:     core.Effects{Resources: resources},
		},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("action %s: %w", name, err)
	}
	return action.ID, nil
}

func createQuest(requestContext context.Context, service *core.Service, dm core.Viewer, input core.NewQuestInput) (uuid.UUID, error) {
	quest, err := service.CreateQuest(requestContext, dm, input)
	if err != nil {
		return uuid.Nil, fmt.Errorf("quest %s: %w", input.Title, err)
	}
	return quest.ID, nil
}

func requestAs(requestContext context.Context, service *core.Service, enter enterAs, discordUserID string, actionID uuid.UUID, note string) (uuid.UUID, error) {
	player, err := enter(discordUserID)
	if err != nil {
		return uuid.Nil, err
	}
	request, err := service.CreateRequest(requestContext, player, core.RequestInput{ActionID: actionID, Note: note})
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s's request: %w", discordUserID, err)
	}
	return request.ID, nil
}
