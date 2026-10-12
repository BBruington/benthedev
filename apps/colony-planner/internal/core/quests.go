package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// Quest is one quest as a projection sees it (spec §4.3, §5.6, §5.9). Players get only revealed
// quests, with the giver NPC and patron only while those are revealed. The failure cost is DM-only
// until it happens (Open Question 14), and even then players read it only on the QUEST_FAILED entry.
type Quest struct {
	ID          uuid.UUID   `json:"id" format:"uuid"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Kind        QuestKind   `json:"kind"`
	Status      QuestStatus `json:"status"`
	// GiverNpc and Patron are null when unset or hidden from this viewer.
	GiverNpc      *QuestGiverNpc   `json:"giverNpc" doc:"null when unset or hidden from this viewer"`
	Patron        *PatronReference `json:"patron" doc:"The patron giver; null when unset or hidden from this viewer"`
	RewardEffects Effects          `json:"rewardEffects"`
	// RewardFavor goes to the patron giver, so it is 0 for a viewer the patron is hidden from.
	RewardFavor int64      `json:"rewardFavor" doc:"Favor the patron giver gains on completion; 0 when the patron is hidden from this viewer"`
	Loot        []LootItem `json:"loot" doc:"Display only"`
	ResolvedDay *int64     `json:"resolvedDay" doc:"The colony day it was completed or failed; null while active"`
	ResolvedAt  *time.Time `json:"resolvedAt" doc:"null while active"`
	CreatedAt   time.Time  `json:"createdAt"`
	Dm          *QuestDm   `json:"dm,omitempty"`
}

// QuestDm is what only the DM sees of a quest.
type QuestDm struct {
	Revealed       bool    `json:"revealed"`
	DmNotes        string  `json:"dmNotes"`
	FailureEffects Effects `json:"failureEffects" doc:"Applied in CLAMP mode when the quest fails"`
	FailureFavor   int64   `json:"failureFavor" doc:"Favor the patron giver loses when the quest fails; it may take favor below 0"`
}

type QuestGiverNpc struct {
	_           struct{}  `nullable:"true"`
	ID          uuid.UUID `json:"id" format:"uuid"`
	DisplayName string    `json:"displayName"`
}

// PatronReference names a patron beside another record.
type PatronReference struct {
	_    struct{}  `nullable:"true"`
	ID   uuid.UUID `json:"id" format:"uuid"`
	Name string    `json:"name"`
}

// LootItem is one line of a quest's loot, which is display only (spec §5.6).
type LootItem struct {
	Name        string `json:"name" doc:"1 to 80 characters"`
	Quantity    int64  `json:"quantity" doc:"1 to 1,000,000"`
	Description string `json:"description" doc:"At most 500 characters"`
}

type QuestList struct {
	Quests []Quest `json:"quests" doc:"Newest first"`
}

// QuestInput replaces a quest's record (PUT semantics). Status isn't here: it changes only through
// Complete, Fail and Reopen. Nor is visibility: only SetQuestRevealed changes it, so an edit form
// opened before the DM hid the quest can't reveal it again.
type QuestInput struct {
	Title          string     `json:"title" doc:"1 to 120 characters"`
	Description    string     `json:"description" doc:"At most 4000 characters"`
	Kind           QuestKind  `json:"kind" doc:"A label with no logic; blank means COLONY"`
	GiverNpcID     *uuid.UUID `json:"giverNpcId" format:"uuid" nullable:"true" doc:"null for none"`
	PatronID       *uuid.UUID `json:"patronId" format:"uuid" nullable:"true" doc:"The patron giver; null for none. Favor needs one."`
	RewardEffects  Effects    `json:"rewardEffects" doc:"Applied in CLAMP mode on completion"`
	RewardFavor    int64      `json:"rewardFavor" doc:"0 or more; paid to the patron giver on completion"`
	FailureEffects Effects    `json:"failureEffects" doc:"The optional failure cost, applied in CLAMP mode when the quest fails"`
	FailureFavor   int64      `json:"failureFavor" doc:"0 or more; taken from the patron giver when the quest fails, even below 0"`
	Loot           []LootItem `json:"loot" doc:"At most 50 items"`
	DmNotes        string     `json:"dmNotes" doc:"At most 4000 characters"`
}

// NewQuestInput adds a quest: its record, whether players start out seeing it, and whether becoming
// visible posts it to the timeline (spec §5.6).
type NewQuestInput struct {
	QuestInput
	Revealed       bool `json:"revealed" doc:"Visible to players; the create form starts it on"`
	PostToTimeline bool `json:"postToTimeline" doc:"Write a PUBLIC QUEST_POSTED entry when the quest is visible; the form starts it on"`
}

// QuestRevealInput is the DM's Visible to players switch.
type QuestRevealInput struct {
	Revealed       bool `json:"revealed"`
	PostToTimeline bool `json:"postToTimeline" doc:"When the quest becomes visible, write a PUBLIC QUEST_POSTED entry"`
}

// QuestResolutionPreview shows Complete or Fail before Confirm: "Food 40 → 60 · +3 to Unassigned ·
// Lady Vex favor 1 → 3" (spec §5.6).
type QuestResolutionPreview struct {
	Resources       []ResourceChangePreview `json:"resources" doc:"Every resource the reward or cost names, sorted by key"`
	ColonistsBefore int64                   `json:"colonistsBefore"`
	ColonistsAfter  int64                   `json:"colonistsAfter"`
	Favor           *FavorChangePreview     `json:"favor" doc:"null when the quest has no patron giver"`
	Summary         string                  `json:"summary" doc:"e.g. Food 0 → 20 · Morale 98 → 100 (capped) · 3 colonists (Unassigned) · Lady Vex favor 1 → 3"`
}

const (
	questTitleMaxLength       = 120
	questProseMaxLength       = 4000
	lootMaxItems              = 50
	lootNameMaxLength         = 80
	lootDescriptionMaxLength  = 500
	lootQuantityMax           = 1_000_000
	messageQuestAlreadyClosed = "Already resolved"
)

var errQuestNotFound = NotFound("That quest doesn't exist.")

// ListQuests is /quests as the viewer's projection sees it.
func (service *Service) ListQuests(requestContext context.Context, viewer Viewer) (QuestList, error) {
	if err := requireActive(viewer); err != nil {
		return QuestList{}, err
	}
	dmProjection := viewer.SeesDmFields()
	rows, err := service.queries.ListQuests(requestContext, store.ListQuestsParams{CampaignID: viewer.campaignID, DmProjection: dmProjection})
	if err != nil {
		return QuestList{}, err
	}
	quests := make([]Quest, 0, len(rows))
	for _, row := range rows {
		quest, err := questFromView(store.GetQuestViewRow(row), dmProjection)
		if err != nil {
			return QuestList{}, err
		}
		quests = append(quests, quest)
	}
	return QuestList{Quests: quests}, nil
}

// GetQuest is one quest's page. A hidden quest is not found for players, exactly like a missing one.
func (service *Service) GetQuest(requestContext context.Context, viewer Viewer, questID uuid.UUID) (Quest, error) {
	if err := requireActive(viewer); err != nil {
		return Quest{}, err
	}
	return readQuest(requestContext, service.queries, viewer.campaignID, questID, viewer.SeesDmFields())
}

// CreateQuest adds a quest; one created visible is posted to the timeline unless the DM unticks it.
// It is not a colony mutation.
func (service *Service) CreateQuest(requestContext context.Context, viewer Viewer, input NewQuestInput) (Quest, error) {
	if err := requireDm(viewer); err != nil {
		return Quest{}, err
	}
	var quest Quest
	posted := input.Revealed && input.PostToTimeline
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so concurrent creates can't each pass the count and go over the limit.
		colony, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID)
		if err != nil {
			return campaignGone(err)
		}
		count, err := queries.CountQuests(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if count >= QuestLimit {
			return Conflict("This campaign has " + formatNumber(QuestLimit) + " quests, the most it can hold. Delete one first.")
		}
		cleaned, err := cleanQuestInput(requestContext, queries, viewer.campaignID, input.QuestInput, nil)
		if err != nil {
			return err
		}
		row, err := queries.CreateQuest(requestContext, store.CreateQuestParams{
			CampaignID:     viewer.campaignID,
			Title:          cleaned.Title,
			Description:    cleaned.Description,
			Kind:           cleaned.Kind,
			Revealed:       input.Revealed,
			GiverNpcID:     cleaned.GiverNpcID,
			PatronID:       cleaned.PatronID,
			RewardEffects:  cleaned.RewardEffects,
			RewardFavor:    cleaned.RewardFavor,
			FailureEffects: cleaned.FailureEffects,
			FailureFavor:   cleaned.FailureFavor,
			Loot:           cleaned.Loot,
			DmNotes:        cleaned.DmNotes,
		})
		if err != nil {
			return err
		}
		if posted {
			if err := writeQuestPosted(requestContext, queries, viewer, row, int64(colony.Day)); err != nil {
				return err
			}
		}
		quest, err = readQuest(requestContext, queries, viewer.campaignID, row.ID, true)
		return err
	})
	if err != nil {
		return Quest{}, err
	}
	topics := []Topic{TopicQuests}
	if posted {
		topics = append(topics, TopicTimeline)
	}
	service.broker.Publish(viewer.campaignID, topics...)
	return quest, nil
}

// UpdateQuest replaces a quest's record, leaving its status and visibility as they are.
func (service *Service) UpdateQuest(requestContext context.Context, viewer Viewer, questID uuid.UUID, input QuestInput) (Quest, error) {
	if err := requireDm(viewer); err != nil {
		return Quest{}, err
	}
	var quest Quest
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		existing, err := queries.GetQuest(requestContext, store.GetQuestParams{CampaignID: viewer.campaignID, ID: questID})
		if err != nil {
			return questGone(err)
		}
		cleaned, err := cleanQuestInput(requestContext, queries, viewer.campaignID, input, existing.PatronID)
		if err != nil {
			return err
		}
		cleaned.ID = questID
		if _, err := queries.UpdateQuest(requestContext, cleaned); err != nil {
			return questGone(err)
		}
		quest, err = readQuest(requestContext, queries, viewer.campaignID, questID, true)
		return err
	})
	if err != nil {
		return Quest{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicQuests)
	return quest, nil
}

// SetQuestRevealed is the DM's Visible to players switch. A quest that becomes visible is posted to
// the timeline unless the DM unticks it (spec §5.6); hiding it again hides its entries from players.
func (service *Service) SetQuestRevealed(requestContext context.Context, viewer Viewer, questID uuid.UUID, input QuestRevealInput) (Quest, error) {
	if err := requireDm(viewer); err != nil {
		return Quest{}, err
	}
	var quest Quest
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID)
		if err != nil {
			return campaignGone(err)
		}
		existing, err := queries.GetQuest(requestContext, store.GetQuestParams{CampaignID: viewer.campaignID, ID: questID})
		if err != nil {
			return questGone(err)
		}
		row, err := queries.SetQuestRevealed(requestContext, store.SetQuestRevealedParams{CampaignID: viewer.campaignID, ID: questID, Revealed: input.Revealed})
		if err != nil {
			return questGone(err)
		}
		if !existing.Revealed && input.Revealed && input.PostToTimeline {
			if err := writeQuestPosted(requestContext, queries, viewer, row, int64(colony.Day)); err != nil {
				return err
			}
		}
		quest, err = readQuest(requestContext, queries, viewer.campaignID, questID, true)
		return err
	})
	if err != nil {
		return Quest{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicQuests, TopicTimeline)
	return quest, nil
}

// DeleteQuest is a hard delete (spec §5.6): its entries keep their text and lose the link. A hidden
// quest's entries move to DM first, since without the link players would see them.
func (service *Service) DeleteQuest(requestContext context.Context, viewer Viewer, questID uuid.UUID) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, as undo and the Visible switch lock it, so the delete neither deadlocks with an
		// undo of this quest's change nor reads a revealed flag that is about to change.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		existing, err := queries.GetQuest(requestContext, store.GetQuestParams{CampaignID: viewer.campaignID, ID: questID})
		if err != nil {
			return questGone(err)
		}
		if !existing.Revealed {
			if err := queries.HideQuestTimelineEvents(requestContext, store.HideQuestTimelineEventsParams{CampaignID: viewer.campaignID, QuestID: questID}); err != nil {
				return err
			}
		}
		_, err = queries.DeleteQuest(requestContext, store.DeleteQuestParams{CampaignID: viewer.campaignID, ID: questID})
		return err
	})
	if err != nil {
		return err
	}
	service.broker.Publish(viewer.campaignID, TopicQuests, TopicTimeline)
	return nil
}

// questResolution is Complete or Fail: which effects and favor apply, and how the entry reads.
type questResolution struct {
	status      QuestStatus
	kind        TimelineKind
	titlePrefix string
}

var (
	questCompletion = questResolution{status: QuestStatusCompleted, kind: TimelineKindQuestCompleted, titlePrefix: "Quest completed: "}
	questFailure    = questResolution{status: QuestStatusFailed, kind: TimelineKindQuestFailed, titlePrefix: "Quest failed: "}
)

// cost is the effects and favor delta this resolution applies to the quest.
func (resolution questResolution) cost(quest store.Quest) (Effects, int64, error) {
	if resolution.status == QuestStatusCompleted {
		effects, err := decodeEffects(quest.RewardEffects)
		return effects, int64(quest.RewardFavor), err
	}
	effects, err := decodeEffects(quest.FailureEffects)
	return effects, -int64(quest.FailureFavor), err
}

// PreviewCompleteQuest shows what Complete would pay, caps included, without writing.
func (service *Service) PreviewCompleteQuest(requestContext context.Context, viewer Viewer, questID uuid.UUID) (QuestResolutionPreview, error) {
	return service.previewQuestResolution(requestContext, viewer, questID, questCompletion)
}

// PreviewFailQuest shows what Fail would cost, floors included, without writing.
func (service *Service) PreviewFailQuest(requestContext context.Context, viewer Viewer, questID uuid.UUID) (QuestResolutionPreview, error) {
	return service.previewQuestResolution(requestContext, viewer, questID, questFailure)
}

func (service *Service) previewQuestResolution(requestContext context.Context, viewer Viewer, questID uuid.UUID, resolution questResolution) (QuestResolutionPreview, error) {
	if err := requireDm(viewer); err != nil {
		return QuestResolutionPreview{}, err
	}
	colony, err := readColony(requestContext, service.queries, viewer.campaignID, false)
	if err != nil {
		return QuestResolutionPreview{}, err
	}
	quest, err := service.queries.GetQuest(requestContext, store.GetQuestParams{CampaignID: viewer.campaignID, ID: questID})
	if err != nil {
		return QuestResolutionPreview{}, questGone(err)
	}
	if quest.Status != QuestStatusActive {
		return QuestResolutionPreview{}, Conflict(messageQuestAlreadyClosed)
	}
	var patron *store.Patron
	if quest.PatronID != nil {
		row, err := service.queries.GetPatron(requestContext, store.GetPatronParams{CampaignID: viewer.campaignID, ID: *quest.PatronID})
		if err != nil {
			return QuestResolutionPreview{}, err
		}
		patron = &row
	}
	result, err := resolveQuest(colony, quest, patron, resolution)
	if err != nil {
		return QuestResolutionPreview{}, err
	}
	resources, parts := effectsPreview(colony, result.Effects)
	preview := QuestResolutionPreview{
		Resources:       resources,
		ColonistsBefore: game.Colonists(result.Effects.Before.Roles),
		ColonistsAfter:  game.Colonists(result.Effects.After.Roles),
	}
	if patron != nil {
		preview.Favor = &FavorChangePreview{PatronID: patron.ID, PatronName: patron.Name, Before: result.PatronBefore.Favor, After: result.PatronAfter.Favor}
		if result.PatronAfter.Favor != result.PatronBefore.Favor {
			parts = append(parts, favorChangeText(patron.Name, result.PatronBefore.Favor, result.PatronAfter.Favor))
		}
	}
	preview.Summary = joinParts(parts...)
	if preview.Summary == "" {
		preview.Summary = "Nothing would change"
	}
	return preview, nil
}

// CompleteQuest pays a quest's reward (spec §5.6): the reward in CLAMP mode and reward_favor to its
// patron giver, behind the status guard, so a second click (or a second tab) gets 409 "Already
// resolved" and pays nothing. It is a colony mutation, so Undo reverses it.
func (service *Service) CompleteQuest(requestContext context.Context, viewer Viewer, questID uuid.UUID) (ColonyChange, error) {
	return service.resolveQuestMutation(requestContext, viewer, questID, questCompletion)
}

// FailQuest applies a quest's optional failure cost (spec §5.6): failure_effects in CLAMP mode and
// failure_favor taken from its patron giver in full, below 0 too. Same guard and undo as Complete.
func (service *Service) FailQuest(requestContext context.Context, viewer Viewer, questID uuid.UUID) (ColonyChange, error) {
	return service.resolveQuestMutation(requestContext, viewer, questID, questFailure)
}

func (service *Service) resolveQuestMutation(requestContext context.Context, viewer Viewer, questID uuid.UUID, resolution questResolution) (ColonyChange, error) {
	return service.mutateColonyGuardedByStatus(requestContext, viewer, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		if _, err := queries.GetQuest(requestContext, store.GetQuestParams{CampaignID: viewer.campaignID, ID: questID}); err != nil {
			return colonyOutcome{}, questGone(err)
		}
		resolvedDay := int32(colony.state.Day)
		quest, err := queries.ResolveQuest(requestContext, store.ResolveQuestParams{
			CampaignID:  viewer.campaignID,
			ID:          questID,
			Status:      resolution.status,
			ResolvedDay: &resolvedDay,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return colonyOutcome{}, Conflict(messageQuestAlreadyClosed)
		}
		if err != nil {
			return colonyOutcome{}, err
		}
		var patron *store.Patron
		if quest.PatronID != nil {
			row, err := queries.GetPatronForUpdate(requestContext, store.GetPatronForUpdateParams{CampaignID: viewer.campaignID, ID: *quest.PatronID})
			if err != nil {
				return colonyOutcome{}, err
			}
			patron = &row
		}
		result, err := resolveQuest(colony, quest, patron, resolution)
		if err != nil {
			return colonyOutcome{}, err
		}
		entry := plannedEntry{
			day:        colony.state.Day,
			kind:       resolution.kind,
			title:      resolution.titlePrefix + quest.Title,
			body:       appliedEffectsText(result.Effects.Applied, colony.order, colony.labels),
			visibility: questEntryVisibility(quest.Revealed),
			deltas:     result.Effects.Applied,
			questID:    &quest.ID,
		}
		outcome := colonyOutcome{
			after:  result.Effects.After,
			label:  entry.title,
			quests: map[uuid.UUID]questBefore{quest.ID: {Status: QuestStatusActive}},
			topics: []Topic{TopicQuests},
		}
		if patron != nil && result.PatronAfter.Favor != result.PatronBefore.Favor {
			if err := queries.SetPatronFavor(requestContext, store.SetPatronFavorParams{CampaignID: viewer.campaignID, ID: patron.ID, Favor: int32(result.PatronAfter.Favor)}); err != nil {
				return colonyOutcome{}, err
			}
			outcome.patronFavor = map[uuid.UUID]favorChange{patron.ID: {before: result.PatronBefore.Favor, after: result.PatronAfter.Favor}}
			outcome.topics = append(outcome.topics, TopicPatrons)
			line := favorLine(patron.Name, result.PatronAfter.Favor-result.PatronBefore.Favor)
			// The favor line names the patron, so it goes in the text only while players may know them.
			if patron.Revealed {
				entry.body = joinParts(entry.body, line)
				entry.patronID = &patron.ID
			} else {
				entry.dmNote = line
			}
		}
		outcome.entries = []plannedEntry{entry}
		return outcome, nil
	})
}

// ReopenQuest moves a failed quest back to ACTIVE (spec §5.6). It is status only: nothing is given
// back. Completed quests don't reopen; Undo or an adjustment fixes a mistaken Complete.
func (service *Service) ReopenQuest(requestContext context.Context, viewer Viewer, questID uuid.UUID) (ColonyChange, error) {
	return service.mutateColonyGuardedByStatus(requestContext, viewer, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		existing, err := queries.GetQuest(requestContext, store.GetQuestParams{CampaignID: viewer.campaignID, ID: questID})
		if err != nil {
			return colonyOutcome{}, questGone(err)
		}
		switch existing.Status {
		case QuestStatusCompleted:
			return colonyOutcome{}, Conflict("A completed quest doesn't reopen. Undo the completion while it is the latest change, or adjust the stock.")
		case QuestStatusActive:
			return colonyOutcome{}, Conflict("That quest is already active.")
		}
		quest, err := queries.ReopenFailedQuest(requestContext, store.ReopenFailedQuestParams{CampaignID: viewer.campaignID, ID: questID})
		if errors.Is(err, pgx.ErrNoRows) {
			return colonyOutcome{}, Conflict("That quest is already active.")
		}
		if err != nil {
			return colonyOutcome{}, err
		}
		entry := plannedEntry{
			day:        colony.state.Day,
			kind:       TimelineKindQuestReopened,
			title:      "Quest reopened: " + quest.Title,
			visibility: questEntryVisibility(quest.Revealed),
			questID:    &quest.ID,
		}
		return colonyOutcome{
			after:   colony.state,
			label:   entry.title,
			entries: []plannedEntry{entry},
			quests:  map[uuid.UUID]questBefore{quest.ID: {Status: existing.Status, ResolvedDay: existing.ResolvedDay, ResolvedAt: existing.ResolvedAt}},
			topics:  []Topic{TopicQuests},
		}, nil
	})
}

// resolveQuest runs the rule for a locked or previewed quest.
func resolveQuest(colony colonySnapshot, quest store.Quest, patron *store.Patron, resolution questResolution) (game.QuestResult, error) {
	effects, favorDelta, err := resolution.cost(quest)
	if err != nil {
		return game.QuestResult{}, err
	}
	var gamePatron *game.Patron
	if patron != nil {
		gamePatron = &game.Patron{Name: patron.Name, Favor: int64(patron.Favor)}
	}
	result, err := game.ResolveQuest(colony.state, gamePatron, effects, favorDelta)
	if err != nil {
		return game.QuestResult{}, gameError(err, nil)
	}
	return result, nil
}

func questEntryVisibility(revealed bool) TimelineVisibility {
	if revealed {
		return TimelineVisibilityPublic
	}
	return TimelineVisibilityDm
}

// writeQuestPosted is QUEST_POSTED: "New quest: Clear the Wolf Den", PUBLIC (spec §5.8).
func writeQuestPosted(requestContext context.Context, queries *store.Queries, viewer Viewer, quest store.Quest, day int64) error {
	return writeLooseEntry(requestContext, queries, viewer, plannedEntry{
		day:        day,
		kind:       TimelineKindQuestPosted,
		title:      "New quest: " + quest.Title,
		visibility: TimelineVisibilityPublic,
		questID:    &quest.ID,
	})
}

// cleanQuestInput checks a quest form and loads every ID in it with the viewer's campaign. A patron
// archived since may stay on the quest it already gives (currentPatronID), but can't be picked anew.
func cleanQuestInput(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, input QuestInput, currentPatronID *uuid.UUID) (store.UpdateQuestParams, error) {
	var fieldErrors []FieldError
	collect := func(fieldError *FieldError) {
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
	}
	parameters := store.UpdateQuestParams{CampaignID: campaignID, Kind: input.Kind}
	var fieldError *FieldError
	parameters.Title, fieldError = cleanText("title", input.Title, questTitleMaxLength)
	collect(fieldError)
	if fieldError == nil && parameters.Title == "" {
		collect(&FieldError{Field: "title", Message: "Give the quest a title."})
	}
	parameters.Description, fieldError = cleanMultilineText("description", input.Description, questProseMaxLength)
	collect(fieldError)
	parameters.DmNotes, fieldError = cleanMultilineText("dmNotes", input.DmNotes, questProseMaxLength)
	collect(fieldError)
	if parameters.Kind == "" {
		parameters.Kind = QuestKindColony
	} else if !parameters.Kind.Valid() {
		collect(&FieldError{Field: "kind", Message: "Pick Mainland or Colony."})
	}
	if input.GiverNpcID != nil {
		if _, err := queries.GetNpc(requestContext, store.GetNpcParams{CampaignID: campaignID, ID: *input.GiverNpcID}); errors.Is(err, pgx.ErrNoRows) {
			collect(&FieldError{Field: "giverNpcId", Message: "That NPC doesn't exist."})
		} else if err != nil {
			return store.UpdateQuestParams{}, err
		}
		parameters.GiverNpcID = input.GiverNpcID
	}
	if input.PatronID != nil {
		patron, err := queries.GetPatron(requestContext, store.GetPatronParams{CampaignID: campaignID, ID: *input.PatronID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			collect(&FieldError{Field: "patronId", Message: "That patron doesn't exist."})
		case err != nil:
			return store.UpdateQuestParams{}, err
		case patron.Archived && (currentPatronID == nil || *currentPatronID != patron.ID):
			collect(&FieldError{Field: "patronId", Message: patron.Name + " is archived."})
		}
		parameters.PatronID = input.PatronID
	}
	for _, favor := range []struct {
		field string
		value int64
	}{{"rewardFavor", input.RewardFavor}, {"failureFavor", input.FailureFavor}} {
		switch {
		case favor.value < 0 || favor.value > game.MaxStored:
			collect(&FieldError{Field: favor.field, Message: "Use a whole number from 0 up."})
		case favor.value > 0 && input.PatronID == nil:
			collect(&FieldError{Field: favor.field, Message: "Favor needs a patron giver."})
		}
	}
	parameters.RewardFavor = int32(min(max(input.RewardFavor, 0), game.MaxStored))
	parameters.FailureFavor = int32(min(max(input.FailureFavor, 0), game.MaxStored))
	resources, err := campaignResources(requestContext, queries, campaignID)
	if err != nil {
		return store.UpdateQuestParams{}, err
	}
	reward, rewardErrors := cleanEffects("rewardEffects", input.RewardEffects, resources)
	fieldErrors = append(fieldErrors, rewardErrors...)
	failure, failureErrors := cleanEffects("failureEffects", input.FailureEffects, resources)
	fieldErrors = append(fieldErrors, failureErrors...)
	loot, lootErrors := cleanLoot(input.Loot)
	fieldErrors = append(fieldErrors, lootErrors...)
	if len(fieldErrors) > 0 {
		return store.UpdateQuestParams{}, Invalid("Check the quest's details.", fieldErrors...)
	}
	if parameters.RewardEffects, err = encodeEffects(reward); err != nil {
		return store.UpdateQuestParams{}, err
	}
	if parameters.FailureEffects, err = encodeEffects(failure); err != nil {
		return store.UpdateQuestParams{}, err
	}
	if parameters.Loot, err = json.Marshal(loot); err != nil {
		return store.UpdateQuestParams{}, fmt.Errorf("core: encode loot: %w", err)
	}
	return parameters, nil
}

func cleanLoot(items []LootItem) ([]LootItem, []FieldError) {
	if len(items) > lootMaxItems {
		return nil, []FieldError{{Field: "loot", Message: fmt.Sprintf("List at most %d items.", lootMaxItems)}}
	}
	cleaned := make([]LootItem, 0, len(items))
	var fieldErrors []FieldError
	for _, item := range items {
		name, nameError := cleanText("loot", item.Name, lootNameMaxLength)
		description, descriptionError := cleanText("loot", item.Description, lootDescriptionMaxLength)
		switch {
		case nameError != nil:
			fieldErrors = append(fieldErrors, *nameError)
		case descriptionError != nil:
			fieldErrors = append(fieldErrors, *descriptionError)
		case name == "":
			fieldErrors = append(fieldErrors, FieldError{Field: "loot", Message: "Name every item."})
		case item.Quantity < 1 || item.Quantity > lootQuantityMax:
			fieldErrors = append(fieldErrors, FieldError{Field: "loot", Message: "Each quantity is a whole number from 1 to 1,000,000."})
		default:
			cleaned = append(cleaned, LootItem{Name: name, Quantity: item.Quantity, Description: description})
		}
	}
	return cleaned, fieldErrors
}

func readQuest(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, questID uuid.UUID, dmProjection bool) (Quest, error) {
	row, err := queries.GetQuestView(requestContext, store.GetQuestViewParams{CampaignID: campaignID, ID: questID, DmProjection: dmProjection})
	if err != nil {
		return Quest{}, questGone(err)
	}
	return questFromView(row, dmProjection)
}

func questGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errQuestNotFound
	}
	return err
}

// questFromView keeps Dm only for the DM's projection, and the Transformer strips it again for anyone
// else. The giver and patron names are NULL for rows this projection can't see, so their IDs never
// leave here either.
func questFromView(row store.GetQuestViewRow, dmProjection bool) (Quest, error) {
	quest := Quest{
		ID:          row.Quest.ID,
		Title:       row.Quest.Title,
		Description: row.Quest.Description,
		Kind:        row.Quest.Kind,
		Status:      row.Quest.Status,
		Loot:        []LootItem{},
		ResolvedDay: int64Pointer(row.Quest.ResolvedDay),
		ResolvedAt:  utcPointer(row.Quest.ResolvedAt),
		CreatedAt:   row.Quest.CreatedAt.UTC(),
		Dm: &QuestDm{
			Revealed:     row.Quest.Revealed,
			DmNotes:      row.Quest.DmNotes,
			FailureFavor: int64(row.Quest.FailureFavor),
		},
	}
	if row.Quest.GiverNpcID != nil && row.GiverNpcName != nil {
		quest.GiverNpc = &QuestGiverNpc{ID: *row.Quest.GiverNpcID, DisplayName: *row.GiverNpcName}
	}
	if row.Quest.PatronID != nil && row.PatronName != nil {
		quest.Patron = &PatronReference{ID: *row.Quest.PatronID, Name: *row.PatronName}
		quest.RewardFavor = int64(row.Quest.RewardFavor)
	}
	var err error
	if quest.RewardEffects, err = decodeEffects(row.Quest.RewardEffects); err != nil {
		return Quest{}, err
	}
	if quest.Dm.FailureEffects, err = decodeEffects(row.Quest.FailureEffects); err != nil {
		return Quest{}, err
	}
	if err := json.Unmarshal(row.Quest.Loot, &quest.Loot); err != nil {
		return Quest{}, fmt.Errorf("core: decode quest %s loot: %w", row.Quest.ID, err)
	}
	if quest.Loot == nil {
		quest.Loot = []LootItem{}
	}
	if !dmProjection {
		quest.Dm = nil
	}
	return quest, nil
}
