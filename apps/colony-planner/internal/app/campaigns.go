package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

// registerAccount holds the routes outside every campaign: who is signed in, their campaigns, invite
// links and their own request to run a campaign. They take the Caller; their DTOs carry no `dm` object
// and no invite code.
func (handlers *handlers) registerAccount(api huma.API) {
	Register(api, AccessAccount, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/me",
		Summary:     "The signed-in account, before any campaign",
		Tags:        []string{"account"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.AccountMe], error) {
		me, err := handlers.core.AccountMe(requestContext, callerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.AccountMe]{Body: me}, nil
	})

	Register(api, AccessAccount, huma.Operation{
		OperationID: "listMyCampaigns",
		Method:      http.MethodGet,
		Path:        "/api/campaigns",
		Summary:     "Every campaign the caller has a membership in, whatever its status (the picker; slug to ID)",
		Tags:        []string{"account"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.MyCampaigns], error) {
		campaigns, err := handlers.core.MyCampaigns(requestContext, callerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MyCampaigns]{Body: campaigns}, nil
	})

	Register(api, AccessAccount, huma.Operation{
		OperationID: "getInvite",
		Method:      http.MethodGet,
		Path:        "/api/invites/{inviteCode}",
		Summary:     "Who runs the invite's campaign, its status, and the caller's membership there",
		Tags:        []string{"account"},
	}, func(requestContext context.Context, input *invitePath) (*bodyOutput[core.InviteInfo], error) {
		info, err := handlers.core.InviteInfo(requestContext, callerFrom(requestContext), input.InviteCode)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.InviteInfo]{Body: info}, nil
	})

	Register(api, AccessAccount, huma.Operation{
		OperationID: "joinByInvite",
		Method:      http.MethodPost,
		Path:        "/api/invites/{inviteCode}/join",
		Summary:     "Ask to join the invite's campaign; an existing membership comes back unchanged",
		Tags:        []string{"account"},
	}, func(requestContext context.Context, input *invitePath) (*bodyOutput[core.MyCampaign], error) {
		joined, err := handlers.core.JoinByInvite(requestContext, callerFrom(requestContext), input.InviteCode)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MyCampaign]{Body: joined}, nil
	})

	Register(api, AccessAccount, huma.Operation{
		OperationID:   "requestCampaign",
		Method:        http.MethodPost,
		Path:          "/api/campaign-requests",
		Summary:       "Ask the site owner to run a campaign (one open request at a time)",
		Tags:          []string{"account"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *bodyInput[core.CampaignRequestInput]) (*bodyOutput[core.CampaignRequestView], error) {
		request, err := handlers.core.RequestCampaign(requestContext, callerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.CampaignRequestView]{Body: request}, nil
	})

	Register(api, AccessAccount, huma.Operation{
		OperationID: "getMyCampaignRequest",
		Method:      http.MethodGet,
		Path:        "/api/campaign-requests/mine",
		Summary:     "The caller's newest request to run a campaign, or null",
		Tags:        []string{"account"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.MyCampaignRequest], error) {
		request, err := handlers.core.MyCampaignRequest(requestContext, callerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MyCampaignRequest]{Body: request}, nil
	})

	Register(api, AccessAccount, huma.Operation{
		OperationID: "withdrawCampaignRequest",
		Method:      http.MethodPost,
		Path:        "/api/campaign-requests/{campaignRequestId}/withdraw",
		Summary:     "Withdraw the caller's own open request",
		Tags:        []string{"account"},
	}, func(requestContext context.Context, input *campaignRequestPath) (*bodyOutput[core.CampaignRequestView], error) {
		request, err := handlers.core.WithdrawCampaignRequest(requestContext, callerFrom(requestContext), input.CampaignRequestID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.CampaignRequestView]{Body: request}, nil
	})
}

// registerCampaign holds the DM's campaign-wide operations: the invite link and deleting the campaign.
func (handlers *handlers) registerCampaign(api huma.API) {
	Register(api, AccessDm, huma.Operation{
		OperationID: "getInviteLink",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/invite",
		Summary:     "The campaign's invite code, for /join/<code> (the only read that returns it)",
		Tags:        []string{"campaign"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.InviteLink], error) {
		invite, err := handlers.core.Invite(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.InviteLink]{Body: invite}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "resetInviteLink",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/invite/reset",
		Summary:     "Replace the invite code; the old link stops working at once",
		Tags:        []string{"campaign"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.InviteLink], error) {
		invite, err := handlers.core.ResetInviteLink(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.InviteLink]{Body: invite}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "deleteOwnCampaign",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/delete",
		Summary:       "Delete this campaign and everything in it, after typing its slug; accounts stay",
		Tags:          []string{"campaign"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *campaignBodyInput[core.DeleteCampaignInput]) (*struct{}, error) {
		if err := handlers.core.DeleteOwnCampaign(requestContext, viewerFrom(requestContext), input.Body); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})
}
