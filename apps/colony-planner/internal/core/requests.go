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

// Request is one request as a viewer sees it (spec §4.3, §5.7). The party sees every request whose
// patron and action they can see. Title, patron name, cost and effects are snapshots taken when it
// was made, so history reads correctly after the action is edited or archived. The decision note is
// per viewer: only the requester and the DM read it (Open Question 12), so it is decided here from the
// Viewer, not by the Transformer.
type Request struct {
	ID             uuid.UUID      `json:"id" format:"uuid"`
	Kind           RequestKind    `json:"kind"`
	Title          string         `json:"title" doc:"What was asked for, as it was named then"`
	PatronID       *uuid.UUID     `json:"patronId" format:"uuid" nullable:"true"`
	PatronName     string         `json:"patronName" doc:"The patron's name when it was asked"`
	PatronActionID *uuid.UUID     `json:"patronActionId" format:"uuid" nullable:"true"`
	FavorCost      int64          `json:"favorCost"`
	Effects        Effects        `json:"effects" doc:"What was asked for"`
	AppliedEffects *Effects       `json:"appliedEffects,omitempty" doc:"The DM's tweak to this grant; absent when the request applied as asked or isn't granted"`
	RequestedBy    RequestMember  `json:"requestedBy"`
	OnBehalfOf     *RequestMember `json:"onBehalfOf" doc:"The character the DM used the action for; null for none"`
	RequestedDay   int64          `json:"requestedDay"`
	Note           string         `json:"note"`
	Status         RequestStatus  `json:"status"`
	DecidedAt      *time.Time     `json:"decidedAt" doc:"null while pending"`
	DecisionNote   *string        `json:"decisionNote" nullable:"true" doc:"The DM's note; only the requester and the DM read it, so it is null for anyone else"`
	Mine           bool           `json:"mine" doc:"This viewer asked, so may withdraw it while it is pending"`
	CreatedAt      time.Time      `json:"createdAt"`
}

// RequestMember names the member behind a request by character name, or display name without one.
type RequestMember struct {
	_        struct{}  `nullable:"true"`
	MemberID uuid.UUID `json:"memberId" format:"uuid"`
	Name     string    `json:"name"`
}

type RequestList struct {
	Requests []Request `json:"requests" doc:"Pending first, then newest"`
}

// RequestFilter narrows the list; every zero value is off. Mine is "my requests".
type RequestFilter struct {
	Status   *RequestStatus
	Mine     bool
	PatronID *uuid.UUID
}

// RequestInput is a player's request (spec §5.7): POST /requests {actionId, note}.
type RequestInput struct {
	ActionID uuid.UUID `json:"actionId" format:"uuid"`
	Note     string    `json:"note" doc:"At most 500 characters; may be blank"`
}

// ApproveRequestInput is the approval sheet's Grant (spec §6.5): an optional tweak for this grant only
// and an optional note to the player.
type ApproveRequestInput struct {
	AppliedEffects *Effects `json:"appliedEffects,omitempty" doc:"Effects for this grant only; leave it out to apply what was asked"`
	Note           string   `json:"note" doc:"At most 500 characters; only the requester and the DM read it"`
}

type DeclineRequestInput struct {
	Note string `json:"note" doc:"At most 500 characters; only the requester and the DM read it"`
}

// UseActionInput is the DM's "Use action now": a request created and granted in one step, credited
// to OnBehalfOf on the timeline when it is set (spec §5.7).
type UseActionInput struct {
	ExpectedVersion int64      `json:"expectedVersion"`
	ActionID        uuid.UUID  `json:"actionId" format:"uuid"`
	OnBehalfOf      *uuid.UUID `json:"onBehalfOf" format:"uuid" nullable:"true" doc:"The member the timeline credits; null credits the DM"`
	AppliedEffects  *Effects   `json:"appliedEffects,omitempty" doc:"Effects for this use only; leave it out to apply the action's"`
	Note            string     `json:"note" doc:"At most 500 characters; may be blank"`
}

