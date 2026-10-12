package app

import (
	"github.com/google/uuid"
)

// bodyOutput and bodyInput wrap a core DTO as a huma response or request body. huma names the schema
// after the DTO, so the wrappers never appear in the generated TypeScript.
type bodyOutput[Body any] struct {
	Body Body
}

type bodyInput[Body any] struct {
	Body Body
}

// campaignPath is the {campaignId} every campaign route carries. Handlers never read it: the access
// gate has already resolved it to the caller's Viewer (an unparsable or unknown ID never gets here).
type campaignPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
}

type campaignBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	Body       Body
}

type memberPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	MemberID   uuid.UUID `path:"memberId" format:"uuid"`
}

type memberBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	MemberID   uuid.UUID `path:"memberId" format:"uuid"`
	Body       Body
}

type timelineEntryPath struct {
	CampaignID      uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	TimelineEntryID uuid.UUID `path:"timelineEntryId" format:"uuid"`
}

type timelineEntryBodyInput[Body any] struct {
	CampaignID      uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	TimelineEntryID uuid.UUID `path:"timelineEntryId" format:"uuid"`
	Body            Body
}

type resourceBodyInput[Body any] struct {
	CampaignID  uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	ResourceKey string    `path:"resourceKey"`
	Body        Body
}

// Account and owner routes name directory rows; the owner's {campaignId} is not a membership.
type invitePath struct {
	InviteCode string `path:"inviteCode" doc:"The code from an invite link /join/<code>"`
}

type campaignRequestPath struct {
	CampaignRequestID uuid.UUID `path:"campaignRequestId" format:"uuid"`
}

type campaignRequestBodyInput[Body any] struct {
	CampaignRequestID uuid.UUID `path:"campaignRequestId" format:"uuid"`
	Body              Body
}

type adminCampaignPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid"`
}

type adminCampaignBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid"`
	Body       Body
}
