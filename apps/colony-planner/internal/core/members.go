package core

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// Member is what any active member of a campaign may see about another member there. Everything else
// is DM-only and lives in Dm, which the redaction Transformer strips for everyone but the DM. ID is
// the membership (campaign_member) id, which is what /players acts on.
type Member struct {
	ID            uuid.UUID `json:"id" format:"uuid"`
	DisplayName   string    `json:"displayName"`
	AvatarURL     *string   `json:"avatarUrl"`
	CharacterName *string   `json:"characterName"`
	Dm            *MemberDm `json:"dm,omitempty"`
}

type MemberDm struct {
	DiscordUserID   string       `json:"discordUserId"`
	DiscordUsername string       `json:"discordUsername"`
	Status          MemberStatus `json:"status"`
	IsDM            bool         `json:"isDm"`
	// RemovalKind is how a REMOVED member got there, the label /players shows; null for anyone else.
	RemovalKind *RemovalKind `json:"removalKind" nullable:"true" doc:"null unless status is REMOVED"`
	CreatedAt   time.Time    `json:"createdAt"`
	ApprovedAt  *time.Time   `json:"approvedAt"`
	LastSeenAt  *time.Time   `json:"lastSeenAt"`
}

// CampaignMe is the caller's own view of themselves in one campaign (the pending-level GET .../me).
// Status, IsDM and ViewAsPlayer are about the caller, so they sit outside any `dm` object and survive
// redaction (the View as player banner needs them). Campaign carries the status, so a paused
// campaign's members see the right screen.
type CampaignMe struct {
	Campaign CampaignSummary `json:"campaign"`
	// DmDisplayName is directory data, as on the invite page: the waiting page names who lets you in.
	DmDisplayName string       `json:"dmDisplayName"`
	Member        Member       `json:"member"`
	Status        MemberStatus `json:"status"`
	IsDM          bool         `json:"isDm"`
	ViewAsPlayer  bool         `json:"viewAsPlayer"`
}

// CampaignSummary is directory data about a campaign: what its members and the site owner may see.
type CampaignSummary struct {
	ID     uuid.UUID      `json:"id" format:"uuid"`
	Slug   string         `json:"slug"`
	Name   string         `json:"name"`
	Status CampaignStatus `json:"status"`
}

type ViewAsPlayerInput struct {
	Enabled bool `json:"enabled"`
}

// ProfileInput replaces the caller's whole profile in this campaign (PUT semantics). A null
// characterName clears it. Lengths and formats are checked in core, not in schema tags, so the person
// reads core's messages.
type ProfileInput struct {
	CharacterName *string `json:"characterName" doc:"At most 60 characters after trimming; blank or null clears it"`
}

type CharacterNameInput struct {
	CharacterName *string `json:"characterName" doc:"At most 60 characters after trimming; blank or null clears it"`
}

// MemberGroups is the DM's /players page: who is waiting, who plays, who was removed, denied or left.
type MemberGroups struct {
	Pending []Member `json:"pending"`
	Active  []Member `json:"active"`
	Removed []Member `json:"removed"`
}

const characterNameMaxLength = 60

// toMember fills Dm unconditionally; the Transformer decides who keeps it.
func toMember(membership store.CampaignMember, account store.Account) Member {
	return Member{
		ID:            membership.ID,
		DisplayName:   account.DisplayName,
		AvatarURL:     account.AvatarUrl,
		CharacterName: membership.CharacterName,
		Dm: &MemberDm{
			DiscordUserID:   account.DiscordUserID,
			DiscordUsername: account.DiscordUsername,
			Status:          membership.Status,
			IsDM:            membership.Role == store.CampaignRoleDM,
			RemovalKind:     removalKindOf(membership),
			CreatedAt:       membership.CreatedAt.UTC(),
			ApprovedAt:      utcPointer(membership.ApprovedAt),
			LastSeenAt:      utcPointer(account.LastSeenAt),
		},
	}
}

