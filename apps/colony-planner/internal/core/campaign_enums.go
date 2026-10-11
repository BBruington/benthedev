package core

import "github.com/BBruington/colony-planner/internal/store"

// CampaignRole is the Postgres campaign_role enum: one DM per campaign, everyone else a player.
type CampaignRole = store.CampaignRole

const (
	CampaignRoleDm     = store.CampaignRoleDM
	CampaignRolePlayer = store.CampaignRolePLAYER
)

func AllCampaignRoleValues() []CampaignRole {
	return store.AllCampaignRoleValues()
}

// CampaignStatus is the Postgres campaign_status enum: SUSPENDED closes the campaign to everyone.
type CampaignStatus = store.CampaignStatus

const (
	CampaignStatusActive    = store.CampaignStatusACTIVE
	CampaignStatusSuspended = store.CampaignStatusSUSPENDED
)

func AllCampaignStatusValues() []CampaignStatus {
	return store.AllCampaignStatusValues()
}

// CampaignRequestStatus is the Postgres campaign_request_status enum.
type CampaignRequestStatus = store.CampaignRequestStatus

const (
	CampaignRequestStatusPending   = store.CampaignRequestStatusPENDING
	CampaignRequestStatusApproved  = store.CampaignRequestStatusAPPROVED
	CampaignRequestStatusDenied    = store.CampaignRequestStatusDENIED
	CampaignRequestStatusWithdrawn = store.CampaignRequestStatusWITHDRAWN
)

func AllCampaignRequestStatusValues() []CampaignRequestStatus {
	return store.AllCampaignRequestStatusValues()
}