// RequestApprovalPreview is the approval sheet's before → after and validity line (spec §6.5).
type RequestApprovalPreview struct {
	Affordable      bool                    `json:"affordable"`
	Shortfall       string                  `json:"shortfall" doc:"e.g. Not enough favor: Lady Vex has 2, needs 3; blank when affordable"`
	Resources       []ResourceChangePreview `json:"resources"`
	ColonistsBefore int64                   `json:"colonistsBefore"`
	ColonistsAfter  int64                   `json:"colonistsAfter"`
	Favor           *FavorChangePreview     `json:"favor" doc:"null for a request with no patron"`
	Summary         string                  `json:"summary" doc:"e.g. Lady Vex favor 12 → 9 · Food 40 → 47"`
}

// requestCost is a request's cost snapshot: {"favor": 3}.
type requestCost struct {
	Favor int64 `json:"favor"`
}

const (
	requestListLimit       = 500
	messageRequestDecided  = "That request was already decided."
	messageRequestNotFound = "That request doesn't exist."
	messageDmUsesActionNow = "Use action now to use an action yourself."
)

var errRequestNotFound = NotFound(messageRequestNotFound)

// ListRequests is the DM's queue and history, or a player's party history; Mine narrows it to the
// viewer's own requests.
func (service *Service) ListRequests(requestContext context.Context, viewer Viewer, filter RequestFilter) (RequestList, error) {
	if err := requireActive(viewer); err != nil {
		return RequestList{}, err
	}
	requests, err := listRequests(requestContext, service.queries, viewer, filter)
	if err != nil {
		return RequestList{}, err
	}
	return RequestList{Requests: requests}, nil
}

// GetRequest is one request. One whose patron or action is hidden is not found for players.
func (service *Service) GetRequest(requestContext context.Context, viewer Viewer, requestID uuid.UUID) (Request, error) {
	if err := requireActive(viewer); err != nil {
		return Request{}, err
	}
	return readRequest(requestContext, service.queries, viewer, requestID)
}

// CreateRequest asks for a patron action (spec §5.7). The action and its patron must be visible and
// unarchived, and the request affordable right now in STRICT terms, or it is refused (422) with the
// shortfall named. Nothing is reserved: approval checks again. It is not a colony mutation. Requests are
// the players' (spec §5.7): the DM, viewing as player or not, uses an action with UseActionNow.
func (service *Service) CreateRequest(requestContext context.Context, viewer Viewer, input RequestInput) (Request, error) {
	if err := requireActive(viewer); err != nil {
		return Request{}, err
	}
	if viewer.IsDM() {
		return Request{}, Forbidden(messageDmUsesActionNow)
	}
	note, fieldError := cleanText("note", input.Note, requestNoteMaxLength)
	if fieldError != nil {
		return Request{}, Invalid(fieldError.Message, *fieldError)
	}
	var request Request
	var prompt RequestPrompt
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := readColony(requestContext, queries, viewer.campaignID, false)
		if err != nil {
			return err
		}
		action, patron, err := loadRequestableAction(requestContext, queries, viewer.campaignID, input.ActionID, false)
		if err != nil {
			return err
		}
		effects, err := decodeEffects(action.Effects)
		if err != nil {
			return err
		}
		if _, err := game.Spend(colony.state, game.Patron{Name: patron.Name, Favor: int64(patron.Favor)}, int64(action.FavorCost), effects); err != nil {
			return gameError(err, nil)
		}
		row, err := insertRequestSnapshot(requestContext, queries, viewer, colony, action, patron, nil, note)
		if err != nil {
			return err
		}
		request, err = readRequest(requestContext, queries, viewer, row.ID)
		if err != nil {
			return err
		}
		prompt = requestPrompt(service, viewer.campaignID, request, colony, int64(patron.Favor))
		return nil
	})
	if err != nil {
		return Request{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicRequests)
	service.requestNotifier.RequestCreated(requestContext, prompt)
	return request, nil
}

