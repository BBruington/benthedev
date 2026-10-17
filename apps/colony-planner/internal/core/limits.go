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
	// QuestLimit, PatronLimit and PatronActionLimit are technical bounds of the same kind. Patrons and
	// actions are never deleted, so only unarchived ones count (creating or unarchiving one), or the
	// limit would be a lifetime cap with no way back under it. The ...IncludingArchived ceilings ten
	// times higher keep a create-and-archive loop from growing the tables forever. Requests have none:
	// no limits, cooldowns or quotas were designed (Open Question 12).
	QuestLimit                         = 2000
	PatronLimit                        = 500
	PatronLimitIncludingArchived       = 5000
	PatronActionLimit                  = 2000
	PatronActionLimitIncludingArchived = 20000

	// ImageBudgetBytes is the image storage of one campaign (docs/design/multi-campaign.md §8), counted
	// over the images in use, and ImageMaxBytes one upload (spec §8.9).
	ImageBudgetBytes = 50 * 1024 * 1024
	ImageMaxBytes    = 5 * 1024 * 1024
	// ImageMaxSide and ImageMaxPixels keep an image one a phone can decode: the server reads only the
	// header, so a small file declaring a huge canvas would otherwise reach every player's browser.
	ImageMaxSide   = 16384
	ImageMaxPixels = 50_000_000
	// HexLimit is the charted hexes of one campaign; HexCellsPerCall is one paint or erase call, so a
	// longer stroke is sent in parts (spec §5.10). HexCoordinateLimit only refuses bad input: the map
	// has no edge (decision 9).
	HexLimit           = 50000
	HexCellsPerCall    = 200
	HexCoordinateLimit = 100000
	// MapLimit is a technical bound of the same kind as PlaceLimit.
	MapLimit = 200

	campaignNameMaxLength = 80
	slugMinLength         = 3
	slugMaxLength         = 40
	requestNoteMaxLength  = 500
)
