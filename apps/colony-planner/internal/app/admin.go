package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// registerAdmin holds the site owner's routes, all under /api/admin/. They read directory tables only
// (core checks IsSiteOwner again); a campaign's content is never reachable from here.
func (handlers *handlers) registerAdmin(api huma.API) {
	Register(api, AccessOwner, huma.Operation{
		OperationID: "listCampaignRequests",
		Method:      http.MethodGet,
		Path:        "/api/admin/campaign-requests",
		Summary:     "Open requests to run a campaign",
		Tags:        []string{"admin"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.AdminCampaignRequestList], error) {
		requests, err := handlers.core.ListCampaignRequests(requestContext, callerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.AdminCampaignRequestList]{Body: requests}, nil
	})

	Register(api, AccessOwner, huma.Operation{
		OperationID: "approveCampaignRequest",
		Method:      http.MethodPost,
		Path:        "/api/admin/campaign-requests/{campaignRequestId}/approve",
		Summary:     "Create the campaign with the requester as its DM; the address may be changed here, then never",
		Tags:        []string{"admin"},
	}, func(requestContext context.Context, input *campaignRequestBodyInput[core.ApproveCampaignRequestInput]) (*bodyOutput[core.AdminCampaign], error) {
		campaign, err := handlers.core.ApproveCampaignRequest(requestContext, callerFrom(requestContext), input.CampaignRequestID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.AdminCampaign]{Body: campaign}, nil
	})

	Register(api, AccessOwner, huma.Operation{
		OperationID: "denyCampaignRequest",
		Method:      http.MethodPost,
		Path:        "/api/admin/campaign-requests/{campaignRequestId}/deny",
		Summary:     "Say no to a request; the requester may ask again",
		Tags:        []string{"admin"},
	}, func(requestContext context.Context, input *campaignRequestPath) (*bodyOutput[core.CampaignRequestView], error) {
		request, err := handlers.core.DenyCampaignRequest(requestContext, callerFrom(requestContext), input.CampaignRequestID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.CampaignRequestView]{Body: request}, nil
	})

	Register(api, AccessOwner, huma.Operation{
		OperationID: "listCampaigns",
		Method:      http.MethodGet,
		Path:        "/api/admin/campaigns",
		Summary:     "Every campaign's directory data: name, address, DM, counts, Discord linked, status",
		Tags:        []string{"admin"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.AdminCampaignList], error) {
		campaigns, err := handlers.core.ListCampaigns(requestContext, callerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.AdminCampaignList]{Body: campaigns}, nil
	})

	campaignActions := []struct {
		operationID string
		action      string
		summary     string
		run         func(*core.Service, context.Context, core.Caller, uuid.UUID) (core.AdminCampaign, error)
	}{
		{"suspendCampaign", "suspend", "Close the campaign to everyone, data intact", (*core.Service).SuspendCampaign},
		{"unsuspendCampaign", "unsuspend", "Open a suspended campaign again", (*core.Service).UnsuspendCampaign},
	}
	for _, campaignAction := range campaignActions {
		Register(api, AccessOwner, huma.Operation{
			OperationID: campaignAction.operationID,
			Method:      http.MethodPost,
			Path:        "/api/admin/campaigns/{campaignId}/" + campaignAction.action,
			Summary:     campaignAction.summary,
			Tags:        []string{"admin"},
		}, func(requestContext context.Context, input *adminCampaignPath) (*bodyOutput[core.AdminCampaign], error) {
			campaign, err := campaignAction.run(handlers.core, requestContext, callerFrom(requestContext), input.CampaignID)
			if err != nil {
				return nil, handlers.fail(requestContext, err)
			}
			return &bodyOutput[core.AdminCampaign]{Body: campaign}, nil
		})
	}

	Register(api, AccessOwner, huma.Operation{
		OperationID:   "deleteCampaign",
		Method:        http.MethodPost,
		Path:          "/api/admin/campaigns/{campaignId}/delete",
		Summary:       "Delete a suspended campaign and everything in it, after typing its slug",
		Tags:          []string{"admin"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *adminCampaignBodyInput[core.DeleteCampaignInput]) (*struct{}, error) {
		if err := handlers.core.DeleteCampaign(requestContext, callerFrom(requestContext), input.CampaignID, input.Body); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})
}