// WithdrawRequest takes back the viewer's own pending request (spec §5.7).
func (service *Service) WithdrawRequest(requestContext context.Context, viewer Viewer, requestID uuid.UUID) (Request, error) {
	if err := requireActive(viewer); err != nil {
		return Request{}, err
	}
	var request Request
	var outcome *RequestOutcome
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// A request this viewer can't see, or someone else's, is not found, so withdrawing never
		// confirms one they can't see.
		existing, err := readRequest(requestContext, queries, viewer, requestID)
		if err != nil {
			return err
		}
		if !existing.Mine {
			return errRequestNotFound
		}
		row, err := queries.WithdrawRequest(requestContext, store.WithdrawRequestParams{CampaignID: viewer.campaignID, ID: requestID, RequestedBy: viewer.memberID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Conflict(messageRequestDecided)
		}
		if err != nil {
			return err
		}
		outcome = decidedOutcome(row, "Withdrawn")
		request, err = readRequest(requestContext, queries, viewer, requestID)
		return err
	})
	if err != nil {
		return Request{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicRequests)
	if outcome != nil {
		service.requestNotifier.RequestDecided(requestContext, *outcome)
	}
	return request, nil
}

// PreviewApproveRequest is the approval sheet's before → after and validity line, without writing.
func (service *Service) PreviewApproveRequest(requestContext context.Context, viewer Viewer, requestID uuid.UUID, input ApproveRequestInput) (RequestApprovalPreview, error) {
	if err := requireDm(viewer); err != nil {
		return RequestApprovalPreview{}, err
	}
	colony, err := readColony(requestContext, service.queries, viewer.campaignID, false)
	if err != nil {
		return RequestApprovalPreview{}, err
	}
	request, err := service.queries.GetRequest(requestContext, store.GetRequestParams{CampaignID: viewer.campaignID, ID: requestID})
	if err != nil {
		return RequestApprovalPreview{}, requestGone(err)
	}
	if request.Status != RequestStatusPending {
		return RequestApprovalPreview{}, Conflict(messageRequestDecided)
	}
	var patron *store.Patron
	if request.PatronID != nil {
		row, err := service.queries.GetPatron(requestContext, store.GetPatronParams{CampaignID: viewer.campaignID, ID: *request.PatronID})
		if err != nil {
			return RequestApprovalPreview{}, err
		}
		patron = &row
	}
	cost, effects, err := grantTerms(colony, request, input.AppliedEffects)
	if err != nil {
		return RequestApprovalPreview{}, err
	}
	preview := RequestApprovalPreview{Affordable: true, Resources: []ResourceChangePreview{}}
	result, err := spendForRequest(colony, patron, cost, effects)
	var shortfallError *game.ShortfallError
	if errors.As(err, &shortfallError) {
		// A grant that can't happen has no after: favor never goes below 0 by spending (spec §5.4), so
		// the sheet shows only the shortfall, which already says how far off it is.
		colonists := game.Colonists(colony.state.Roles)
		return RequestApprovalPreview{Shortfall: shortfallError.Error(), Resources: []ResourceChangePreview{}, ColonistsBefore: colonists, ColonistsAfter: colonists}, nil
	}
	if err != nil {
		return RequestApprovalPreview{}, gameError(err, nil)
	}
	resources, parts := effectsPreview(colony, result.Effects)
	preview.Resources = resources
	preview.ColonistsBefore = game.Colonists(result.Effects.Before.Roles)
	preview.ColonistsAfter = game.Colonists(result.Effects.After.Roles)
	if patron != nil {
		preview.Favor = &FavorChangePreview{PatronID: patron.ID, PatronName: patron.Name, Before: result.PatronBefore.Favor, After: result.PatronAfter.Favor}
		parts = append([]string{favorChangeText(patron.Name, result.PatronBefore.Favor, result.PatronAfter.Favor)}, parts...)
	}
	preview.Summary = joinParts(parts...)
	return preview, nil
}

// ApproveRequest grants a pending request (spec §5.7), from the approval sheet or Beny Bot's Grant.
// It locks the colony, then the patron, requires PENDING, and applies the cost and the effects (the
// snapshot, or this grant's tweak) in STRICT mode; anything short is a 409 naming the shortfall. It is
// a colony mutation guarded by the request's status, so a second Grant pays nothing.
func (service *Service) ApproveRequest(requestContext context.Context, viewer Viewer, requestID uuid.UUID, input ApproveRequestInput) (ColonyChange, error) {
	var outcomeText string
	return service.approveRequest(requestContext, viewer, requestID, input, &outcomeText)
}

