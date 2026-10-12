package app

import (
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
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

type npcPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	NpcID      uuid.UUID `path:"npcId" format:"uuid"`
}

type npcBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	NpcID      uuid.UUID `path:"npcId" format:"uuid"`
	Body       Body
}

type placePath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	PlaceID    uuid.UUID `path:"placeId" format:"uuid"`
}

type placeBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	PlaceID    uuid.UUID `path:"placeId" format:"uuid"`
	Body       Body
}

type roleBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	RoleKey    string    `path:"roleKey"`
	Body       Body
}

// RevealInput is the body of the reveal switches on NPCs, places, patrons and patron actions.
type RevealInput struct {
	Revealed bool `json:"revealed"`
}

type questPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	QuestID    uuid.UUID `path:"questId" format:"uuid"`
}

type questBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	QuestID    uuid.UUID `path:"questId" format:"uuid"`
	Body       Body
}

type patronPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	PatronID   uuid.UUID `path:"patronId" format:"uuid"`
}

type patronBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	PatronID   uuid.UUID `path:"patronId" format:"uuid"`
	Body       Body
}

type patronActionBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	ActionID   uuid.UUID `path:"actionId" format:"uuid"`
	Body       Body
}

type requestPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	RequestID  uuid.UUID `path:"requestId" format:"uuid"`
}

type requestBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	RequestID  uuid.UUID `path:"requestId" format:"uuid"`
	Body       Body
}

// ArchiveInput is the body of the archive switches on patrons and their actions.
type ArchiveInput struct {
	Archived bool `json:"archived"`
}

type mapPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	MapID      uuid.UUID `path:"mapId" format:"uuid"`
}

type mapBodyInput[Body any] struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	MapID      uuid.UUID `path:"mapId" format:"uuid"`
	Body       Body
}

// MainlandInput is the body of a map's Mainland switch.
type MainlandInput struct {
	Mainland bool `json:"mainland" doc:"true makes this map the Mainland, taking the flag from any other map"`
}

type hexLabelBodyInput struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	Q          int64     `path:"q" doc:"The hex's axial q coordinate"`
	R          int64     `path:"r" doc:"The hex's axial r coordinate"`
	Body       core.HexLabelInput
}
