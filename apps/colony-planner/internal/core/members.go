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

// Member is what any signed-in member may see about another member. Everything else is DM-only and
// lives in Dm, which the redaction Transformer strips for everyone but the DM.
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
	CreatedAt       time.Time    `json:"createdAt"`
	ApprovedAt      *time.Time   `json:"approvedAt"`
	LastSeenAt      *time.Time   `json:"lastSeenAt"`
}

// Me is the caller's own view of themselves. Status, IsDM and ViewAsPlayer are about the caller, so
// they sit outside any `dm` object and survive redaction (the View as player banner needs them).
type Me struct {
	Member       Member       `json:"member"`
	Status       MemberStatus `json:"status"`
	IsDM         bool         `json:"isDm"`
	ViewAsPlayer bool         `json:"viewAsPlayer"`
}

type ViewAsPlayerInput struct {
	Enabled bool `json:"enabled"`
}

// ProfileInput replaces the caller's whole profile (PUT semantics). A null characterName clears it.
// Lengths and formats are checked in core, not in schema tags, so the person reads core's messages.
type ProfileInput struct {
	CharacterName *string `json:"characterName" doc:"At most 60 characters after trimming; blank or null clears it"`
}

type CharacterNameInput struct {
	CharacterName *string `json:"characterName" doc:"At most 60 characters after trimming; blank or null clears it"`
}

// MemberGroups is the DM's /players page: who is waiting, who plays, who was removed or denied.
type MemberGroups struct {
	Pending []Member `json:"pending"`
	Active  []Member `json:"active"`
	Removed []Member `json:"removed"`
}

const characterNameMaxLength = 60

// toMember fills Dm unconditionally; the Transformer decides who keeps it.
func (service *Service) toMember(row store.Member) Member {
	isDM := service.isDM(row.DiscordUserID)
	return Member{
		ID:            row.ID,
		DisplayName:   row.DisplayName,
		AvatarURL:     row.AvatarUrl,
		CharacterName: row.CharacterName,
		Dm: &MemberDm{
			DiscordUserID:   row.DiscordUserID,
			DiscordUsername: row.DiscordUsername,
			Status:          service.effectiveStatus(row),
			IsDM:            isDM,
			CreatedAt:       row.CreatedAt.UTC(),
			ApprovedAt:      utcPointer(row.ApprovedAt),
			LastSeenAt:      utcPointer(row.LastSeenAt),
		},
	}
}