func (service *Service) approveRequest(requestContext context.Context, viewer Viewer, requestID uuid.UUID, input ApproveRequestInput, outcomeText *string) (ColonyChange, error) {
	if err := requireDm(viewer); err != nil {
		return ColonyChange{}, err
	}
	note, fieldError := cleanText("note", input.Note, requestNoteMaxLength)
	if fieldError != nil {
		return ColonyChange{}, Invalid(fieldError.Message, *fieldError)
	}
	return service.mutateColonyGuardedByStatus(requestContext, viewer, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		request, err := queries.GetRequest(requestContext, store.GetRequestParams{CampaignID: viewer.campaignID, ID: requestID})
		if err != nil {
			return colonyOutcome{}, requestGone(err)
		}
		if request.Status != RequestStatusPending {
			return colonyOutcome{}, Conflict(messageRequestDecided)
		}
		return service.grant(requestContext, queries, viewer, colony, request, grantChoice{appliedEffects: input.AppliedEffects, note: note, outcomeText: outcomeText})
	})
}

// DeclineRequest refuses a pending request with an optional note (spec §5.7). It writes no timeline
// entry and is not a colony mutation.
func (service *Service) DeclineRequest(requestContext context.Context, viewer Viewer, requestID uuid.UUID, input DeclineRequestInput) (Request, error) {
	if err := requireDm(viewer); err != nil {
		return Request{}, err
	}
	note, fieldError := cleanText("note", input.Note, requestNoteMaxLength)
	if fieldError != nil {
		return Request{}, Invalid(fieldError.Message, *fieldError)
	}
	var request Request
	var outcome *RequestOutcome
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		row, err := queries.DeclineRequest(requestContext, store.DeclineRequestParams{
			CampaignID:   viewer.campaignID,
			ID:           requestID,
			DecidedBy:    memberReference(viewer),
			DecisionNote: note,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			if _, err := queries.GetRequest(requestContext, store.GetRequestParams{CampaignID: viewer.campaignID, ID: requestID}); err != nil {
				return requestGone(err)
			}
			return Conflict(messageRequestDecided)
		}
		if err != nil {
			return err
		}
		outcome = decidedOutcome(row, "Declined")
		request, err = readRequest(requestContext, queries, viewer, requestID)
		return err
	})
	if err != nil {
		return Request{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicRequests)
	if outcome != nil {
		service.requestNotifier.RequestDecided(requestContext, *outcome)
	}
	return request, nil
}

// UseActionNow creates and grants a request in one step (spec §5.7). The DM may use a hidden action;
// an archived one, or one of an archived patron, is not offered.
func (service *Service) UseActionNow(requestContext context.Context, viewer Viewer, input UseActionInput) (ColonyChange, error) {
	if err := requireDm(viewer); err != nil {
		return ColonyChange{}, err
	}
	note, fieldError := cleanText("note", input.Note, requestNoteMaxLength)
	if fieldError != nil {
		return ColonyChange{}, Invalid(fieldError.Message, *fieldError)
	}
	// It makes a new request each time, so no status guards it: the version stops a repeat (spec §5.1).
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		action, patron, err := loadRequestableAction(requestContext, queries, viewer.campaignID, input.ActionID, true)
		if err != nil {
			return colonyOutcome{}, err
		}
		if input.OnBehalfOf != nil {
			member, err := queries.GetCampaignMember(requestContext, store.GetCampaignMemberParams{CampaignID: viewer.campaignID, ID: *input.OnBehalfOf})
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && member.CampaignMember.Status != MemberStatusActive) {
				message := "Pick an active member of this campaign."
				return colonyOutcome{}, Invalid(message, FieldError{Field: "onBehalfOf", Message: message})
			}
			if err != nil {
				return colonyOutcome{}, err
			}
		}
		row, err := insertRequestSnapshot(requestContext, queries, viewer, colony, action, patron, input.OnBehalfOf, note)
		if err != nil {
			return colonyOutcome{}, err
		}
		var outcomeText string
		return service.grant(requestContext, queries, viewer, colony, row, grantChoice{appliedEffects: input.AppliedEffects, created: true, outcomeText: &outcomeText})
	})
}

