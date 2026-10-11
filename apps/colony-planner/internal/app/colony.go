package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

type advancePreviewQuery struct {
	Days int64 `query:"days" required:"true" doc:"1 or more"`
}

func (handlers *handlers) registerColony(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "getColonyBoard",
		Method:      http.MethodGet,
		Path:        "/api/colony",
		Summary:     "The colony board: names, day, shipments, stock and headcounts",
		Tags:        []string{"colony"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.ColonyBoard], error) {
		board, err := handlers.core.GetColonyBoard(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyBoard]{Body: board}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "previewAdvance",
		Method:      http.MethodGet,
		Path:        "/api/colony/advance-preview",
		Summary:     "What advancing N days would do, without doing it",
		Tags:        []string{"colony"},
	}, func(requestContext context.Context, input *advancePreviewQuery) (*bodyOutput[core.AdvancePreview], error) {
		preview, err := handlers.core.PreviewAdvance(requestContext, viewerFrom(requestContext), core.AdvancePreviewInput{Days: input.Days})
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.AdvancePreview]{Body: preview}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "previewAdjustStock",
		Method:      http.MethodPost,
		Path:        "/api/colony/adjust-preview",
		Summary:     "What an adjustment or cargo would apply, caps included, without doing it",
		Tags:        []string{"colony"},
	}, func(requestContext context.Context, input *bodyInput[core.AdjustInput]) (*bodyOutput[core.AdjustPreview], error) {
		preview, err := handlers.core.PreviewAdjustStock(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.AdjustPreview]{Body: preview}, nil
	})

	registerColonyChange(handlers, api, "advanceDays", "/api/colony/advance", "Advance the colony N days", (*core.Service).Advance)
	registerColonyChange(handlers, api, "adjustStock", "/api/colony/adjust", "Adjust stock, or record a shipment's cargo", (*core.Service).AdjustStock)
	registerColonyChange(handlers, api, "clearShipmentSchedule", "/api/colony/shipments/clear", "Clear the shipment schedule", (*core.Service).ClearShipmentSchedule)
	registerColonyChange(handlers, api, "correctDay", "/api/colony/correct-day", "Set the day directly, with no upkeep or arrivals", (*core.Service).CorrectDay)
	registerColonyChange(handlers, api, "undoLastChange", "/api/colony/undo", "Undo the last colony change", (*core.Service).UndoLastChange)

	Register(api, AccessDm, huma.Operation{
		OperationID: "setShipmentSchedule",
		Method:      http.MethodPut,
		Path:        "/api/colony/shipments",
		Summary:     "Set the shipment interval and next arrival day",
		Tags:        []string{"colony"},
	}, func(requestContext context.Context, input *bodyInput[core.ShipmentScheduleInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.SetShipmentSchedule(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})
}

// registerColonyChange registers one POST colony mutation that answers with a ColonyChange.
func registerColonyChange[Input any](handlers *handlers, api huma.API, operationID, path, summary string,
	run func(*core.Service, context.Context, core.Viewer, Input) (core.ColonyChange, error),
) {
	Register(api, AccessDm, huma.Operation{
		OperationID: operationID,
		Method:      http.MethodPost,
		Path:        path,
		Summary:     summary,
		Tags:        []string{"colony"},
	}, func(requestContext context.Context, input *bodyInput[Input]) (*bodyOutput[core.ColonyChange], error) {
		change, err := run(handlers.core, requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})
}
