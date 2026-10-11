package core

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// Viewer is who is asking. The auth middleware builds it on every request; Beny Bot builds it from the
// caller's Discord ID. IsDM is derived from DM_DISCORD_ID each time and never stored.
type Viewer struct {
	MemberID     uuid.UUID
	Status       MemberStatus
	IsDM         bool
	ViewAsPlayer bool
	// SessionTokenHash identifies the web session that made the request. It is nil for Beny Bot and the
	// seed, which have no session.
	SessionTokenHash []byte
}

// SeesDmFields reports whether responses keep their `dm` objects. View as player turns them off.
func (viewer Viewer) SeesDmFields() bool {
	return viewer.IsDM && !viewer.ViewAsPlayer
}

func (viewer Viewer) IsActive() bool {
	return viewer.Status == store.MemberStatusACTIVE
}

// ViewerIdentity is what a caller knows about a member when it builds their Viewer.
type ViewerIdentity struct {
	MemberID      uuid.UUID
	DiscordUserID string
	Status        MemberStatus
	// ViewAsPlayer is the web session's flag; it means nothing for anyone but the DM.
	ViewAsPlayer     bool
	SessionTokenHash []byte
}

// NewViewer is the one place the DM is recognized: by Discord ID, on every call, never from storage.
// The DM is always ACTIVE.
func (service *Service) NewViewer(identity ViewerIdentity) Viewer {
	isDM := service.isDM(identity.DiscordUserID)
	status := identity.Status
	if isDM {
		status = store.MemberStatusACTIVE
	}
	return Viewer{
		MemberID:         identity.MemberID,
		Status:           status,
		IsDM:             isDM,
		ViewAsPlayer:     isDM && identity.ViewAsPlayer,
		SessionTokenHash: identity.SessionTokenHash,
	}
}

// ViewerByDiscordID resolves a Discord user (Beny Bot's caller) to a Viewer. Unknown users get NotFound.
func (service *Service) ViewerByDiscordID(requestContext context.Context, discordUserID string) (Viewer, error) {
	member, err := service.queries.GetMemberByDiscordUserID(requestContext, discordUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Viewer{}, NotFound("no member with that Discord account")
	}
	if err != nil {
		return Viewer{}, err
	}
	return service.NewViewer(ViewerIdentity{MemberID: member.ID, DiscordUserID: member.DiscordUserID, Status: member.Status}), nil
}

// SeedDmDiscordID is the Discord ID `colony seed` gives the sample DM when DM_DISCORD_ID is unset; a
// devauth build then treats that member as the DM. No real Discord account can have it.
const SeedDmDiscordID = "seed:dm"
