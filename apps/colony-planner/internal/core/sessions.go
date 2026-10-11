package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// Core owns every read and write of web_session; internal/auth only carries the token in a cookie and
// caches lookups (spec §8.6). Removal and sign-out delete sessions here too, inside the same
// transaction as the status change.

const (
	// SessionLifetime is how long a web session lasts unused; using it slides the expiry forward.
	SessionLifetime = 30 * 24 * time.Hour
	// The sliding expiry and last_seen_at are written at most once a day, so ordinary requests and
	// reconnects never write (spec §8.5).
	sessionTouchInterval = 24 * time.Hour
	sessionTokenBytes    = 32
)

// WebSession is a newly created web session. Token goes in the cookie; only its SHA-256 is stored.
type WebSession struct {
	Token     string
	ExpiresAt time.Time
}

// SessionRecord is what a session lookup finds: the session and the member it belongs to.
type SessionRecord struct {
	MemberID      uuid.UUID
	DiscordUserID string
	Status        MemberStatus
	ViewAsPlayer  bool
	ExpiresAt     time.Time
}

func newSessionToken() (string, []byte, error) {
	tokenBytes := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, err
	}
	tokenHash := sha256.Sum256(tokenBytes)
	return base64.RawURLEncoding.EncodeToString(tokenBytes), tokenHash[:], nil
}

// HashSessionToken accepts only what CreateSession hands out, so junk cookies never reach the
// database.
func HashSessionToken(token string) ([]byte, bool) {
	tokenBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(tokenBytes) != sessionTokenBytes {
		return nil, false
	}
	tokenHash := sha256.Sum256(tokenBytes)
	return tokenHash[:], true
}

// CreateSession stores a new web session for the member and returns its token.
func (service *Service) CreateSession(requestContext context.Context, memberID uuid.UUID) (WebSession, error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return WebSession{}, err
	}
	expiresAt := time.Now().UTC().Add(SessionLifetime)
	if _, err := service.queries.CreateSession(requestContext, store.CreateSessionParams{
		TokenHash: tokenHash,
		MemberID:  memberID,
		ExpiresAt: expiresAt,
	}); err != nil {
		return WebSession{}, err
	}
	// Sign-ins are rare, so this is a cheap moment to sweep sessions nobody will present again.
	if err := service.queries.DeleteExpiredSessions(requestContext); err != nil {
		service.logger.WarnContext(requestContext, "could not delete expired sessions", slog.Any("error", err))
	}
	return WebSession{Token: token, ExpiresAt: expiresAt}, nil
}

// LookupSession finds an unexpired session by its token hash; found is false when there is none. At
// most once a day it slides the expiry forward, and extended reports that, so the caller can refresh
// its cookie. That bookkeeping is logged when it fails and never fails the lookup.
func (service *Service) LookupSession(requestContext context.Context, tokenHash []byte) (record SessionRecord, found bool, extended bool, err error) {
	if service.queries == nil {
		return SessionRecord{}, false, false, nil
	}
	row, err := service.queries.GetSessionWithMember(requestContext, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionRecord{}, false, false, nil
	}
	if err != nil {
		return SessionRecord{}, false, false, err
	}
	record = SessionRecord{
		MemberID:      row.Member.ID,
		DiscordUserID: row.Member.DiscordUserID,
		Status:        row.Member.Status,
		ViewAsPlayer:  row.WebSession.ViewAsPlayer,
		ExpiresAt:     row.WebSession.ExpiresAt,
	}
	if time.Since(row.WebSession.LastSeenAt) >= sessionTouchInterval {
		record.ExpiresAt, extended = service.extendSession(requestContext, tokenHash, row.Member.ID, row.WebSession.ExpiresAt)
	}
	return record, true, extended, nil
}

// extendSession slides the session's expiry and records when the member was last seen. It returns the
// expiry in force afterwards and whether it moved.
func (service *Service) extendSession(requestContext context.Context, tokenHash []byte, memberID uuid.UUID, currentExpiry time.Time) (time.Time, bool) {
	expiresAt := time.Now().UTC().Add(SessionLifetime)
	if err := service.queries.TouchSession(requestContext, store.TouchSessionParams{ExpiresAt: expiresAt, TokenHash: tokenHash}); err != nil {
		service.logger.WarnContext(requestContext, "could not extend a session", slog.Any("error", err))
		return currentExpiry, false
	}
	if err := service.queries.TouchMemberLastSeen(requestContext, memberID); err != nil {
		service.logger.WarnContext(requestContext, "could not record when a member was last seen", slog.Any("error", err))
	}
	return expiresAt, true
}
