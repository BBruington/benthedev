package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerSettings(api huma.API) {
	Register(api, AccessDm, huma.Operation{
		OperationID: "getSettings",
		Method:      http.MethodGet,
		Path:        "/api/settings",
		Summary:     "Campaign settings",
		Tags:        []string{"settings"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.Settings], error) {
		settings, err := handlers.core.GetSettings(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Settings]{Body: settings}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateSettings",
		Method:      http.MethodPut,
		Path:        "/api/settings",
		Summary:     "Replace the campaign settings",
		Tags:        []string{"settings"},
	}, func(requestContext context.Context, input *bodyInput[core.SettingsInput]) (*bodyOutput[core.Settings], error) {
		settings, err := handlers.core.UpdateSettings(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Settings]{Body: settings}, nil
	})
}
