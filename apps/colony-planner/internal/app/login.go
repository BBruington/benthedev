package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerLogin(api huma.API) {
	Register(api, AccessPublic, huma.Operation{
		OperationID: "getLoginOptions",
		Method:      http.MethodGet,
		Path:        "/api/login-options",
		Summary:     "Sign-in methods for /login",
		Tags:        []string{"auth"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.LoginOptions], error) {
		options, err := handlers.core.LoginOptions(requestContext, core.LoginOptionsInput{
			DiscordEnabled: handlers.auth.DiscordEnabled(),
			DevSignIn:      auth.DevAuthEnabled,
		})
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.LoginOptions]{Body: options}, nil
	})
}
