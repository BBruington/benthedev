package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

// registerMe holds the caller's own place in one campaign. GET .../me and POST .../leave are two of the
// three pending-level routes: they answer any membership, whatever its status, even while the
// campaign is suspended, so the SPA can show the waiting, no-access and paused screens.
func (handlers *handlers) registerMe(api huma.API) {
	Register(api, AccessPending, huma.Operation{
		OperationID: "getCampaignMe",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/me",
		Summary:     "The caller's membership in this campaign: status, whether they are its DM, and the campaign's status",
		Tags:        []string{"me"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.CampaignMe], error) {
		me, err := handlers.core.CampaignMe(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.CampaignMe]{Body: me}, nil
	})

	Register(api, AccessPending, huma.Operation{
		OperationID: "leaveCampaign",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/leave",
		Summary:     "Leave this campaign (players only, from waiting or active); only its DM can let you back in",
		Tags:        []string{"me"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.CampaignMe], error) {
		me, err := handlers.core.LeaveCampaign(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.CampaignMe]{Body: me}, nil
	})

	// Member level, not dm: the DM must be able to turn view as player off while it is on. Core
	// checks IsDM. The toggle belongs to the session, so it applies in every campaign the caller DMs.
	Register(api, AccessMember, huma.Operation{
		OperationID:   "setViewAsPlayer",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/me/view-as-player",
		Summary:       "Turn view as player on or off (this campaign's DM only); then read .../me again",
		Tags:          []string{"me"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *campaignBodyInput[core.ViewAsPlayerInput]) (*struct{}, error) {
		if err := handlers.core.SetViewAsPlayer(requestContext, viewerFrom(requestContext), input.Body); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "updateProfile",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/me/profile",
		Summary:     "Replace the caller's profile in this campaign (character name)",
		Tags:        []string{"me"},
	}, func(requestContext context.Context, input *campaignBodyInput[core.ProfileInput]) (*bodyOutput[core.CampaignMe], error) {
		me, err := handlers.core.UpdateProfile(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.CampaignMe]{Body: me}, nil
	})
}
