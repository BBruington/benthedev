package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// requestListQuery narrows the request list; every empty value is off. huma takes no pointer query
// parameters, so an absent patron is the nil UUID and an absent status is blank.
type requestListQuery struct {
	CampaignID uuid.UUID          `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	Status     core.RequestStatus `query:"status"`
	Mine       bool               `query:"mine" doc:"Only the viewer's own requests"`
	PatronID   uuid.UUID          `query:"patronId" format:"uuid" doc:"Only this patron's requests"`
}

func (query *requestListQuery) filter() core.RequestFilter {
	filter := core.RequestFilter{Mine: query.Mine}
	if query.Status != "" {
		status := query.Status
		filter.Status = &status
	}
	if query.PatronID != uuid.Nil {
		patronID := query.PatronID
		filter.PatronID = &patronID
	}
	return filter
}

func (handlers *handlers) registerRequests(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listRequests",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/requests",
		Summary:     "The DM's approval queue and history, or the party's history; pending first, then newest",
		Tags:        []string{"requests"},
	}, func(requestContext context.Context, input *requestListQuery) (*bodyOutput[core.RequestList], error) {
		requests, err := handlers.core.ListRequests(requestContext, viewerFrom(requestContext), input.filter())
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.RequestList]{Body: requests}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID:   "createRequest",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/requests",
		Summary:       "Ask a patron for an action; it must be affordable now",
		Tags:          []string{"requests"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.RequestInput]) (*bodyOutput[core.Request], error) {
		request, err := handlers.core.CreateRequest(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Request]{Body: request}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "useActionNow",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/requests/use-action",
		Summary:     "Use a patron action now, optionally on behalf of a member: a request made and granted at once",
		Tags:        []string{"requests"},
	}, func(requestContext context.Context, input *campaignBodyInput[core.UseActionInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.UseActionNow(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getRequest",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/requests/{requestId}",
		Summary:     "One request; one for a hidden patron or action is not found for players",
		Tags:        []string{"requests"},
	}, func(requestContext context.Context, input *requestPath) (*bodyOutput[core.Request], error) {
		request, err := handlers.core.GetRequest(requestContext, viewerFrom(requestContext), input.RequestID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Request]{Body: request}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "withdrawRequest",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/requests/{requestId}/withdraw",
		Summary:     "Withdraw one of your own pending requests",
		Tags:        []string{"requests"},
	}, func(requestContext context.Context, input *requestPath) (*bodyOutput[core.Request], error) {
		request, err := handlers.core.WithdrawRequest(requestContext, viewerFrom(requestContext), input.RequestID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Request]{Body: request}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "previewApproveRequest",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/requests/{requestId}/approve-preview",
		Summary:     "What granting a request would apply and whether it is affordable, without doing it",
		Tags:        []string{"requests"},
	}, func(requestContext context.Context, input *requestBodyInput[core.ApproveRequestInput]) (*bodyOutput[core.RequestApprovalPreview], error) {
		preview, err := handlers.core.PreviewApproveRequest(requestContext, viewerFrom(requestContext), input.RequestID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.RequestApprovalPreview]{Body: preview}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "approveRequest",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/requests/{requestId}/approve",
		Summary:     "Grant a pending request, optionally tweaked for this grant, with an optional note to the requester",
		Tags:        []string{"requests"},
	}, func(requestContext context.Context, input *requestBodyInput[core.ApproveRequestInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.ApproveRequest(requestContext, viewerFrom(requestContext), input.RequestID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "declineRequest",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/requests/{requestId}/decline",
		Summary:     "Decline a pending request with an optional note only the requester reads",
		Tags:        []string{"requests"},
	}, func(requestContext context.Context, input *requestBodyInput[core.DeclineRequestInput]) (*bodyOutput[core.Request], error) {
		request, err := handlers.core.DeclineRequest(requestContext, viewerFrom(requestContext), input.RequestID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Request]{Body: request}, nil
	})
}
