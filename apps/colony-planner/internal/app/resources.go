package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerResources(api huma.API) {
	Register(api, AccessDm, huma.Operation{
		OperationID: "listResources",
		Method:      http.MethodGet,
		Path:        "/api/resources",
		Summary:     "Every resource, archived ones included, for settings",
		Tags:        []string{"resources"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.ResourceList], error) {
		resources, err := handlers.core.ListResources(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ResourceList]{Body: resources}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createResource",
		Method:        http.MethodPost,
		Path:          "/api/resources",
		Summary:       "Add a resource",
		Tags:          []string{"resources"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *bodyInput[core.NewResourceInput]) (*bodyOutput[core.ResourceSettings], error) {
		resource, err := handlers.core.CreateResource(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ResourceSettings]{Body: resource}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateResource",
		Method:      http.MethodPut,
		Path:        "/api/resources/{resourceKey}",
		Summary:     "Replace a resource's label, icon, bounds, order and archived flag",
		Tags:        []string{"resources"},
	}, func(requestContext context.Context, input *resourceBodyInput[core.ResourceInput]) (*bodyOutput[core.ResourceSettings], error) {
		resource, err := handlers.core.UpdateResource(requestContext, viewerFrom(requestContext), input.ResourceKey, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ResourceSettings]{Body: resource}, nil
	})
}
