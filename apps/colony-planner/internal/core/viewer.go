package core

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// Caller is who is asking, before any campaign: the auth middleware builds it from the session cache.
// Account functions (my campaigns, invites, campaign requests) take a Caller; owner functions take a
// Caller and check IsSiteOwner. A Caller grants nothing inside a campaign: only a Viewer does.
type Caller struct {
	AccountID     uuid.UUID
	DiscordUserID string
	// IsSiteOwner is derived from SITE_OWNER_DISCORD_ID on every request (Service.IsSiteOwner), never
	// stored.
	IsSiteOwner bool
	// SessionTokenHash identifies the web session; nil for callers without one (the seed).
	SessionTokenHash []byte
	// ViewAsPlayer is the session's toggle. It applies in every campaign where the caller is DM.
	ViewAsPlayer bool
}

// Viewer is someone inside one campaign. Only core builds one, from a membership read in the same
// call (EnterCampaign, ViewerForGuild), so a Viewer is proof of membership. Its fields are unexported
// and it has no site-owner flag, so campaign code cannot consult owner status even by mistake. Its
// zero value names no campaign, and every campaign function refuses it.
type Viewer struct {
	accountID        uuid.UUID
	memberID         uuid.UUID
	campaignID       uuid.UUID
	sessionTokenHash []byte
	viewAsPlayer     bool
	role             CampaignRole
	status           MemberStatus
	campaignStatus   CampaignStatus
}

func (viewer Viewer) CampaignID() uuid.UUID { return viewer.campaignID }

// MemberID is the campaign_member id, not the account id.
func (viewer Viewer) MemberID() uuid.UUID { return viewer.memberID }

func (viewer Viewer) AccountID() uuid.UUID { return viewer.accountID }

// IsDM reports the stored role; view as player does not change it (SeesDmFields does).
func (viewer Viewer) IsDM() bool { return viewer.role == store.CampaignRoleDM }

func (viewer Viewer) Status() MemberStatus { return viewer.status }

func (viewer Viewer) IsActive() bool { return viewer.status == store.MemberStatusACTIVE }

// ViewAsPlayer is the session's toggle, and only ever true for the DM.
func (viewer Viewer) ViewAsPlayer() bool { return viewer.viewAsPlayer }

// SeesDmFields reports whether responses keep their `dm` objects. View as player turns them off.
func (viewer Viewer) SeesDmFields() bool { return viewer.IsDM() && !viewer.viewAsPlayer }

func (viewer Viewer) CampaignStatus() CampaignStatus { return viewer.campaignStatus }

// CampaignActive is false while the site owner has suspended the campaign.
func (viewer Viewer) CampaignActive() bool {
	return viewer.campaignStatus == store.CampaignStatusACTIVE
}

// AsPlayer is the same Viewer with view as player on: it only ever lowers privileges. Beny Bot answers
// every command from the player projection, even when the DM runs it (spec §7.2).
func (viewer Viewer) AsPlayer() Viewer {
	viewer.viewAsPlayer = viewer.IsDM()
	return viewer
}

// SessionTokenHash is nil for Beny Bot and the seed.
func (viewer Viewer) SessionTokenHash() []byte { return viewer.sessionTokenHash }

// ErrNoSuchCampaign answers a non-member, the site owner included, exactly as a campaign that doesn't
// exist; an unparsable campaign ID gets it too.
var ErrNoSuchCampaign = &Error{Kind: KindNotFound, Message: "No such campaign."}

func newViewer(caller Caller, membership store.CampaignMember, campaign store.Campaign) Viewer {
	isDM := membership.Role == store.CampaignRoleDM
	return Viewer{
		accountID:        membership.AccountID,
		memberID:         membership.ID,
		campaignID:       membership.CampaignID,
		sessionTokenHash: caller.SessionTokenHash,
		viewAsPlayer:     isDM && caller.ViewAsPlayer,
		role:             membership.Role,
		status:           membership.Status,
		campaignStatus:   campaign.Status,
	}
}

// EnterCampaign is the one way into a campaign: one indexed read of the caller's membership and its
// campaign. No membership, for anyone (the site owner included), is ErrNoSuchCampaign. There is no
// membership cache, so there is no stale grant to evict.
func (service *Service) EnterCampaign(requestContext context.Context, caller Caller, campaignID uuid.UUID) (Viewer, error) {
	if caller.AccountID == uuid.Nil || campaignID == uuid.Nil {
		return Viewer{}, ErrNoSuchCampaign
	}
	row, err := service.queries.GetMembershipForAccount(requestContext, store.GetMembershipForAccountParams{
		CampaignID: campaignID,
		AccountID:  caller.AccountID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Viewer{}, ErrNoSuchCampaign
	}
	if err != nil {
		return Viewer{}, err
	}
	return newViewer(caller, row.CampaignMember, row.Campaign), nil
}

// Beny Bot's answers when an interaction can't become a Viewer (docs/design/multi-campaign.md §6.3).
var (
	ErrGuildNotLinked = &Error{Kind: KindNotFound, Message: "This server isn't linked to a campaign yet. A DM gets a code in their campaign's Settings and runs /link here."}
	// ErrCampaignSuspended is also the answer of every member and dm level route of a suspended campaign.
	ErrCampaignSuspended = &Error{Kind: KindForbidden, Message: "This campaign is paused by the site owner."}
	ErrNotInCampaign     = &Error{Kind: KindForbidden, Message: "You're not in this campaign. Ask its DM for the invite link."}
	ErrNotLetIn          = &Error{Kind: KindForbidden, Message: "The DM hasn't let you in yet."}
)

// ViewerForGuild is Beny Bot's way into a campaign: one read joining the guild link, the campaign, the
// account and its membership. Only an ACTIVE member of an ACTIVE campaign gets a Viewer.
func (service *Service) ViewerForGuild(requestContext context.Context, guildID string, discordUserID string) (Viewer, error) {
	row, err := service.queries.ViewerForGuild(requestContext, store.ViewerForGuildParams{GuildID: guildID, DiscordUserID: discordUserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Viewer{}, ErrGuildNotLinked
	}
	if err != nil {
		return Viewer{}, err
	}
	if row.CampaignStatus != store.CampaignStatusACTIVE {
		return Viewer{}, ErrCampaignSuspended
	}
	if row.MemberID == nil || row.MemberAccountID == nil || row.MemberRole == nil || row.MemberStatus == nil {
		return Viewer{}, ErrNotInCampaign
	}
	if *row.MemberStatus != store.MemberStatusACTIVE {
		return Viewer{}, ErrNotLetIn
	}
	return Viewer{
		accountID:      *row.MemberAccountID,
		memberID:       *row.MemberID,
		campaignID:     row.LinkedCampaignID,
		role:           *row.MemberRole,
		status:         *row.MemberStatus,
		campaignStatus: row.CampaignStatus,
	}, nil
}

// SeedDmDiscordID is the Discord ID `colony seed` gives the sample DM when SITE_OWNER_DISCORD_ID is
// unset; a devauth build then treats that account as the site owner. No real Discord account has it.
const SeedDmDiscordID = "seed:dm"
