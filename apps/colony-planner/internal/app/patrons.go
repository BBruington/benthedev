package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

// PatronActionList is every action the viewer may see, across patrons, for the request and Use action
// now pickers.
type PatronActionList struct {
	Actions []core.PatronAction `json:"actions" doc:"For players, only revealed, unarchived actions of revealed, unarchived patrons; the DM also gets archived ones, flagged"`
}

func (handlers *handlers) registerPatrons(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listPatrons",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/patrons",
		Summary:     "Patrons with their favor, in the viewer's projection",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.PatronList], error) {
		patrons, err := handlers.core.ListPatrons(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronList]{Body: patrons}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createPatron",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/patrons",
		Summary:       "Add a patron, hidden unless asked otherwise",
		Tags:          []string{"patrons"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.NewPatronInput]) (*bodyOutput[core.PatronPage], error) {
		page, err := handlers.core.CreatePatron(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronPage]{Body: page}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getPatron",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/patrons/{patronId}",
		Summary:     "One patron with its actions and request history; a hidden one is not found for players",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, input *patronPath) (*bodyOutput[core.PatronPage], error) {
		page, err := handlers.core.GetPatron(requestContext, viewerFrom(requestContext), input.PatronID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronPage]{Body: page}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updatePatron",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/patrons/{patronId}",
		Summary:     "Replace a patron's record (favor, visibility and archiving each have their own operation)",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, input *patronBodyInput[core.PatronInput]) (*bodyOutput[core.PatronPage], error) {
		page, err := handlers.core.UpdatePatron(requestContext, viewerFrom(requestContext), input.PatronID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronPage]{Body: page}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setPatronRevealed",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/patrons/{patronId}/revealed",
		Summary:     "Reveal a patron to the players, or hide them again",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, input *patronBodyInput[RevealInput]) (*bodyOutput[core.PatronPage], error) {
		page, err := handlers.core.SetPatronRevealed(requestContext, viewerFrom(requestContext), input.PatronID, input.Body.Revealed)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronPage]{Body: page}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setPatronArchived",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/patrons/{patronId}/archived",
		Summary:     "Archive a patron (gone from pickers and players' lists, history kept) or bring them back",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, input *patronBodyInput[ArchiveInput]) (*bodyOutput[core.PatronPage], error) {
		page, err := handlers.core.SetPatronArchived(requestContext, viewerFrom(requestContext), input.PatronID, input.Body.Archived)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronPage]{Body: page}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "adjustFavor",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/patrons/{patronId}/adjust-favor",
		Summary:     "Change a patron's favor by a delta or set it, below 0 included",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, input *patronBodyInput[core.FavorAdjustInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.AdjustFavor(requestContext, viewerFrom(requestContext), input.PatronID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "listPatronActions",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/patron-actions",
		Summary:     "Every patron action the viewer may see, for pickers (filter out archived ones there)",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[PatronActionList], error) {
		actions, err := handlers.core.ListPatronActions(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[PatronActionList]{Body: PatronActionList{Actions: actions}}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createPatronAction",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/patron-actions",
		Summary:       "Add an action to a patron, hidden unless asked otherwise",
		Tags:          []string{"patrons"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.NewPatronActionInput]) (*bodyOutput[core.PatronAction], error) {
		action, err := handlers.core.CreatePatronAction(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronAction]{Body: action}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updatePatronAction",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/patron-actions/{actionId}",
		Summary:     "Replace an action's record (visibility and archiving each have their own operation); pending requests keep what they asked for",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, input *patronActionBodyInput[core.PatronActionInput]) (*bodyOutput[core.PatronAction], error) {
		action, err := handlers.core.UpdatePatronAction(requestContext, viewerFrom(requestContext), input.ActionID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronAction]{Body: action}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setPatronActionRevealed",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/patron-actions/{actionId}/revealed",
		Summary:     "Reveal an action to the players, or hide it again",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, input *patronActionBodyInput[RevealInput]) (*bodyOutput[core.PatronAction], error) {
		action, err := handlers.core.SetPatronActionRevealed(requestContext, viewerFrom(requestContext), input.ActionID, input.Body.Revealed)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronAction]{Body: action}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setPatronActionArchived",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/patron-actions/{actionId}/archived",
		Summary:     "Archive an action (gone from pickers and players' lists, history kept) or bring it back",
		Tags:        []string{"patrons"},
	}, func(requestContext context.Context, input *patronActionBodyInput[ArchiveInput]) (*bodyOutput[core.PatronAction], error) {
		action, err := handlers.core.SetPatronActionArchived(requestContext, viewerFrom(requestContext), input.ActionID, input.Body.Archived)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PatronAction]{Body: action}, nil
	})
}