// DecideRequestFromDiscord is Beny Bot's Grant and Decline (spec §7.5). The bot builds viewer with
// ViewerForGuild for the clicking member, so the request is loaded with the linked campaign and only
// its DM may decide. Grant applies the snapshot with no tweak and no note; Decline has no note. The
// outcome's Text is what the message becomes ("Granted · Food +7 · favor 5 → 2", "Declined"). A
// shortfall is a Conflict whose message names it; the bot then keeps the buttons. Like a decision on
// the website, it also calls RequestNotifier.RequestDecided for the stored message.
func (service *Service) DecideRequestFromDiscord(requestContext context.Context, viewer Viewer, requestID uuid.UUID, grant bool) (RequestOutcome, error) {
	if err := requireDm(viewer); err != nil {
		return RequestOutcome{}, Forbidden("Only the DM can decide.")
	}
	outcome := RequestOutcome{CampaignID: viewer.campaignID, RequestID: requestID, Status: RequestStatusDeclined, Text: "Declined"}
	if grant {
		outcome.Status = RequestStatusApproved
		if _, err := service.approveRequest(requestContext, viewer, requestID, ApproveRequestInput{}, &outcome.Text); err != nil {
			return RequestOutcome{}, err
		}
	} else if _, err := service.DeclineRequest(requestContext, viewer, requestID, DeclineRequestInput{}); err != nil {
		return RequestOutcome{}, err
	}
	row, err := service.queries.GetRequest(requestContext, store.GetRequestParams{CampaignID: viewer.campaignID, ID: requestID})
	if err != nil {
		return RequestOutcome{}, requestGone(err)
	}
	if row.DiscordMessageID != nil {
		outcome.DiscordMessageID = *row.DiscordMessageID
	}
	return outcome, nil
}

// grantChoice is how the DM grants: an optional tweak and note, whether "Use action now" created the
// request, and where to put the text Beny Bot's message becomes.
type grantChoice struct {
	appliedEffects *Effects
	note           string
	created        bool
	outcomeText    *string
}

