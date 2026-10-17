package core

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// Patron is one patron as a projection sees it (spec §4.3, §5.7). Players get only revealed patrons,
// with their exact favor (Open Question 5), and archived ones leave their list but keep their page.
type Patron struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Favor       int64     `json:"favor" doc:"May be below 0 after a DM adjustment or a failed quest"`
	PlayerNotes string    `json:"playerNotes"`
	Archived    bool      `json:"archived"`
	SortOrder   int64     `json:"sortOrder"`
	Dm          *PatronDm `json:"dm,omitempty"`
}

// PatronDm is what only the DM sees of a patron.
type PatronDm struct {
	Revealed bool   `json:"revealed"`
	DmNotes  string `json:"dmNotes"`
}

// PatronAction is one thing a patron offers for favor: "Grain Barge", 3 favor → Food +7.
type PatronAction struct {
	ID          uuid.UUID       `json:"id" format:"uuid"`
	PatronID    uuid.UUID       `json:"patronId" format:"uuid"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	FavorCost   int64           `json:"favorCost"`
	Effects     Effects         `json:"effects"`
	Archived    bool            `json:"archived"`
	SortOrder   int64           `json:"sortOrder"`
	Dm          *PatronActionDm `json:"dm,omitempty"`
}

type PatronActionDm struct {
	Revealed bool `json:"revealed"`
}

type PatronList struct {
	Patrons []Patron `json:"patrons" doc:"Unarchived first, then by position and name"`
}

// PatronPage is /patrons/$patronId: the patron, its actions and its request history.
type PatronPage struct {
	Patron   Patron         `json:"patron"`
	Actions  []PatronAction `json:"actions" doc:"For players, only revealed, unarchived actions"`
	Requests []Request      `json:"requests" doc:"This patron's requests, pending first, then newest"`
}

// PatronInput replaces a patron's record (PUT semantics). Favor, visibility and archiving each have
// their own operation, so a stale edit form can't undo them.
type PatronInput struct {
	Name        string `json:"name" doc:"1 to 80 characters"`
	Description string `json:"description" doc:"At most 4000 characters"`
	PlayerNotes string `json:"playerNotes" doc:"At most 4000 characters"`
	DmNotes     string `json:"dmNotes" doc:"At most 4000 characters"`
	SortOrder   int64  `json:"sortOrder"`
}

type NewPatronInput struct {
	PatronInput
	Revealed bool `json:"revealed" doc:"Visible to players; the create form starts it off"`
}

// PatronActionInput replaces an action's record (PUT semantics).
type PatronActionInput struct {
	Name        string  `json:"name" doc:"1 to 80 characters"`
	Description string  `json:"description" doc:"At most 4000 characters"`
	FavorCost   int64   `json:"favorCost" doc:"0 or more"`
	Effects     Effects `json:"effects"`
	SortOrder   int64   `json:"sortOrder"`
}

type NewPatronActionInput struct {
	PatronActionInput
	PatronID uuid.UUID `json:"patronId" format:"uuid"`
	Revealed bool      `json:"revealed" doc:"Visible to players; the create form starts it off"`
}

// FavorAdjustInput is the DM's favor adjustment (spec §5.4): a delta or a "set to" value, which the
// server turns into a delta under the colony lock. DM adjustments are unbounded, below 0 included.
type FavorAdjustInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Delta           *int64 `json:"delta" nullable:"true" doc:"The change; null when setTo is given"`
	SetTo           *int64 `json:"setTo" nullable:"true" doc:"The new favor; null when delta is given"`
	Reason          string `json:"reason" doc:"At most 200 characters; may be blank"`
	// Public opts in to a PUBLIC FAVOR_ADJUSTED entry; it takes effect only while the patron is revealed.
	Public bool `json:"public"`
}

const (
	patronNameMaxLength  = 80
	patronProseMaxLength = 4000
	favorReasonMaxLength = 200
	sortOrderLimit       = 1_000_000
)

var (
	errPatronNotFound       = NotFound("That patron doesn't exist.")
	errPatronActionNotFound = NotFound("That action doesn't exist.")
	errPatronLimit          = Conflict("This campaign has " + formatNumber(PatronLimit) + " patrons, the most it can hold. Archive one first.")
	errPatronTotalLimit     = Conflict("This campaign has kept " + formatNumber(PatronLimitIncludingArchived) +
		" patrons, archived ones included, the most it can hold.")
	errPatronActionLimit = Conflict("This campaign has " + formatNumber(PatronActionLimit) +
		" patron actions, the most it can hold. Archive one first.")
	errPatronActionTotalLimit = Conflict("This campaign has kept " + formatNumber(PatronActionLimitIncludingArchived) +
		" patron actions, archived ones included, the most it can hold.")
)

// ListPatrons is /patrons as the viewer's projection sees it.
func (service *Service) ListPatrons(requestContext context.Context, viewer Viewer) (PatronList, error) {
	if err := requireActive(viewer); err != nil {
		return PatronList{}, err
	}
	dmProjection := viewer.SeesDmFields()
	rows, err := service.queries.ListPatrons(requestContext, store.ListPatronsParams{CampaignID: viewer.campaignID, DmProjection: dmProjection})
	if err != nil {
		return PatronList{}, err
	}
	patrons := make([]Patron, 0, len(rows))
	for _, row := range rows {
		patrons = append(patrons, patronFromRow(row, dmProjection))
	}
	return PatronList{Patrons: patrons}, nil
}

// GetPatron is one patron's page. A hidden patron is not found for players, exactly like a missing one.
func (service *Service) GetPatron(requestContext context.Context, viewer Viewer, patronID uuid.UUID) (PatronPage, error) {
	if err := requireActive(viewer); err != nil {
		return PatronPage{}, err
	}
	return service.readPatronPage(requestContext, service.queries, viewer, patronID)
}

// ListPatronActions is every action the viewer may see, for pickers ("Use action now", "Ask a patron").
// Archived actions stay in the DM's list, marked, so history can name them.
func (service *Service) ListPatronActions(requestContext context.Context, viewer Viewer) ([]PatronAction, error) {
	if err := requireActive(viewer); err != nil {
		return nil, err
	}
	return listPatronActions(requestContext, service.queries, viewer.campaignID, nil, viewer.SeesDmFields())
}

func (service *Service) CreatePatron(requestContext context.Context, viewer Viewer, input NewPatronInput) (PatronPage, error) {
	if err := requireDm(viewer); err != nil {
		return PatronPage{}, err
	}
	var page PatronPage
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		counts, err := queries.CountPatrons(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if counts.Unarchived >= PatronLimit {
			return errPatronLimit
		}
		if counts.Total >= PatronLimitIncludingArchived {
			return errPatronTotalLimit
		}
		cleaned, err := cleanPatronInput(input.PatronInput)
		if err != nil {
			return err
		}
		row, err := queries.CreatePatron(requestContext, store.CreatePatronParams{
			CampaignID:  viewer.campaignID,
			Name:        cleaned.Name,
			Description: cleaned.Description,
			Revealed:    input.Revealed,
			PlayerNotes: cleaned.PlayerNotes,
			DmNotes:     cleaned.DmNotes,
			SortOrder:   cleaned.SortOrder,
		})
		if err != nil {
			return err
		}
		page, err = service.readPatronPage(requestContext, queries, viewer, row.ID)
		return err
	})
	if err != nil {
		return PatronPage{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicPatrons)
	return page, nil
}

func (service *Service) UpdatePatron(requestContext context.Context, viewer Viewer, patronID uuid.UUID, input PatronInput) (PatronPage, error) {
	if err := requireDm(viewer); err != nil {
		return PatronPage{}, err
	}
	cleaned, err := cleanPatronInput(input)
	if err != nil {
		return PatronPage{}, err
	}
	cleaned.CampaignID = viewer.campaignID
	cleaned.ID = patronID
	return service.changePatron(requestContext, viewer, patronID, func(queries *store.Queries) error {
		_, err := queries.UpdatePatron(requestContext, cleaned)
		return err
	})
}

// SetPatronRevealed is the DM's Visible to players switch. Hiding a patron hides their actions,
// requests and the entries that name them from players.
func (service *Service) SetPatronRevealed(requestContext context.Context, viewer Viewer, patronID uuid.UUID, revealed bool) (PatronPage, error) {
	if err := requireDm(viewer); err != nil {
		return PatronPage{}, err
	}
	return service.changePatron(requestContext, viewer, patronID, func(queries *store.Queries) error {
		_, err := queries.SetPatronRevealed(requestContext, store.SetPatronRevealedParams{CampaignID: viewer.campaignID, ID: patronID, Revealed: revealed})
		return err
	}, TopicQuests, TopicRequests, TopicTimeline)
}

// SetPatronArchived archives a patron (or brings one back). Patrons are never deleted (spec §5.7).
func (service *Service) SetPatronArchived(requestContext context.Context, viewer Viewer, patronID uuid.UUID, archived bool) (PatronPage, error) {
	if err := requireDm(viewer); err != nil {
		return PatronPage{}, err
	}
	return service.changePatron(requestContext, viewer, patronID, func(queries *store.Queries) error {
		if !archived {
			if err := requireRoomToUnarchivePatron(requestContext, queries, viewer.campaignID, patronID); err != nil {
				return err
			}
		}
		_, err := queries.SetPatronArchived(requestContext, store.SetPatronArchivedParams{CampaignID: viewer.campaignID, ID: patronID, Archived: archived})
		return err
	}, TopicQuests)
}

// Bringing a patron back counts against PatronLimit like creating one, under the same colony lock.
func requireRoomToUnarchivePatron(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, patronID uuid.UUID) error {
	if _, err := queries.GetColonyForUpdate(requestContext, campaignID); err != nil {
		return campaignGone(err)
	}
	patron, err := queries.GetPatron(requestContext, store.GetPatronParams{CampaignID: campaignID, ID: patronID})
	if err != nil {
		return err
	}
	if !patron.Archived {
		return nil
	}
	counts, err := queries.CountPatrons(requestContext, campaignID)
	if err != nil {
		return err
	}
	if counts.Unarchived >= PatronLimit {
		return errPatronLimit
	}
	return nil
}

func (service *Service) changePatron(requestContext context.Context, viewer Viewer, patronID uuid.UUID, change func(queries *store.Queries) error, topics ...Topic) (PatronPage, error) {
	var page PatronPage
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if err := change(queries); err != nil {
			return patronGone(err)
		}
		var err error
		page, err = service.readPatronPage(requestContext, queries, viewer, patronID)
		return err
	})
	if err != nil {
		return PatronPage{}, err
	}
	service.broker.Publish(viewer.campaignID, append([]Topic{TopicPatrons}, topics...)...)
	return page, nil
}

// AdjustFavor is the DM's favor adjustment, a colony mutation (spec §5.4): unbounded, so it may take
// favor below 0, and logged as FAVOR_ADJUSTED, DM-only unless the DM opts in while the patron is
// revealed.
func (service *Service) AdjustFavor(requestContext context.Context, viewer Viewer, patronID uuid.UUID, input FavorAdjustInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		patron, err := queries.GetPatronForUpdate(requestContext, store.GetPatronForUpdateParams{CampaignID: viewer.campaignID, ID: patronID})
		if err != nil {
			return colonyOutcome{}, patronGone(err)
		}
		reason, fieldError := cleanText("reason", input.Reason, favorReasonMaxLength)
		if fieldError != nil {
			return colonyOutcome{}, Invalid(fieldError.Message, *fieldError)
		}
		var delta int64
		switch {
		case (input.Delta == nil) == (input.SetTo == nil):
			message := "Give either a change or a new amount."
			return colonyOutcome{}, Invalid(message, FieldError{Field: "delta", Message: message})
		case input.Delta != nil:
			delta = *input.Delta
		default:
			delta = *input.SetTo - int64(patron.Favor)
		}
		after, err := game.AddFavor(game.Patron{Name: patron.Name, Favor: int64(patron.Favor)}, delta)
		if err != nil {
			return colonyOutcome{}, gameError(err, map[string]string{"favor": "delta"})
		}
		if after.Favor == int64(patron.Favor) {
			return colonyOutcome{}, Invalid("Nothing would change: " + patron.Name + " already has " + formatNumber(after.Favor) + " favor.")
		}
		if err := queries.SetPatronFavor(requestContext, store.SetPatronFavorParams{CampaignID: viewer.campaignID, ID: patron.ID, Favor: int32(after.Favor)}); err != nil {
			return colonyOutcome{}, err
		}
		visibility := TimelineVisibilityDm
		if input.Public && patron.Revealed {
			visibility = TimelineVisibilityPublic
		}
		entry := plannedEntry{
			day:        colony.state.Day,
			kind:       TimelineKindFavorAdjusted,
			title:      "Favor adjusted: " + patron.Name,
			body:       joinLines(favorChangeText(patron.Name, int64(patron.Favor), after.Favor), reason),
			visibility: visibility,
			patronID:   &patron.ID,
		}
		return colonyOutcome{
			after:       colony.state,
			label:       favorLine(patron.Name, after.Favor-int64(patron.Favor)),
			entries:     []plannedEntry{entry},
			patronFavor: map[uuid.UUID]favorChange{patron.ID: {before: int64(patron.Favor), after: after.Favor}},
			topics:      []Topic{TopicPatrons},
		}, nil
	})
}

func (service *Service) CreatePatronAction(requestContext context.Context, viewer Viewer, input NewPatronActionInput) (PatronAction, error) {
	if err := requireDm(viewer); err != nil {
		return PatronAction{}, err
	}
	var action PatronAction
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		counts, err := queries.CountPatronActions(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if counts.Unarchived >= PatronActionLimit {
			return errPatronActionLimit
		}
		if counts.Total >= PatronActionLimitIncludingArchived {
			return errPatronActionTotalLimit
		}
		patron, err := queries.GetPatron(requestContext, store.GetPatronParams{CampaignID: viewer.campaignID, ID: input.PatronID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Invalid("That patron doesn't exist.", FieldError{Field: "patronId", Message: "That patron doesn't exist."})
		}
		if err != nil {
			return err
		}
		if patron.Archived {
			message := patron.Name + " is archived."
			return Invalid(message, FieldError{Field: "patronId", Message: message})
		}
		cleaned, err := cleanPatronActionInput(requestContext, queries, viewer.campaignID, input.PatronActionInput)
		if err != nil {
			return err
		}
		row, err := queries.CreatePatronAction(requestContext, store.CreatePatronActionParams{
			CampaignID:  viewer.campaignID,
			PatronID:    patron.ID,
			Name:        cleaned.Name,
			Description: cleaned.Description,
			FavorCost:   cleaned.FavorCost,
			Effects:     cleaned.Effects,
			Revealed:    input.Revealed,
			SortOrder:   cleaned.SortOrder,
		})
		if err != nil {
			return err
		}
		action, err = patronActionFromRow(row, true)
		return err
	})
	if err != nil {
		return PatronAction{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicPatrons)
	return action, nil
}

// UpdatePatronAction replaces an action's record. Requests already made keep their snapshot.
func (service *Service) UpdatePatronAction(requestContext context.Context, viewer Viewer, actionID uuid.UUID, input PatronActionInput) (PatronAction, error) {
	if err := requireDm(viewer); err != nil {
		return PatronAction{}, err
	}
	var action PatronAction
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		cleaned, err := cleanPatronActionInput(requestContext, queries, viewer.campaignID, input)
		if err != nil {
			return err
		}
		cleaned.ID = actionID
		row, err := queries.UpdatePatronAction(requestContext, cleaned)
		if err != nil {
			return patronActionGone(err)
		}
		action, err = patronActionFromRow(row, true)
		return err
	})
	if err != nil {
		return PatronAction{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicPatrons)
	return action, nil
}

// SetPatronActionRevealed is the action's Visible to players switch; hiding it hides its requests and
// their entries from players.
func (service *Service) SetPatronActionRevealed(requestContext context.Context, viewer Viewer, actionID uuid.UUID, revealed bool) (PatronAction, error) {
	if err := requireDm(viewer); err != nil {
		return PatronAction{}, err
	}
	row, err := service.queries.SetPatronActionRevealed(requestContext, store.SetPatronActionRevealedParams{CampaignID: viewer.campaignID, ID: actionID, Revealed: revealed})
	if err != nil {
		return PatronAction{}, patronActionGone(err)
	}
	service.broker.Publish(viewer.campaignID, TopicPatrons, TopicRequests, TopicTimeline)
	return patronActionFromRow(row, true)
}

// SetPatronActionArchived archives an action (or brings one back); actions are never deleted.
func (service *Service) SetPatronActionArchived(requestContext context.Context, viewer Viewer, actionID uuid.UUID, archived bool) (PatronAction, error) {
	if err := requireDm(viewer); err != nil {
		return PatronAction{}, err
	}
	var action PatronAction
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if !archived {
			if err := requireRoomToUnarchivePatronAction(requestContext, queries, viewer.campaignID, actionID); err != nil {
				return err
			}
		}
		row, err := queries.SetPatronActionArchived(requestContext, store.SetPatronActionArchivedParams{CampaignID: viewer.campaignID, ID: actionID, Archived: archived})
		if err != nil {
			return patronActionGone(err)
		}
		action, err = patronActionFromRow(row, true)
		return err
	})
	if err != nil {
		return PatronAction{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicPatrons)
	return action, nil
}

// Bringing an action back counts against PatronActionLimit like creating one, under the same colony lock.
func requireRoomToUnarchivePatronAction(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, actionID uuid.UUID) error {
	if _, err := queries.GetColonyForUpdate(requestContext, campaignID); err != nil {
		return campaignGone(err)
	}
	action, err := queries.GetPatronAction(requestContext, store.GetPatronActionParams{CampaignID: campaignID, ID: actionID})
	if err != nil {
		return patronActionGone(err)
	}
	if !action.Archived {
		return nil
	}
	counts, err := queries.CountPatronActions(requestContext, campaignID)
	if err != nil {
		return err
	}
	if counts.Unarchived >= PatronActionLimit {
		return errPatronActionLimit
	}
	return nil
}

func (service *Service) readPatronPage(requestContext context.Context, queries *store.Queries, viewer Viewer, patronID uuid.UUID) (PatronPage, error) {
	dmProjection := viewer.SeesDmFields()
	row, err := queries.GetPatronView(requestContext, store.GetPatronViewParams{CampaignID: viewer.campaignID, ID: patronID, DmProjection: dmProjection})
	if err != nil {
		return PatronPage{}, patronGone(err)
	}
	actions, err := listPatronActions(requestContext, queries, viewer.campaignID, &patronID, dmProjection)
	if err != nil {
		return PatronPage{}, err
	}
	requests, err := listRequests(requestContext, queries, viewer, RequestFilter{PatronID: &patronID})
	if err != nil {
		return PatronPage{}, err
	}
	return PatronPage{Patron: patronFromRow(row, dmProjection), Actions: actions, Requests: requests}, nil
}

func listPatronActions(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, patronID *uuid.UUID, dmProjection bool) ([]PatronAction, error) {
	rows, err := queries.ListPatronActions(requestContext, store.ListPatronActionsParams{CampaignID: campaignID, PatronID: patronID, DmProjection: dmProjection})
	if err != nil {
		return nil, err
	}
	actions := make([]PatronAction, 0, len(rows))
	for _, row := range rows {
		action, err := patronActionFromRow(row, dmProjection)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, nil
}

func cleanPatronInput(input PatronInput) (store.UpdatePatronParams, error) {
	var fieldErrors []FieldError
	collect := func(fieldError *FieldError) {
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
	}
	var parameters store.UpdatePatronParams
	var fieldError *FieldError
	parameters.Name, fieldError = cleanText("name", input.Name, patronNameMaxLength)
	collect(fieldError)
	if fieldError == nil && parameters.Name == "" {
		collect(&FieldError{Field: "name", Message: "Give the patron a name."})
	}
	parameters.Description, fieldError = cleanMultilineText("description", input.Description, patronProseMaxLength)
	collect(fieldError)
	parameters.PlayerNotes, fieldError = cleanMultilineText("playerNotes", input.PlayerNotes, patronProseMaxLength)
	collect(fieldError)
	parameters.DmNotes, fieldError = cleanMultilineText("dmNotes", input.DmNotes, patronProseMaxLength)
	collect(fieldError)
	collect(checkSortOrder(input.SortOrder))
	if len(fieldErrors) > 0 {
		return store.UpdatePatronParams{}, Invalid("Check the patron's details.", fieldErrors...)
	}
	parameters.SortOrder = int32(input.SortOrder)
	return parameters, nil
}

func cleanPatronActionInput(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, input PatronActionInput) (store.UpdatePatronActionParams, error) {
	var fieldErrors []FieldError
	collect := func(fieldError *FieldError) {
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
	}
	parameters := store.UpdatePatronActionParams{CampaignID: campaignID}
	var fieldError *FieldError
	parameters.Name, fieldError = cleanText("name", input.Name, patronNameMaxLength)
	collect(fieldError)
	if fieldError == nil && parameters.Name == "" {
		collect(&FieldError{Field: "name", Message: "Give the action a name."})
	}
	parameters.Description, fieldError = cleanMultilineText("description", input.Description, patronProseMaxLength)
	collect(fieldError)
	if input.FavorCost < 0 || input.FavorCost > game.MaxStored {
		collect(&FieldError{Field: "favorCost", Message: "Use a whole number from 0 up."})
	}
	collect(checkSortOrder(input.SortOrder))
	resources, err := campaignResources(requestContext, queries, campaignID)
	if err != nil {
		return store.UpdatePatronActionParams{}, err
	}
	effects, effectsErrors := cleanEffects("effects", input.Effects, resources)
	fieldErrors = append(fieldErrors, effectsErrors...)
	if len(fieldErrors) > 0 {
		return store.UpdatePatronActionParams{}, Invalid("Check the action's details.", fieldErrors...)
	}
	parameters.FavorCost = int32(input.FavorCost)
	parameters.SortOrder = int32(input.SortOrder)
	if parameters.Effects, err = encodeEffects(effects); err != nil {
		return store.UpdatePatronActionParams{}, err
	}
	return parameters, nil
}

func checkSortOrder(sortOrder int64) *FieldError {
	if sortOrder < -sortOrderLimit || sortOrder > sortOrderLimit {
		return &FieldError{Field: "sortOrder", Message: "Use a position from −1,000,000 to 1,000,000."}
	}
	return nil
}

func patronGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errPatronNotFound
	}
	return err
}

func patronActionGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errPatronActionNotFound
	}
	return err
}

func patronFromRow(row store.Patron, dmProjection bool) Patron {
	patron := Patron{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Favor:       int64(row.Favor),
		PlayerNotes: row.PlayerNotes,
		Archived:    row.Archived,
		SortOrder:   int64(row.SortOrder),
	}
	if dmProjection {
		patron.Dm = &PatronDm{Revealed: row.Revealed, DmNotes: row.DmNotes}
	}
	return patron
}

func patronActionFromRow(row store.PatronAction, dmProjection bool) (PatronAction, error) {
	effects, err := decodeEffects(row.Effects)
	if err != nil {
		return PatronAction{}, err
	}
	action := PatronAction{
		ID:          row.ID,
		PatronID:    row.PatronID,
		Name:        row.Name,
		Description: row.Description,
		FavorCost:   int64(row.FavorCost),
		Effects:     effects,
		Archived:    row.Archived,
		SortOrder:   int64(row.SortOrder),
	}
	if dmProjection {
		action.Dm = &PatronActionDm{Revealed: row.Revealed}
	}
	return action, nil
}
