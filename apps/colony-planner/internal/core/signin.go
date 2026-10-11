package core

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/store"
)

// DiscordIdentity is what Discord's GET /users/@me says about the person signing in (or what dev
// sign-in makes up).
type DiscordIdentity struct {
	UserID      string
	Username    string
	DisplayName string
	AvatarURL   *string
}

const (
	displayNameMaxLength = 100
	// Discord usernames are at most 32 characters; the slack covers older name formats.
	usernameMaxLength = 100
)

// SignIn records a Discord sign-in and returns the account it belongs to. It only creates or refreshes
// the account: nobody becomes a member of anything, and nobody is pinged. Joining is by invite link.
func (service *Service) SignIn(requestContext context.Context, identity DiscordIdentity) (uuid.UUID, error) {
	identity, err := normalizeIdentity(identity)
	if err != nil {
		return uuid.Nil, err
	}
	account, err := service.queries.UpsertAccountFromDiscord(requestContext, store.UpsertAccountFromDiscordParams{
		DiscordUserID:   identity.UserID,
		DiscordUsername: identity.Username,
		DisplayName:     identity.DisplayName,
		AvatarUrl:       identity.AvatarURL,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return account.ID, nil
}

func normalizeIdentity(identity DiscordIdentity) (DiscordIdentity, error) {
	identity.UserID = strings.TrimSpace(identity.UserID)
	username, usernameError := cleanText("username", identity.Username, usernameMaxLength)
	// A long display name is cut rather than refused: Discord chose it, and it is only shown.
	displayName, displayNameError := cleanText("displayName", truncateRunes(strings.TrimSpace(identity.DisplayName), displayNameMaxLength), displayNameMaxLength)
	for _, fieldError := range []*FieldError{usernameError, displayNameError} {
		if fieldError != nil {
			return DiscordIdentity{}, Invalid("Your Discord name has characters the planner can't store.", *fieldError)
		}
	}
	identity.Username = username
	identity.DisplayName = displayName
	if identity.UserID == "" || identity.Username == "" {
		return DiscordIdentity{}, Invalid("Discord did not say who you are.")
	}
	if identity.DisplayName == "" {
		identity.DisplayName = identity.Username
	}
	return identity, nil
}

func truncateRunes(value string, maxLength int) string {
	if utf8.RuneCountInString(value) <= maxLength {
		return value
	}
	return string([]rune(value)[:maxLength])
}

// SignOut ends the caller's current web session and closes its live streams in every campaign.
func (service *Service) SignOut(requestContext context.Context, caller Caller) error {
	if len(caller.SessionTokenHash) == 0 {
		return nil
	}
	if err := service.queries.DeleteSession(requestContext, caller.SessionTokenHash); err != nil {
		return err
	}
	service.sessions.EvictSession(caller.SessionTokenHash)
	service.broker.CloseSession(caller.SessionTokenHash)
	return nil
}

// SignOutEverywhere ends every web session of the caller's account, on every device, and closes their
// streams.
func (service *Service) SignOutEverywhere(requestContext context.Context, caller Caller) error {
	if err := requireSignedIn(caller); err != nil {
		return err
	}
	if err := service.queries.DeleteAccountSessions(requestContext, caller.AccountID); err != nil {
		return err
	}
	service.sessions.EvictAccount(caller.AccountID)
	service.broker.CloseAccount(caller.AccountID)
	return nil
}
