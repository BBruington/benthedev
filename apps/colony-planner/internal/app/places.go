package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

type placeListQuery struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	Search     string    `query:"search" doc:"Names containing this text"`
}

func (handlers *handlers) registerPlaces(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listPlaces",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/places",
		Summary:     "Places, optionally searched by name, in the viewer's projection",
		Tags:        []string{"places"},
	}, func(requestContext context.Context, input *placeListQuery) (*bodyOutput[core.PlaceList], error) {
		places, err := handlers.core.ListPlaces(requestContext, viewerFrom(requestContext), input.Search)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PlaceList]{Body: places}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createPlace",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/places",
		Summary:       "Add a place",
		Tags:          []string{"places"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.PlaceInput]) (*bodyOutput[core.Place], error) {
		place, err := handlers.core.CreatePlace(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Place]{Body: place}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getPlace",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/places/{placeId}",
		Summary:     "One place with the NPCs who are there; a hidden one is not found for players",
		Tags:        []string{"places"},
	}, func(requestContext context.Context, input *placePath) (*bodyOutput[core.Place], error) {
		place, err := handlers.core.GetPlace(requestContext, viewerFrom(requestContext), input.PlaceID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Place]{Body: place}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updatePlace",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/places/{placeId}",
		Summary:     "Replace a place's record",
		Tags:        []string{"places"},
	}, func(requestContext context.Context, input *placeBodyInput[core.PlaceInput]) (*bodyOutput[core.Place], error) {
		place, err := handlers.core.UpdatePlace(requestContext, viewerFrom(requestContext), input.PlaceID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Place]{Body: place}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setPlaceRevealed",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/places/{placeId}/revealed",
		Summary:     "Reveal a place to the players, or hide it again",
		Tags:        []string{"places"},
	}, func(requestContext context.Context, input *placeBodyInput[RevealInput]) (*bodyOutput[core.Place], error) {
		place, err := handlers.core.SetPlaceRevealed(requestContext, viewerFrom(requestContext), input.PlaceID, input.Body.Revealed)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Place]{Body: place}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "deletePlace",
		Method:        http.MethodDelete,
		Path:          "/api/campaigns/{campaignId}/places/{placeId}",
		Summary:       "Delete a place; NPCs there or from there keep no place",
		Tags:          []string{"places"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *placePath) (*struct{}, error) {
		if err := handlers.core.DeletePlace(requestContext, viewerFrom(requestContext), input.PlaceID); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})
}