func toCampaignSummary(campaign store.Campaign) CampaignSummary {
	return CampaignSummary{ID: campaign.ID, Slug: campaign.Slug, Name: campaign.Name, Status: campaign.Status}
}

func utcPointer(instant *time.Time) *time.Time {
	if instant == nil {
		return nil
	}
	converted := instant.UTC()
	return &converted
}

func memberNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return NotFound("No such member.")
	}
	return err
}

// CampaignMe answers any membership, whatever its status and the campaign's: the waiting page, the
// "You don't have access" page and the paused page all start here.
func (service *Service) CampaignMe(requestContext context.Context, viewer Viewer) (CampaignMe, error) {
	if err := requireMembership(viewer); err != nil {
		return CampaignMe{}, err
	}
	row, err := service.queries.GetCampaignMember(requestContext, store.GetCampaignMemberParams{CampaignID: viewer.campaignID, ID: viewer.memberID})
	if err != nil {
		return CampaignMe{}, campaignGone(err)
	}
	campaign, err := service.queries.GetCampaignWithDm(requestContext, viewer.campaignID)
	if err != nil {
		return CampaignMe{}, campaignGone(err)
	}
	return CampaignMe{
		Campaign:      toCampaignSummary(campaign.Campaign),
		DmDisplayName: campaign.DmDisplayName,
		Member:        toMember(row.CampaignMember, row.Account),
		Status:        row.CampaignMember.Status,
		IsDM:          row.CampaignMember.Role == store.CampaignRoleDM,
		ViewAsPlayer:  viewer.viewAsPlayer,
	}, nil
}

// SetViewAsPlayer flips view as player on the caller's current web session. Only a DM may call it, and
// it must keep working while view as player is on, so the check is IsDM rather than requireDm. The
// toggle is per session: it applies in every campaign where the caller is DM. It answers nothing: the
// request ran under the old projection, so the client reads .../me afresh.
func (service *Service) SetViewAsPlayer(requestContext context.Context, viewer Viewer, input ViewAsPlayerInput) error {
	if err := requireCampaign(viewer); err != nil {
		return err
	}
	if !viewer.IsDM() {
		return Forbidden("Only the DM can view as player.")
	}
	if len(viewer.sessionTokenHash) == 0 {
		return Invalid("View as player belongs to a signed-in browser session.")
	}
	updated, err := service.queries.SetSessionViewAsPlayer(requestContext, store.SetSessionViewAsPlayerParams{
		ViewAsPlayer: input.Enabled,
		TokenHash:    viewer.sessionTokenHash,
	})
	if err != nil {
		return err
	}
	if updated == 0 {
		return service.sessionEnded()
	}
	service.sessions.EvictSession(viewer.sessionTokenHash)
	// Every projection this session sees just changed, so its open tabs refetch everything.
	service.broker.publishToSession(viewer.sessionTokenHash, AllTopics()...)
	return nil
}

// UpdateProfile sets the caller's own character name in this campaign.
func (service *Service) UpdateProfile(requestContext context.Context, viewer Viewer, input ProfileInput) (CampaignMe, error) {
	if err := requireActive(viewer); err != nil {
		return CampaignMe{}, err
	}
	characterName, err := normalizeCharacterName(input.CharacterName)
	if err != nil {
		return CampaignMe{}, err
	}
	if _, err := service.queries.SetCampaignMemberCharacterName(requestContext, store.SetCampaignMemberCharacterNameParams{
		CharacterName: characterName,
		CampaignID:    viewer.campaignID,
		ID:            viewer.memberID,
	}); err != nil {
		return CampaignMe{}, campaignGone(err)
	}
	service.broker.publishToMember(viewer.campaignID, viewer.memberID, TopicMe)
	service.broker.Publish(viewer.campaignID, TopicMembers)
	return service.CampaignMe(requestContext, viewer)
}

func normalizeCharacterName(characterName *string) (*string, error) {
	if characterName == nil {
		return nil, nil
	}
	cleaned, fieldError := cleanText("characterName", *characterName, characterNameMaxLength)
	if fieldError != nil {
		return nil, Invalid("Check the character name.", *fieldError)
	}
	if cleaned == "" {
		return nil, nil
	}
	return &cleaned, nil
}

