package core

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// PendingMemberLimit caps how many people can wait at once; past it, new sign-ins are turned away so
// a leaked link can't flood /players (spec §2.1).
const PendingMemberLimit = 20

// ErrSignUpsFull is returned to a first-time sign-in while PendingMemberLimit people are waiting.
var ErrSignUpsFull = &Error{Kind: KindConflict, Message: "Not accepting new players right now."}

// DiscordIdentity is what Discord's GET /users/@me says about the person signing in (or what dev
// sign-in makes up).
type DiscordIdentity struct {
	UserID      string
	Username    string
	DisplayName string
	AvatarURL   *string
}

const displayNameMaxLength = 100

// SignIn records a Discord sign-in and returns the member it belongs to. The DM is always ACTIVE; a
// first-time sign-in becomes PENDING (and Beny Bot pings the DM); everyone else keeps their status, so a
// removed or denied member stays REMOVED and is not announced again (spec §8.5).
func (service *Service) SignIn(requestContext context.Context, identity DiscordIdentity) (uuid.UUID, error) {
	identity, err := normalizeIdentity(identity)
	if err != nil {
		return uuid.Nil, err
	}
	isDM := service.isDM(identity.UserID)
	var member store.Member
	created := false
	becameActive := false
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		if err := queries.LockMemberSignIns(requestContext); err != nil {
			return err
		}
		existing, err := queries.GetMemberByDiscordUserID(requestContext, identity.UserID)
		if err == nil {
			becameActive = isDM && existing.Status != store.MemberStatusACTIVE
			member, err = queries.UpdateMemberFromDiscord(requestContext, store.UpdateMemberFromDiscordParams{
				DiscordUsername: identity.Username,
				DisplayName:     identity.DisplayName,
				AvatarUrl:       identity.AvatarURL,
				MakeActive:      isDM,
				ID:              existing.ID,
			})
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		signedInAt := time.Now().UTC()
		parameters := store.CreateMemberParams{
			DiscordUserID:   identity.UserID,
			DiscordUsername: identity.Username,
			DisplayName:     identity.DisplayName,
			AvatarUrl:       identity.AvatarURL,
			Status:          store.MemberStatusPENDING,
			LastSeenAt:      &signedInAt,
		}
		if isDM {
			parameters.Status = store.MemberStatusACTIVE
			parameters.ApprovedAt = &signedInAt
		} else {
			pending, err := queries.CountPendingMembers(requestContext)
			if err != nil {
				return err
			}
			if pending >= PendingMemberLimit {
				return ErrSignUpsFull
			}
		}
		member, err = queries.CreateMember(requestContext, parameters)
		if err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}

	if becameActive {
		service.sessions.EvictMember(member.ID)
	}
	if created || becameActive {
		service.broker.Publish(TopicMembers)
	}
	if created && member.Status == store.MemberStatusPENDING {
		// The request may end before Discord answers; the ping is best-effort and must not be cut short.
		service.notifier.MemberWaiting(context.WithoutCancel(requestContext), WaitingMember{
			MemberID:        member.ID,
			DisplayName:     member.DisplayName,
			DiscordUsername: member.DiscordUsername,
		})
	}
	return member.ID, nil
}

func normalizeIdentity(identity DiscordIdentity) (DiscordIdentity, error) {
	identity.UserID = strings.TrimSpace(identity.UserID)
	identity.Username = strings.TrimSpace(identity.Username)
	identity.DisplayName = strings.TrimSpace(identity.DisplayName)
	if identity.UserID == "" || identity.Username == "" {
		return DiscordIdentity{}, Invalid("Discord did not say who you are.")
	}
	if identity.DisplayName == "" {
		identity.DisplayName = identity.Username
	}
	if utf8.RuneCountInString(identity.DisplayName) > displayNameMaxLength {
		identity.DisplayName = string([]rune(identity.DisplayName)[:displayNameMaxLength])
	}
	return identity, nil
}

// SignOut ends the caller's current web session and closes its live streams.
func (service *Service) SignOut(requestContext context.Context, viewer Viewer) error {
	if len(viewer.SessionTokenHash) == 0 {
		return nil
	}
	if err := service.queries.DeleteSession(requestContext, viewer.SessionTokenHash); err != nil {
		return err
	}
	service.sessions.EvictSession(viewer.SessionTokenHash)
	service.broker.CloseSession(viewer.SessionTokenHash)
	return nil
}

// SignOutEverywhere ends every web session of the caller, on every device, and closes their streams.
func (service *Service) SignOutEverywhere(requestContext context.Context, viewer Viewer) error {
	if err := service.queries.DeleteMemberSessions(requestContext, viewer.MemberID); err != nil {
		return err
	}
	service.sessions.EvictMember(viewer.MemberID)
	service.broker.CloseMember(viewer.MemberID)
	return nil
}
