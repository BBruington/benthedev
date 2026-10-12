package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerPeople(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "getWorkforce",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/people",
		Summary:     "Every role with its headcounts and named colonists, in the viewer's projection",
		Tags:        []string{"people"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.Workforce], error) {
		workforce, err := handlers.core.GetWorkforce(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Workforce]{Body: workforce}, nil
	})

	registerColonyChange(handlers, api, "transferColonists", "/api/campaigns/{campaignId}/people/transfer",
		"Move unnamed colonists from one role to another", (*core.Service).TransferColonists)
	registerColonyChange(handlers, api, "setRoleCount", "/api/campaigns/{campaignId}/people/set-count",
		"Set a role's unnamed headcount directly", (*core.Service).SetRoleCount)

	Register(api, AccessDm, huma.Operation{
		OperationID: "listRoles",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/roles",
		Summary:     "Every role, archived ones included, for settings",
		Tags:        []string{"people"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.RoleList], error) {
		roles, err := handlers.core.ListRoles(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.RoleList]{Body: roles}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createRole",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/roles",
		Summary:       "Add a role",
		Tags:          []string{"people"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.NewRoleInput]) (*bodyOutput[core.RoleSettings], error) {
		role, err := handlers.core.CreateRole(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.RoleSettings]{Body: role}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateRole",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/roles/{roleKey}",
		Summary:     "Replace a role's label, icon, order and archived flag",
		Tags:        []string{"people"},
	}, func(requestContext context.Context, input *roleBodyInput[core.RoleInput]) (*bodyOutput[core.RoleSettings], error) {
		role, err := handlers.core.UpdateRole(requestContext, viewerFrom(requestContext), input.RoleKey, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.RoleSettings]{Body: role}, nil
	})
}
