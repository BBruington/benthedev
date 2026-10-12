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

// RequestNotifier is Beny Bot's side of patron requests (spec §7.5): a second seam beside Notifier, so
// the bot can take it on in its own stage. Core calls it after the transaction commits; delivery is
// best-effort and never fails the operation. Implementations post through postToCampaign, which
// checks the channel belongs to the campaign's linked server, and log only IDs and statuses.
type RequestNotifier interface {
	// RequestCreated asks the DM to decide: the implementation posts prompt.Message with Grant and
	// Decline buttons (custom IDs req:{requestId}:grant and req:{requestId}:decline) to the campaign's
	// DM channel, then stores the message with prompt.RecordMessage.
	RequestCreated(requestContext context.Context, prompt RequestPrompt)
	// RequestDecided edits a posted prompt once the request is granted, declined, withdrawn or undone,
	// whether that happened on the website or in Discord. Core calls it only when the request has a
	// stored Discord message.
	RequestDecided(requestContext context.Context, outcome RequestOutcome)
}

// RequestPrompt is a new request for the DM's channel, which the bot words as "Thorin requests **Grain
// Barge** from Lady Vex (3 favor → Food +7). Favor now 5." It comes in parts because players choose
// their character names, so only the bot can escape them for Discord. It comes from the DM's
// projection, because only the DM channel receives it.
type RequestPrompt struct {
	CampaignID    uuid.UUID
	RequestID     uuid.UUID
	RequesterName string
	Title         string
	PatronName    string
	// Terms is the cost and what it buys: "3 favor → Food +7".
	Terms    string
	FavorNow string
	// RecordMessage stores the posted message's ID on this request (and only this one), so later
	// decisions can edit it. A decision made while the prompt was being posted had no message to edit,
	// so then it returns that outcome for the implementation to show; otherwise nil.
	RecordMessage func(requestContext context.Context, discordMessageID string) (*RequestOutcome, error)
}

// RequestOutcome is what a prompt's message becomes: "Granted · Food +7 · favor 5 → 2", "Declined",
// "Withdrawn" or "Undone". An undone grant waits again, so its buttons come back (Undone is true).
type RequestOutcome struct {
	CampaignID       uuid.UUID
	RequestID        uuid.UUID
	DiscordMessageID string
	Status           RequestStatus
	Text             string
	Undone           bool
}

// NoopRequestNotifier is used while Beny Bot is off and in tests.
type NoopRequestNotifier struct{}

func (NoopRequestNotifier) RequestCreated(context.Context, RequestPrompt) {}

func (NoopRequestNotifier) RequestDecided(context.Context, RequestOutcome) {}
