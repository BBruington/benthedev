package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

// The hex map is the whole party's tool: any active member paints, labels and erases (spec §5.10).
func (handlers *handlers) registerHexes(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listHexes",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/hexes",
		Summary:     "Every charted hex of the campaign",
		Tags:        []string{"hexes"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.HexList], error) {
		hexes, err := handlers.core.ListHexes(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.HexList]{Body: hexes}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID:   "paintHexes",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/hexes/paint",
		Summary:       "Paint one batch of a stroke; painted hexes keep their labels",
		Tags:          []string{"hexes"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *campaignBodyInput[core.PaintHexesInput]) (*struct{}, error) {
		if err := handlers.core.PaintHexes(requestContext, viewerFrom(requestContext), input.Body); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID:   "eraseHexes",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/hexes/erase",
		Summary:       "Unchart hexes, terrain and label together",
		Tags:          []string{"hexes"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *campaignBodyInput[core.EraseHexesInput]) (*struct{}, error) {
		if err := handlers.core.EraseHexes(requestContext, viewerFrom(requestContext), input.Body); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID:   "setHexLabel",
		Method:        http.MethodPut,
		Path:          "/api/campaigns/{campaignId}/hexes/{q}/{r}/label",
		Summary:       "Label a charted hex, or clear its label with a blank one",
		Tags:          []string{"hexes"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *hexLabelBodyInput) (*struct{}, error) {
		coordinate := core.HexCoordinate{Q: input.Q, R: input.R}
		if err := handlers.core.SetHexLabel(requestContext, viewerFrom(requestContext), coordinate, input.Body); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})
}
