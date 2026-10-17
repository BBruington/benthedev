package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerMaps(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listMaps",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/maps",
		Summary:     "The campaign's image maps in the DM's order; players get revealed ones only",
		Tags:        []string{"maps"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.WorldMapList], error) {
		maps, err := handlers.core.ListMaps(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.WorldMapList]{Body: maps}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createMap",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/maps",
		Summary:       "Add a map with its art (an uploaded image of this campaign or an https address)",
		Tags:          []string{"maps"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.NewMapInput]) (*bodyOutput[core.WorldMap], error) {
		worldMap, err := handlers.core.CreateMap(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.WorldMap]{Body: worldMap}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "reorderMaps",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/maps/order",
		Summary:     "Put every map of the campaign in a new order",
		Tags:        []string{"maps"},
	}, func(requestContext context.Context, input *campaignBodyInput[core.MapOrderInput]) (*bodyOutput[core.WorldMapList], error) {
		maps, err := handlers.core.ReorderMaps(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.WorldMapList]{Body: maps}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getMap",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/maps/{mapId}",
		Summary:     "One map with its pins; a hidden map is not found for players, who see revealed places' pins only",
		Tags:        []string{"maps"},
	}, func(requestContext context.Context, input *mapPath) (*bodyOutput[core.WorldMap], error) {
		worldMap, err := handlers.core.GetMap(requestContext, viewerFrom(requestContext), input.MapID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.WorldMap]{Body: worldMap}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateMap",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/maps/{mapId}",
		Summary:     "Rename a map (art, visibility, Mainland and order each have their own operation)",
		Tags:        []string{"maps"},
	}, func(requestContext context.Context, input *mapBodyInput[core.MapInput]) (*bodyOutput[core.WorldMap], error) {
		worldMap, err := handlers.core.RenameMap(requestContext, viewerFrom(requestContext), input.MapID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.WorldMap]{Body: worldMap}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "replaceMapArt",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/maps/{mapId}/art",
		Summary:     "Replace a map's art; pins keep their place on the new image",
		Tags:        []string{"maps"},
	}, func(requestContext context.Context, input *mapBodyInput[core.MapArtInput]) (*bodyOutput[core.WorldMap], error) {
		worldMap, err := handlers.core.ReplaceMapArt(requestContext, viewerFrom(requestContext), input.MapID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.WorldMap]{Body: worldMap}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setMapRevealed",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/maps/{mapId}/revealed",
		Summary:     "Reveal a map to the players, or hide it again",
		Tags:        []string{"maps"},
	}, func(requestContext context.Context, input *mapBodyInput[RevealInput]) (*bodyOutput[core.WorldMap], error) {
		worldMap, err := handlers.core.SetMapRevealed(requestContext, viewerFrom(requestContext), input.MapID, input.Body.Revealed)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.WorldMap]{Body: worldMap}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setMapMainland",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/maps/{mapId}/mainland",
		Summary:     "Make a map the campaign's Mainland, or unmark it",
		Tags:        []string{"maps"},
	}, func(requestContext context.Context, input *mapBodyInput[MainlandInput]) (*bodyOutput[core.WorldMap], error) {
		worldMap, err := handlers.core.SetMapMainland(requestContext, viewerFrom(requestContext), input.MapID, input.Body.Mainland)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.WorldMap]{Body: worldMap}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "deleteMap",
		Method:        http.MethodDelete,
		Path:          "/api/campaigns/{campaignId}/maps/{mapId}",
		Summary:       "Delete a map; its places stay, unpinned",
		Tags:          []string{"maps"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *mapPath) (*struct{}, error) {
		if err := handlers.core.DeleteMap(requestContext, viewerFrom(requestContext), input.MapID); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setPlacePin",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/places/{placeId}/pin",
		Summary:     "Pin a place on a map, or move its pin",
		Tags:        []string{"places"},
	}, func(requestContext context.Context, input *placeBodyInput[core.PinInput]) (*bodyOutput[core.Place], error) {
		place, err := handlers.core.SetPlacePin(requestContext, viewerFrom(requestContext), input.PlaceID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Place]{Body: place}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "clearPlacePin",
		Method:      http.MethodDelete,
		Path:        "/api/campaigns/{campaignId}/places/{placeId}/pin",
		Summary:     "Take a place's pin off its map",
		Tags:        []string{"places"},
	}, func(requestContext context.Context, input *placePath) (*bodyOutput[core.Place], error) {
		place, err := handlers.core.ClearPlacePin(requestContext, viewerFrom(requestContext), input.PlaceID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Place]{Body: place}, nil
	})
}