func (service *Service) ListMembers(requestContext context.Context, viewer Viewer) (MemberGroups, error) {
	if err := requireDm(viewer); err != nil {
		return MemberGroups{}, err
	}
	rows, err := service.queries.ListCampaignMembers(requestContext, viewer.campaignID)
	if err != nil {
		return MemberGroups{}, err
	}
	groups := MemberGroups{Pending: []Member{}, Active: []Member{}, Removed: []Member{}}
	for _, row := range rows {
		member := toMember(row.CampaignMember, row.Account)
		switch row.CampaignMember.Status {
		case store.MemberStatusPENDING:
			groups.Pending = append(groups.Pending, member)
		case store.MemberStatusACTIVE:
			groups.Active = append(groups.Active, member)
		case store.MemberStatusREMOVED:
			groups.Removed = append(groups.Removed, member)
		}
	}
	return groups, nil
}

// statusChange is one of the DM's decisions on /players, or a player leaving.
type statusChange struct {
	to          store.MemberStatus
	allowedFrom []store.MemberStatus
	// conflicts explains, per current status, why the change doesn't apply.
	conflicts map[store.MemberStatus]string
	leftOnOwn bool
}

var (
	approveChange = statusChange{
		to:          store.MemberStatusACTIVE,
		allowedFrom: []store.MemberStatus{store.MemberStatusPENDING},
		conflicts: map[store.MemberStatus]string{
			store.MemberStatusREMOVED: "They were removed, denied or left. Re-admit them instead.",
		},
	}
	// Denying keeps the person's membership, so their waiting page turns into "You don't have access".
	denyChange = statusChange{
		to:          store.MemberStatusREMOVED,
		allowedFrom: []store.MemberStatus{store.MemberStatusPENDING},
		conflicts: map[store.MemberStatus]string{
			store.MemberStatusACTIVE: "They are already in. Remove them instead.",
		},
	}
	// Removal is per campaign: the person's session and their other campaigns are untouched.
	removeChange = statusChange{
		to:          store.MemberStatusREMOVED,
		allowedFrom: []store.MemberStatus{store.MemberStatusACTIVE, store.MemberStatusPENDING},
	}
	readmitChange = statusChange{
		to:          store.MemberStatusACTIVE,
		allowedFrom: []store.MemberStatus{store.MemberStatusREMOVED},
		conflicts: map[store.MemberStatus]string{
			store.MemberStatusPENDING: "They are still waiting. Approve them instead.",
		},
	}
	// Leaving is removal the member chose (decision C5): from PENDING (withdrawing the request) or
	// ACTIVE, never from REMOVED, so it can't be a way back in.
	leaveChange = statusChange{
		to:          store.MemberStatusREMOVED,
		allowedFrom: []store.MemberStatus{store.MemberStatusACTIVE, store.MemberStatusPENDING},
		conflicts: map[store.MemberStatus]string{
			store.MemberStatusREMOVED: "You're not in this campaign.",
		},
		leftOnOwn: true,
	}
)

func (service *Service) ApproveMember(requestContext context.Context, viewer Viewer, memberID uuid.UUID) (Member, error) {
	if err := requireDm(viewer); err != nil {
		return Member{}, err
	}
	return service.changeMemberStatus(requestContext, viewer.campaignID, memberID, approveChange)
}

func (service *Service) DenyMember(requestContext context.Context, viewer Viewer, memberID uuid.UUID) (Member, error) {
	if err := requireDm(viewer); err != nil {
		return Member{}, err
	}
	return service.changeMemberStatus(requestContext, viewer.campaignID, memberID, denyChange)
}

// RemoveMember sets the membership to REMOVED and rescopes its streams in this campaign to `me`. The
// person stays signed in: their other campaigns keep working.
func (service *Service) RemoveMember(requestContext context.Context, viewer Viewer, memberID uuid.UUID) (Member, error) {
	if err := requireDm(viewer); err != nil {
		return Member{}, err
	}
	return service.changeMemberStatus(requestContext, viewer.campaignID, memberID, removeChange)
}