// effectiveStatus is the stored status, except that the DM is always ACTIVE.
func (service *Service) effectiveStatus(row store.Member) store.MemberStatus {
	if service.isDM(row.DiscordUserID) {
		return store.MemberStatusACTIVE
	}
	return row.Status
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

func (service *Service) Me(requestContext context.Context, viewer Viewer) (Me, error) {
	row, err := service.queries.GetMember(requestContext, viewer.MemberID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Me{}, service.sessionEnded()
	}
	if err != nil {
		return Me{}, err
	}
	return service.toMe(row, viewer), nil
}

func (service *Service) toMe(row store.Member, viewer Viewer) Me {
	return Me{
		Member:       service.toMember(row),
		Status:       viewer.Status,
		IsDM:         viewer.IsDM,
		ViewAsPlayer: viewer.ViewAsPlayer,
	}
}

// SetViewAsPlayer flips view as player on the caller's current web session. Only the DM may call it,
// and it must keep working while view as player is on, so the check is IsDM rather than an access level.
// It answers nothing: the request ran under the old projection, so the client reads /api/me afresh.
func (service *Service) SetViewAsPlayer(requestContext context.Context, viewer Viewer, input ViewAsPlayerInput) error {
	if !viewer.IsDM {
		return Forbidden("Only the DM can view as player.")
	}
	if len(viewer.SessionTokenHash) == 0 {
		return Invalid("View as player belongs to a signed-in browser session.")
	}
	updated, err := service.queries.SetSessionViewAsPlayer(requestContext, store.SetSessionViewAsPlayerParams{
		ViewAsPlayer: input.Enabled,
		TokenHash:    viewer.SessionTokenHash,
	})
	if err != nil {
		return err
	}
	if updated == 0 {
		return service.sessionEnded()
	}
	service.sessions.EvictSession(viewer.SessionTokenHash)
	// Every projection this session sees just changed, so its open tabs refetch everything.
	service.broker.publishToSession(viewer.SessionTokenHash, AllTopics()...)
	return nil
}

func (service *Service) UpdateProfile(requestContext context.Context, viewer Viewer, input ProfileInput) (Me, error) {
	if err := requireActive(viewer); err != nil {
		return Me{}, err
	}
	characterName, err := normalizeCharacterName(input.CharacterName)
	if err != nil {
		return Me{}, err
	}
	row, err := service.queries.SetMemberCharacterName(requestContext, store.SetMemberCharacterNameParams{
		CharacterName: characterName,
		ID:            viewer.MemberID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Me{}, service.sessionEnded()
	}
	if err != nil {
		return Me{}, err
	}
	service.broker.publishToMember(viewer.MemberID, TopicMe)
	service.broker.Publish(TopicMembers)
	return service.toMe(row, viewer), nil
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
	rows, err := service.queries.ListMembers(requestContext)
	if err != nil {
		return MemberGroups{}, err
	}
	groups := MemberGroups{Pending: []Member{}, Active: []Member{}, Removed: []Member{}}
	for _, row := range rows {
		member := service.toMember(row)
		switch service.effectiveStatus(row) {
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

// statusChange is one of the DM's decisions on /players.
type statusChange struct {
	to          store.MemberStatus
	allowedFrom []store.MemberStatus
	// conflicts explains, per current status, why the change doesn't apply.
	conflicts map[store.MemberStatus]string
	// endSessions signs the member out everywhere and closes their streams.
	endSessions bool
}

var (
	approveChange = statusChange{
		to:          store.MemberStatusACTIVE,
		allowedFrom: []store.MemberStatus{store.MemberStatusPENDING},
		conflicts: map[store.MemberStatus]string{
			store.MemberStatusREMOVED: "They were removed or denied. Re-admit them instead.",
		},
	}
	// Denying keeps the person's session, so their /pending page turns into "You don't have access".
	denyChange = statusChange{
		to:          store.MemberStatusREMOVED,
		allowedFrom: []store.MemberStatus{store.MemberStatusPENDING},
		conflicts: map[store.MemberStatus]string{
			store.MemberStatusACTIVE: "They are already in. Remove them instead.",
		},
	}
	removeChange = statusChange{
		to:          store.MemberStatusREMOVED,
		allowedFrom: []store.MemberStatus{store.MemberStatusACTIVE, store.MemberStatusPENDING, store.MemberStatusREMOVED},
		endSessions: true,
	}
	readmitChange = statusChange{
		to:          store.MemberStatusACTIVE,
		allowedFrom: []store.MemberStatus{store.MemberStatusREMOVED},
		conflicts: map[store.MemberStatus]string{
			store.MemberStatusPENDING: "They are still waiting. Approve them instead.",
		},
	}
)

func (service *Service) ApproveMember(requestContext context.Context, viewer Viewer, memberID uuid.UUID) (Member, error) {
	return service.changeMemberStatus(requestContext, viewer, memberID, approveChange)
}

func (service *Service) DenyMember(requestContext context.Context, viewer Viewer, memberID uuid.UUID) (Member, error) {
	return service.changeMemberStatus(requestContext, viewer, memberID, denyChange)
}

// RemoveMember signs the member out everywhere at once: their sessions are deleted, evicted from the
// cache, and their live streams closed. Signing in again yields only a pending-level session.
func (service *Service) RemoveMember(requestContext context.Context, viewer Viewer, memberID uuid.UUID) (Member, error) {
	return service.changeMemberStatus(requestContext, viewer, memberID, removeChange)
}

func (service *Service) ReadmitMember(requestContext context.Context, viewer Viewer, memberID uuid.UUID) (Member, error) {
	return service.changeMemberStatus(requestContext, viewer, memberID, readmitChange)
}

func (service *Service) changeMemberStatus(requestContext context.Context, viewer Viewer, memberID uuid.UUID, change statusChange) (Member, error) {
	if err := requireDm(viewer); err != nil {
		return Member{}, err
	}
	var result store.Member
	changed := false
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		current, err := queries.GetMemberForUpdate(requestContext, memberID)
		if err != nil {
			return memberNotFound(err)
		}
		if service.isDM(current.DiscordUserID) {
			return Forbidden("The DM is always in.")
		}
		result = current
		if current.Status == change.to && !change.endSessions {
			return nil
		}
		if !slices.Contains(change.allowedFrom, current.Status) {
			message, explained := change.conflicts[current.Status]
			if !explained {
				message = "That doesn't apply to this member."
			}
			return Conflict(message)
		}
		if current.Status != change.to {
			result, err = queries.SetMemberStatus(requestContext, store.SetMemberStatusParams{Status: change.to, ID: memberID})
			if err != nil {
				return err
			}
		}
		if change.endSessions {
			if err := queries.DeleteMemberSessions(requestContext, memberID); err != nil {
				return err
			}
		}
		changed = true
		return nil
	})
	if err != nil {
		return Member{}, err
	}
	if changed {
		service.sessions.EvictMember(memberID)
		service.broker.memberStatusChanged(memberID, change.to == store.MemberStatusACTIVE)
		if change.endSessions {
			service.broker.CloseMember(memberID)
		}
		service.broker.publishToMember(memberID, TopicMe)
		service.broker.Publish(TopicMembers)
	}
	return service.toMember(result), nil
}

func (service *Service) SetCharacterName(requestContext context.Context, viewer Viewer, memberID uuid.UUID, input CharacterNameInput) (Member, error) {
	if err := requireDm(viewer); err != nil {
		return Member{}, err
	}
	characterName, err := normalizeCharacterName(input.CharacterName)
	if err != nil {
		return Member{}, err
	}
	row, err := service.queries.SetMemberCharacterName(requestContext, store.SetMemberCharacterNameParams{
		CharacterName: characterName,
		ID:            memberID,
	})
	if err != nil {
		return Member{}, memberNotFound(err)
	}
	service.broker.publishToMember(memberID, TopicMe)
	service.broker.Publish(TopicMembers)
	return service.toMember(row), nil
}