// grant applies a pending request inside a colony mutation: lock the patron (after the colony), spend
// in STRICT mode, mark it APPROVED behind its status guard, and write PATRON_ACTION.
func (service *Service) grant(requestContext context.Context, queries *store.Queries, viewer Viewer, colony colonySnapshot, request store.Request,
	choice grantChoice,
) (colonyOutcome, error) {
	appliedEffects := choice.appliedEffects
	var patron *store.Patron
	if request.PatronID != nil {
		row, err := queries.GetPatronForUpdate(requestContext, store.GetPatronForUpdateParams{CampaignID: viewer.campaignID, ID: *request.PatronID})
		if err != nil {
			return colonyOutcome{}, err
		}
		patron = &row
	}
	cost, effects, err := grantTerms(colony, request, appliedEffects)
	if err != nil {
		return colonyOutcome{}, err
	}
	result, err := spendForRequest(colony, patron, cost, effects)
	var shortfallError *game.ShortfallError
	if errors.As(err, &shortfallError) {
		return colonyOutcome{}, Conflict(shortfallError.Error() + ".")
	}
	if err != nil {
		return colonyOutcome{}, gameError(err, nil)
	}
	var appliedJSON []byte
	if appliedEffects != nil {
		if appliedJSON, err = encodeEffects(effects); err != nil {
			return colonyOutcome{}, err
		}
	}
	groupID := uuid.New()
	approved, err := queries.ApproveRequest(requestContext, store.ApproveRequestParams{
		CampaignID:     viewer.campaignID,
		ID:             request.ID,
		AppliedEffects: appliedJSON,
		DecidedBy:      memberReference(viewer),
		DecisionNote:   choice.note,
		EventGroupID:   &groupID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return colonyOutcome{}, Conflict(messageRequestDecided)
	}
	if err != nil {
		return colonyOutcome{}, err
	}
	outcome := colonyOutcome{
		groupID:  groupID,
		after:    result.Effects.After,
		requests: map[uuid.UUID]requestBefore{request.ID: {Created: choice.created}},
		topics:   []Topic{TopicRequests, TopicPatrons},
	}
	creditName, err := requestCreditName(requestContext, queries, viewer.campaignID, request)
	if err != nil {
		return colonyOutcome{}, err
	}
	patronName := ""
	if request.PatronName != nil {
		patronName = *request.PatronName
	}
	effectsText := appliedEffectsText(result.Effects.Applied, colony.order, colony.labels)
	body := effectsText
	switch {
	case cost > 0 && effectsText == "":
		body = "favor " + formatDelta(-cost)
	case cost > 0:
		body = effectsText + " (favor " + formatDelta(-cost) + ")"
	}
	entry := plannedEntry{
		day:        colony.state.Day,
		kind:       TimelineKindPatronAction,
		title:      creditName + " called in “" + request.Title + "” from " + patronName,
		body:       body,
		visibility: TimelineVisibilityDm,
		deltas:     result.Effects.Applied,
		patronID:   request.PatronID,
		requestID:  &request.ID,
	}
	if patron != nil {
		visible, err := actionVisibleToPlayers(requestContext, queries, viewer.campaignID, request.PatronActionID)
		if err != nil {
			return colonyOutcome{}, err
		}
		if patron.Revealed && visible {
			entry.visibility = TimelineVisibilityPublic
		}
		if result.PatronAfter.Favor != result.PatronBefore.Favor {
			if err := queries.SetPatronFavor(requestContext, store.SetPatronFavorParams{CampaignID: viewer.campaignID, ID: patron.ID, Favor: int32(result.PatronAfter.Favor)}); err != nil {
				return colonyOutcome{}, err
			}
			outcome.patronFavor = map[uuid.UUID]favorChange{patron.ID: {before: result.PatronBefore.Favor, after: result.PatronAfter.Favor}}
		}
	}
	outcome.entries = []plannedEntry{entry}
	outcomeText := joinParts("Granted", effectsText)
	if patron != nil && cost > 0 {
		outcomeText = joinParts(outcomeText, "favor "+formatNumber(result.PatronBefore.Favor)+" → "+formatNumber(result.PatronAfter.Favor))
	}
	outcome.label = entry.title
	*choice.outcomeText = outcomeText
	if approved.DiscordMessageID != nil {
		decided := RequestOutcome{
			CampaignID:       viewer.campaignID,
			RequestID:        request.ID,
			DiscordMessageID: *approved.DiscordMessageID,
			Status:           RequestStatusApproved,
			Text:             outcomeText,
		}
		outcome.afterCommit = append(outcome.afterCommit, func(requestContext context.Context) {
			service.requestNotifier.RequestDecided(requestContext, decided)
		})
	}
	return outcome, nil
}

// grantTerms is a request's favor cost and the effects to apply: the snapshot, or the DM's tweak,
// whose keys are checked against this campaign's resources.
func grantTerms(colony colonySnapshot, request store.Request, appliedEffects *Effects) (int64, Effects, error) {
	var cost requestCost
	if err := json.Unmarshal(request.Cost, &cost); err != nil {
		return 0, Effects{}, fmt.Errorf("core: decode request %s cost: %w", request.ID, err)
	}
	if appliedEffects == nil {
		effects, err := decodeEffects(request.Effects)
		return cost.Favor, effects, err
	}
	cleaned, fieldErrors := cleanEffects("appliedEffects", *appliedEffects, colony.resources)
	if len(fieldErrors) > 0 {
		return 0, Effects{}, Invalid("Check the grant's effects.", fieldErrors...)
	}
	return cost.Favor, cleaned, nil
}

func spendForRequest(colony colonySnapshot, patron *store.Patron, cost int64, effects Effects) (game.SpendResult, error) {
	if patron == nil {
		if cost > 0 {
			return game.SpendResult{}, Conflict("This request has no patron to pay its favor.")
		}
		return game.Spend(colony.state, game.Patron{}, 0, effects)
	}
	return game.Spend(colony.state, game.Patron{Name: patron.Name, Favor: int64(patron.Favor)}, cost, effects)
}

// loadRequestableAction loads an action and its patron with the viewer's campaign. A player may ask
// only for a visible action of a visible patron; the DM may use hidden ones. Neither is offered once
// archived.
func loadRequestableAction(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, actionID uuid.UUID, dmUse bool) (store.PatronAction, store.Patron, error) {
	notFound := Invalid("That action doesn't exist.", FieldError{Field: "actionId", Message: "That action doesn't exist."})
	action, err := queries.GetPatronAction(requestContext, store.GetPatronActionParams{CampaignID: campaignID, ID: actionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.PatronAction{}, store.Patron{}, notFound
	}
	if err != nil {
		return store.PatronAction{}, store.Patron{}, err
	}
	patron, err := queries.GetPatron(requestContext, store.GetPatronParams{CampaignID: campaignID, ID: action.PatronID})
	if err != nil {
		return store.PatronAction{}, store.Patron{}, err
	}
	if !dmUse && (!action.Revealed || !patron.Revealed) {
		return store.PatronAction{}, store.Patron{}, notFound
	}
	if action.Archived || patron.Archived {
		message := "That action is archived."
		return store.PatronAction{}, store.Patron{}, Invalid(message, FieldError{Field: "actionId", Message: message})
	}
	return action, patron, nil
}

func insertRequestSnapshot(requestContext context.Context, queries *store.Queries, viewer Viewer, colony colonySnapshot, action store.PatronAction,
	patron store.Patron, onBehalfOf *uuid.UUID, note string,
) (store.Request, error) {
	cost, err := json.Marshal(requestCost{Favor: int64(action.FavorCost)})
	if err != nil {
		return store.Request{}, fmt.Errorf("core: encode request cost: %w", err)
	}
	return queries.CreateRequest(requestContext, store.CreateRequestParams{
		CampaignID:     viewer.campaignID,
		Kind:           string(RequestKindPatronAction),
		Title:          action.Name,
		PatronID:       &patron.ID,
		PatronName:     &patron.Name,
		PatronActionID: &action.ID,
		Cost:           cost,
		Effects:        action.Effects,
		RequestedBy:    viewer.memberID,
		OnBehalfOf:     onBehalfOf,
		RequestedDay:   int32(colony.state.Day),
		Note:           note,
		// Use action now on a hidden patron or action writes a DM-only entry; its request stays DM-only
		// too, so revealing them later under new names never shows players the old ones.
		MadeWhileRevealed: patron.Revealed && action.Revealed,
	})
}

func actionVisibleToPlayers(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, actionID *uuid.UUID) (bool, error) {
	if actionID == nil {
		return true, nil
	}
	action, err := queries.GetPatronAction(requestContext, store.GetPatronActionParams{CampaignID: campaignID, ID: *actionID})
	if err != nil {
		return false, err
	}
	return action.Revealed, nil
}

// requestCreditName is who the timeline credits: the member the DM acted for, else the requester.
func requestCreditName(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, request store.Request) (string, error) {
	memberID := request.RequestedBy
	if request.OnBehalfOf != nil {
		memberID = *request.OnBehalfOf
	}
	member, err := queries.GetCampaignMember(requestContext, store.GetCampaignMemberParams{CampaignID: campaignID, ID: memberID})
	if err != nil {
		return "", err
	}
	return memberName(member.CampaignMember.CharacterName, member.Account.DisplayName), nil
}

func memberName(characterName *string, displayName string) string {
	if characterName != nil && *characterName != "" {
		return *characterName
	}
	return displayName
}

// requestPrompt is the DM channel's message for a new request, with the colony's labels.
func requestPrompt(service *Service, campaignID uuid.UUID, request Request, colony colonySnapshot, favorNow int64) RequestPrompt {
	requestID := request.ID
	return RequestPrompt{
		CampaignID:    campaignID,
		RequestID:     requestID,
		RequesterName: request.RequestedBy.Name,
		Title:         request.Title,
		PatronName:    request.PatronName,
		Terms:         formatNumber(request.FavorCost) + " favor → " + effectsOrNothing(appliedEffectsText(request.Effects, colony.order, colony.labels)),
		FavorNow:      formatNumber(favorNow),
		RecordMessage: func(requestContext context.Context, discordMessageID string) (*RequestOutcome, error) {
			status, err := service.queries.SetRequestDiscordMessage(requestContext, store.SetRequestDiscordMessageParams{
				CampaignID:       campaignID,
				ID:               requestID,
				DiscordMessageID: &discordMessageID,
			})
			if err != nil {
				return nil, err
			}
			text, decided := decidedPromptText[status]
			if !decided {
				return nil, nil
			}
			return &RequestOutcome{CampaignID: campaignID, RequestID: requestID, DiscordMessageID: discordMessageID, Status: status, Text: text}, nil
		},
	}
}

// decidedPromptText words a decision the prompt missed; a grant's terms and favor are on the website.
var decidedPromptText = map[RequestStatus]string{
	RequestStatusApproved:  "Granted",
	RequestStatusDeclined:  "Declined",
	RequestStatusWithdrawn: "Withdrawn",
}

func effectsOrNothing(text string) string {
	if text == "" {
		return "nothing"
	}
	return text
}

func decidedOutcome(row store.Request, text string) *RequestOutcome {
	if row.DiscordMessageID == nil {
		return nil
	}
	return &RequestOutcome{CampaignID: row.CampaignID, RequestID: row.ID, DiscordMessageID: *row.DiscordMessageID, Status: row.Status, Text: text}
}

func listRequests(requestContext context.Context, queries *store.Queries, viewer Viewer, filter RequestFilter) ([]Request, error) {
	parameters := store.ListRequestsParams{
		CampaignID:   viewer.campaignID,
		Status:       filter.Status,
		PatronID:     filter.PatronID,
		DmProjection: viewer.SeesDmFields(),
		ResultLimit:  requestListLimit,
	}
	if filter.Mine {
		memberID := viewer.memberID
		parameters.RequestedBy = &memberID
	}
	rows, err := queries.ListRequests(requestContext, parameters)
	if err != nil {
		return nil, err
	}
	requests := make([]Request, 0, len(rows))
	for _, row := range rows {
		request, err := requestFromRow(store.GetRequestViewRow(row), viewer)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func readRequest(requestContext context.Context, queries *store.Queries, viewer Viewer, requestID uuid.UUID) (Request, error) {
	row, err := queries.GetRequestView(requestContext, store.GetRequestViewParams{CampaignID: viewer.campaignID, ID: requestID, DmProjection: viewer.SeesDmFields()})
	if err != nil {
		return Request{}, requestGone(err)
	}
	return requestFromRow(row, viewer)
}

func requestGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errRequestNotFound
	}
	return err
}

// mayReadDecisionNote is the per-viewer rule (Open Question 12): the DM, and the player who asked.
// The DM viewing as player reads none, not even on a Use action now request recorded as theirs,
// because a player in their place would read none either.
func mayReadDecisionNote(viewer Viewer, request store.Request) bool {
	return viewer.SeesDmFields() || (!viewer.IsDM() && request.RequestedBy == viewer.memberID)
}

func requestFromRow(row store.GetRequestViewRow, viewer Viewer) (Request, error) {
	effects, err := decodeEffects(row.Request.Effects)
	if err != nil {
		return Request{}, err
	}
	var cost requestCost
	if err := json.Unmarshal(row.Request.Cost, &cost); err != nil {
		return Request{}, fmt.Errorf("core: decode request %s cost: %w", row.Request.ID, err)
	}
	request := Request{
		ID:             row.Request.ID,
		Kind:           RequestKind(row.Request.Kind),
		Title:          row.Request.Title,
		PatronID:       row.Request.PatronID,
		PatronActionID: row.Request.PatronActionID,
		FavorCost:      cost.Favor,
		Effects:        effects,
		RequestedBy: RequestMember{
			MemberID: row.Request.RequestedBy,
			Name:     memberName(row.RequesterCharacterName, row.RequesterDisplayName),
		},
		RequestedDay: int64(row.Request.RequestedDay),
		Note:         row.Request.Note,
		Status:       row.Request.Status,
		DecidedAt:    utcPointer(row.Request.DecidedAt),
		Mine:         row.Request.RequestedBy == viewer.memberID,
		CreatedAt:    row.Request.CreatedAt.UTC(),
	}
	if row.Request.PatronName != nil {
		request.PatronName = *row.Request.PatronName
	}
	if row.Request.AppliedEffects != nil {
		applied, err := decodeEffects(row.Request.AppliedEffects)
		if err != nil {
			return Request{}, err
		}
		request.AppliedEffects = &applied
	}
	if row.Request.OnBehalfOf != nil {
		displayName := ""
		if row.OnBehalfOfDisplayName != nil {
			displayName = *row.OnBehalfOfDisplayName
		}
		request.OnBehalfOf = &RequestMember{MemberID: *row.Request.OnBehalfOf, Name: memberName(row.OnBehalfOfCharacterName, displayName)}
	}
	if mayReadDecisionNote(viewer, row.Request) {
		note := row.Request.DecisionNote
		request.DecisionNote = &note
	}
	return request, nil
}
