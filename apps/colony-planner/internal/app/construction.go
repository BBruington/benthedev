package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerConstruction(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listConstruction",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/construction",
		Summary:     "Blueprints, buildings and idle colonists per labor role, in the viewer's projection",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.ConstructionPage], error) {
		page, err := handlers.core.ListConstruction(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ConstructionPage]{Body: page}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createBlueprint",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/construction/blueprints",
		Summary:       "Add a blueprint, hidden unless asked otherwise",
		Tags:          []string{"construction"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.NewBlueprintInput]) (*bodyOutput[core.BuildingBlueprint], error) {
		blueprint, err := handlers.core.CreateBlueprint(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BuildingBlueprint]{Body: blueprint}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateBlueprint",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/construction/blueprints/{blueprintId}",
		Summary:     "Replace a blueprint's record (visibility and archiving each have their own operation); started buildings keep their snapshot",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, input *blueprintBodyInput[core.BlueprintInput]) (*bodyOutput[core.BuildingBlueprint], error) {
		blueprint, err := handlers.core.UpdateBlueprint(requestContext, viewerFrom(requestContext), input.BlueprintID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BuildingBlueprint]{Body: blueprint}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setBlueprintRevealed",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/construction/blueprints/{blueprintId}/revealed",
		Summary:     "Reveal a blueprint and its buildings to the players, or hide them again",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, input *blueprintBodyInput[RevealInput]) (*bodyOutput[core.BuildingBlueprint], error) {
		blueprint, err := handlers.core.SetBlueprintRevealed(requestContext, viewerFrom(requestContext), input.BlueprintID, input.Body.Revealed)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BuildingBlueprint]{Body: blueprint}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setBlueprintArchived",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/construction/blueprints/{blueprintId}/archived",
		Summary:     "Archive a blueprint (gone from players' Start list, its buildings stand) or bring it back",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, input *blueprintBodyInput[ArchiveInput]) (*bodyOutput[core.BuildingBlueprint], error) {
		blueprint, err := handlers.core.SetBlueprintArchived(requestContext, viewerFrom(requestContext), input.BlueprintID, input.Body.Archived)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BuildingBlueprint]{Body: blueprint}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "addBuilding",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/construction/blueprints/{blueprintId}/buildings",
		Summary:       "Add a finished, Active building of a blueprint at once and for free: no cost, labor or completion Effects; the DM may undo it",
		Tags:          []string{"construction"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *blueprintBodyInput[core.AddBuildingInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.AddBuilding(requestContext, viewerFrom(requestContext), input.BlueprintID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID:   "startBuilding",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/construction/buildings",
		Summary:       "Start a building: takes its cost in full and sets its first crews; the DM may undo it",
		Tags:          []string{"construction"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.StartBuildingInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.StartBuilding(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "setBuildingCrews",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/construction/buildings/{buildingId}/crews",
		Summary:     "Replace a building's crews; a role left out gets none",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, input *buildingBodyInput[core.BuildingCrewsInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.SetBuildingCrews(requestContext, viewerFrom(requestContext), input.BuildingID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "cancelBuilding",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/construction/buildings/{buildingId}/cancel",
		Summary:     "Cancel a building under construction, giving back what the DM typed (capped)",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, input *buildingBodyInput[core.BuildingRefundInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.CancelBuilding(requestContext, viewerFrom(requestContext), input.BuildingID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "demolishBuilding",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/construction/buildings/{buildingId}/demolish",
		Summary:     "Demolish a finished building, giving back what the DM typed (capped)",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, input *buildingBodyInput[core.BuildingRefundInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.DemolishBuilding(requestContext, viewerFrom(requestContext), input.BuildingID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "idleBuilding",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/construction/buildings/{buildingId}/idle",
		Summary:     "Set a finished building Idle: its daily output and storage bonus stop, and stock above a cap that falls stays",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, input *buildingBodyInput[core.BuildingVersionInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.IdleBuilding(requestContext, viewerFrom(requestContext), input.BuildingID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "activateBuilding",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/construction/buildings/{buildingId}/activate",
		Summary:     "Set an Idle building Active again: its storage bonus counts at once and its output runs from the next day",
		Tags:        []string{"construction"},
	}, func(requestContext context.Context, input *buildingBodyInput[core.BuildingVersionInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.ActivateBuilding(requestContext, viewerFrom(requestContext), input.BuildingID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})
}
