package core

import (
	"context"

	"github.com/google/uuid"
)

// Notifier is the single seam between core and Discord; Beny Bot (internal/bot) implements it. Core calls
// it after the transaction commits. Delivery is best-effort: implementations log failures and never
// block or fail the operation (spec §7.2, "No outbox").
type Notifier interface {
	// MemberWaiting announces a first-time sign-in that created a PENDING member. Core calls it only for
	// brand-new members (never for a REMOVED member signing in again, never past the pending cap).
	// The implementation applies the five-pings-an-hour limit and posts to the DM channel from
	// settings, skipping quietly when no channel is set.
	MemberWaiting(requestContext context.Context, member WaitingMember)
}

type WaitingMember struct {
	MemberID        uuid.UUID
	DisplayName     string
	DiscordUsername string
}

// NoopNotifier is used when Beny Bot is off (no Discord variables) and in tests.
type NoopNotifier struct{}

func (NoopNotifier) MemberWaiting(context.Context, WaitingMember) {}
