package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerMe(api huma.API) {
	Register(api, AccessPending, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/me",
		Summary:     "The signed-in member, their status and whether they are the DM",
		Tags:        []string{"me"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.Me], error) {
		me, err := handlers.core.Me(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Me]{Body: me}, nil
	})

	// Member level, not dm: the DM must be able to turn view as player off while it is on. Core
	// checks IsDM.
	Register(api, AccessMember, huma.Operation{
		OperationID:   "setViewAsPlayer",
		Method:        http.MethodPost,
		Path:          "/api/me/view-as-player",
		Summary:       "Turn view as player on or off (DM only); then read /api/me again",
		Tags:          []string{"me"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *bodyInput[core.ViewAsPlayerInput]) (*struct{}, error) {
		if err := handlers.core.SetViewAsPlayer(requestContext, viewerFrom(requestContext), input.Body); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "updateProfile",
		Method:      http.MethodPut,
		Path:        "/api/me/profile",
		Summary:     "Replace the caller's profile (character name)",
		Tags:        []string{"me"},
	}, func(requestContext context.Context, input *bodyInput[core.ProfileInput]) (*bodyOutput[core.Me], error) {
		me, err := handlers.core.UpdateProfile(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Me]{Body: me}, nil
	})
}
