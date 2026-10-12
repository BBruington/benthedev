package core

import (
	"context"

	"github.com/google/uuid"
)

// Notifier is the single seam between core and Discord; Beny Bot (internal/bot) implements it. Core calls
// it after the transaction commits. Delivery is best-effort: implementations log failures and never
// block or fail the operation (spec §7.2, "No outbox"). Logs carry IDs and kinds, never the names in
// these messages.
type Notifier interface {
	// MemberWaiting announces a new PENDING membership created through an invite link. Core calls it
	// only for brand-new memberships (never for an existing one opening the link again, never past the
	// pending cap). The implementation applies the per-campaign limit (WaitingPingsPerHour) and posts
	// to that campaign's DM channel in its linked server, skipping quietly when there is none.
	MemberWaiting(requestContext context.Context, member WaitingMember)
	// CampaignRequested tells the site owner someone asks to run a campaign (OwnerRequestPingsPerHour).
	CampaignRequested(requestContext context.Context, request RequestedCampaign)
}

type WaitingMember struct {
	CampaignID      uuid.UUID
	CampaignSlug    string
	MemberID        uuid.UUID
	DisplayName     string
	DiscordUsername string
}

type RequestedCampaign struct {
	CampaignRequestID uuid.UUID
	CampaignName      string
	DisplayName       string
	DiscordUsername   string
}

// NoopNotifier is used when Beny Bot is off (no Discord variables) and in tests.
type NoopNotifier struct{}

func (NoopNotifier) MemberWaiting(context.Context, WaitingMember) {}

func (NoopNotifier) CampaignRequested(context.Context, RequestedCampaign) {}
