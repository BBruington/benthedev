// Package core holds every operation as a plain Go method that takes a context, the Viewer and an
// input struct, and returns a DTO or a typed error. It knows nothing about HTTP, so the huma
// handlers, Beny Bot and the seed all call the same functions (spec §8.6).
package core

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BBruington/colony-planner/internal/store"
)

type Options struct {
	// Pool is nil when the binary only exports the OpenAPI document.
	Pool     *pgxpool.Pool
	Notifier Notifier
	Logger   *slog.Logger
	// DmDiscordID is the effective DM identity: DM_DISCORD_ID, or the seeded DM in devauth builds.
	DmDiscordID string
	// Sessions is the session cache that internal/auth keeps. Core evicts from it whenever an operation
	// changes a member's status or sessions; it lives behind an interface because auth imports core.
	Sessions SessionEvictor
	// BotInviteURL adds Beny Bot to a Discord server; it is empty without DISCORD_CLIENT_ID.
	BotInviteURL string
}

// SessionEvictor drops cached session lookups so the next request reads the database again.
type SessionEvictor interface {
	EvictSession(tokenHash []byte)
	EvictMember(memberID uuid.UUID)
	EvictAll()
}

type noopSessionEvictor struct{}

func (noopSessionEvictor) EvictSession([]byte)   {}
func (noopSessionEvictor) EvictMember(uuid.UUID) {}
func (noopSessionEvictor) EvictAll()             {}

type Service struct {
	pool         *pgxpool.Pool
	queries      *store.Queries
	notifier     Notifier
	logger       *slog.Logger
	dmDiscordID  string
	sessions     SessionEvictor
	broker       *Broker
	botInviteURL string
}

func New(options Options) *Service {
	service := &Service{
		pool:         options.Pool,
		notifier:     options.Notifier,
		logger:       options.Logger,
		dmDiscordID:  options.DmDiscordID,
		sessions:     options.Sessions,
		broker:       NewBroker(),
		botInviteURL: options.BotInviteURL,
	}
	if options.Pool != nil {
		service.queries = store.New(options.Pool)
	}
	if service.notifier == nil {
		service.notifier = NoopNotifier{}
	}
	if service.logger == nil {
		service.logger = slog.Default()
	}
	if service.sessions == nil {
		service.sessions = noopSessionEvictor{}
	}
	return service
}

// inTransaction runs work in one transaction; it commits when work returns nil.
func (service *Service) inTransaction(requestContext context.Context, work func(queries *store.Queries) error) error {
	return pgx.BeginFunc(requestContext, service.pool, func(transaction pgx.Tx) error {
		return work(service.queries.WithTx(transaction))
	})
}

func (service *Service) isDM(discordUserID string) bool {
	return service.dmDiscordID != "" && discordUserID == service.dmDiscordID
}

// requireDm guards DM-only operations for callers that bypass the HTTP access levels (Beny Bot, tests);
// view as player counts as a player.
func requireDm(viewer Viewer) error {
	if !viewer.IsDM {
		return Forbidden("Only the DM can do that.")
	}
	if viewer.ViewAsPlayer {
		return Forbidden("View as player is on.")
	}
	return nil
}

// sessionEnded answers a caller whose own member row is gone. Members are never deleted, so this
// happens only when a local database is reset under a running server (`mise run reset`). Every cached
// session then predates the reset, so the whole cache goes, and each browser's next request reads the
// database, finds no session and is signed out instead of acting on rows that no longer exist.
func (service *Service) sessionEnded() error {
	service.sessions.EvictAll()
	return ErrSessionEnded
}

func requireActive(viewer Viewer) error {
	if !viewer.IsActive() {
		return Forbidden("The DM hasn't let you in.")
	}
	return nil
}