func (service *Service) ReadmitMember(requestContext context.Context, viewer Viewer, memberID uuid.UUID) (Member, error) {
	if err := requireDm(viewer); err != nil {
		return Member{}, err
	}
	return service.changeMemberStatus(requestContext, viewer.campaignID, memberID, readmitChange)
}

// LeaveCampaign removes the caller's own membership, and only theirs (decision C5). It works from
// PENDING or ACTIVE, even in a suspended campaign; the DM can't leave (they delete the campaign).
// Afterwards the membership is REMOVED like any removal: the invite link can't bring them back and
// pings nobody, and only the DM re-admits them.
func (service *Service) LeaveCampaign(requestContext context.Context, viewer Viewer) (CampaignMe, error) {
	if err := requireMembership(viewer); err != nil {
		return CampaignMe{}, err
	}
	if viewer.IsDM() {
		return CampaignMe{}, Forbidden("The DM can't leave. Delete the campaign in Settings instead.")
	}
	if _, err := service.changeMemberStatus(requestContext, viewer.campaignID, viewer.memberID, leaveChange); err != nil {
		return CampaignMe{}, err
	}
	return service.CampaignMe(requestContext, viewer)
}

func (service *Service) changeMemberStatus(requestContext context.Context, campaignID uuid.UUID, memberID uuid.UUID, change statusChange) (Member, error) {
	var result store.CampaignMember
	var account store.Account
	changed := false
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		current, err := queries.GetCampaignMemberForUpdate(requestContext, store.GetCampaignMemberForUpdateParams{CampaignID: campaignID, ID: memberID})
		if err != nil {
			return memberNotFound(err)
		}
		account = current.Account
		result = current.CampaignMember
		if current.CampaignMember.Role == store.CampaignRoleDM {
			return Forbidden("The DM is always in.")
		}
		// Repeating a DM decision changes nothing; leaving again is refused, so it can't hide a removal.
		if current.CampaignMember.Status == change.to && !change.leftOnOwn {
			return nil
		}
		if !slices.Contains(change.allowedFrom, current.CampaignMember.Status) {
			message, explained := change.conflicts[current.CampaignMember.Status]
			if !explained {
				message = "That doesn't apply to this member."
			}
			return Conflict(message)
		}
		result, err = queries.SetCampaignMemberStatus(requestContext, store.SetCampaignMemberStatusParams{
			Status:     change.to,
			LeftOnOwn:  change.leftOnOwn,
			CampaignID: campaignID,
			ID:         memberID,
		})
		if err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return Member{}, err
	}
	if changed {
		service.broker.memberStatusChanged(memberID, change.to == store.MemberStatusACTIVE)
		service.broker.publishToMember(campaignID, memberID, TopicMe)
		service.broker.Publish(campaignID, TopicMembers)
	}
	return toMember(result, account), nil
}

func (service *Service) SetCharacterName(requestContext context.Context, viewer Viewer, memberID uuid.UUID, input CharacterNameInput) (Member, error) {
	if err := requireDm(viewer); err != nil {
		return Member{}, err
	}
	characterName, err := normalizeCharacterName(input.CharacterName)
	if err != nil {
		return Member{}, err
	}
	if _, err := service.queries.SetCampaignMemberCharacterName(requestContext, store.SetCampaignMemberCharacterNameParams{
		CharacterName: characterName,
		CampaignID:    viewer.campaignID,
		ID:            memberID,
	}); err != nil {
		return Member{}, memberNotFound(err)
	}
	row, err := service.queries.GetCampaignMember(requestContext, store.GetCampaignMemberParams{CampaignID: viewer.campaignID, ID: memberID})
	if err != nil {
		return Member{}, memberNotFound(err)
	}
	service.broker.publishToMember(viewer.campaignID, memberID, TopicMe)
	service.broker.Publish(viewer.campaignID, TopicMembers)
	return toMember(row.CampaignMember, row.Account), nil
}
