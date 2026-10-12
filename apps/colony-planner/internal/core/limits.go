package core

import "time"

// The limits of docs/design/multi-campaign.md §8. None is a game rule: owner approval is the gate for
// new campaigns and the DM approves every player, so nothing else is capped.
const (
	// OpenCampaignRequestsPerAccount: "You already asked. Withdraw it to change it."
	OpenCampaignRequestsPerAccount = 1
	// PendingMemberLimit caps how many people wait to join one campaign, so a leaked invite link
	// can't flood its /players page.
	PendingMemberLimit = 20
	// WaitingPingsPerHour is Beny Bot's limit on "is waiting to join" posts, per campaign.
	WaitingPingsPerHour = 5
	// OwnerRequestPingsPerHour is Beny Bot's limit on direct messages to the site owner.
	OwnerRequestPingsPerHour = 5
	// LinkCodeLifetime: one live code per campaign, single use.
	LinkCodeLifetime = 15 * time.Minute
	// NpcLimit and PlaceLimit are technical bounds well beyond any table's cast, so a runaway script
	// can't fill the database; they are not game rules.
	NpcLimit   = 2000
	PlaceLimit = 1000

	campaignNameMaxLength = 80
	slugMinLength         = 3
	slugMaxLength         = 40
	requestNoteMaxLength  = 500
)
