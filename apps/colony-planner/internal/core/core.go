// Package core holds every operation as a plain Go method that takes a context, a Caller (account and
// owner operations) or a Viewer (campaign operations) and an input struct, and returns a DTO or a
// typed error. It knows nothing about HTTP, so the huma handlers, Beny Bot and the seed all call the
// same functions (spec §8.6).
package core

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BBruington/colony-planner/internal/store"
)

type Options struct {
	// Pool is nil when the binary only exports the OpenAPI document.
	Pool     *pgxpool.Pool
	Notifier Notifier
	// RequestNotifier posts patron requests to the DM's Discord channel; nil while Beny Bot is off.
	RequestNotifier RequestNotifier
	Logger          *slog.Logger
	// SiteOwnerDiscordID is the effective site owner: SITE_OWNER_DISCORD_ID, or the seeded seed:dm
	// account in devauth builds when it is unset. Being owner confers nothing inside any campaign.
	SiteOwnerDiscordID string
	// Sessions is the session cache that internal/auth keeps. Core evicts from it whenever an operation
	// changes an account's sessions; it lives behind an interface because auth imports core.
	Sessions SessionEvictor
	// Directory lists a linked Discord server's text channels (Beny Bot's gateway state); nil while
	// Beny Bot is off.
	Directory DiscordDirectory
	// BotInviteURL adds Beny Bot to a Discord server; it is empty without DISCORD_CLIENT_ID.
	BotInviteURL string
	// PublicURL is the site's own address; a map's external art may not point at this host (C6).
	PublicURL string
	// Now is the clock for link-code expiry; nil means time.Now. Tests move it.
	Now func() time.Time
}

// SessionEvictor drops cached session lookups so the next request reads the database again.
type SessionEvictor interface {
	EvictSession(tokenHash []byte)
	EvictAccount(accountID uuid.UUID)
	EvictAll()
}

type noopSessionEvictor struct{}

func (noopSessionEvictor) EvictSession([]byte)    {}
func (noopSessionEvictor) EvictAccount(uuid.UUID) {}
func (noopSessionEvictor) EvictAll()              {}

type Service struct {
	pool               *pgxpool.Pool
	queries            *store.Queries
	notifier           Notifier
	requestNotifier    RequestNotifier
	logger             *slog.Logger
	siteOwnerDiscordID string
	sessions           SessionEvictor
	directory          DiscordDirectory
	broker             *Broker
	botInviteURL       string
	siteHost           string
	linkCodes          *linkCodeStore
}

func New(options Options) *Service {
	service := &Service{
		pool:               options.Pool,
		notifier:           options.Notifier,
		requestNotifier:    options.RequestNotifier,
		logger:             options.Logger,
		siteOwnerDiscordID: options.SiteOwnerDiscordID,
		sessions:           options.Sessions,
		directory:          options.Directory,
		broker:             NewBroker(),
		botInviteURL:       options.BotInviteURL,
		siteHost:           siteHostOf(options.PublicURL),
		linkCodes:          newLinkCodeStore(options.Now),
	}
	if options.Pool != nil {
		service.queries = store.New(options.Pool)
	}
	if service.notifier == nil {
		service.notifier = NoopNotifier{}
	}
	if service.requestNotifier == nil {
		service.requestNotifier = NoopRequestNotifier{}
	}
	if service.logger == nil {
		service.logger = slog.Default()
	}
	if service.sessions == nil {
		service.sessions = noopSessionEvictor{}
	}
	return service
}

// siteHostOf is PUBLIC_URL's host name in lower case, or empty when it is unset or unparsable.
func siteHostOf(publicURL string) string {
	parsed, err := url.Parse(publicURL)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
}

// SetDirectory attaches Beny Bot's channel directory once the bot exists (cmd/colony builds core
// first, because the bot needs it).
func (service *Service) SetDirectory(directory DiscordDirectory) {
	service.directory = directory
}

// SetRequestNotifier attaches Beny Bot's request prompts once the bot exists, like SetDirectory.
func (service *Service) SetRequestNotifier(requestNotifier RequestNotifier) {
	service.requestNotifier = requestNotifier
}

// inTransaction runs work in one transaction; it commits when work returns nil.
func (service *Service) inTransaction(requestContext context.Context, work func(queries *store.Queries) error) error {
	return pgx.BeginFunc(requestContext, service.pool, func(transaction pgx.Tx) error {
		return work(service.queries.WithTx(transaction))
	})
}

// IsSiteOwner is the one place the site owner is recognized: by Discord ID, on every call, never from
// storage. The auth middleware copies it into Caller.IsSiteOwner.
func (service *Service) IsSiteOwner(discordUserID string) bool {
	return service.siteOwnerDiscordID != "" && discordUserID == service.siteOwnerDiscordID
}

// requireCampaign refuses the zero Viewer and a suspended campaign. Every campaign operation except
// the two pending-level reads (and leaving) starts with it.
func requireCampaign(viewer Viewer) error {
	if viewer.campaignID == uuid.Nil {
		return ErrNoSuchCampaign
	}
	if !viewer.CampaignActive() {
		return ErrCampaignSuspended
	}
	return nil
}

// requireDm guards DM-only operations for callers that bypass the HTTP access levels (Beny Bot, tests);
// view as player counts as a player.
func requireDm(viewer Viewer) error {
	if err := requireCampaign(viewer); err != nil {
		return err
	}
	if !viewer.IsDM() {
		return Forbidden("Only the DM can do that.")
	}
	if viewer.viewAsPlayer {
		return Forbidden("View as player is on.")
	}
	return nil
}

func requireActive(viewer Viewer) error {
	if err := requireCampaign(viewer); err != nil {
		return err
	}
	if !viewer.IsActive() {
		return Forbidden("The DM hasn't let you in.")
	}
	return nil
}

// requireMembership admits any membership whatever its status, even in a suspended campaign: the
// pending-level reads, so the SPA can show the right screen.
func requireMembership(viewer Viewer) error {
	if viewer.campaignID == uuid.Nil {
		return ErrNoSuchCampaign
	}
	return nil
}

func requireSignedIn(caller Caller) error {
	if caller.AccountID == uuid.Nil {
		return ErrSessionEnded
	}
	return nil
}

func requireSiteOwner(caller Caller) error {
	if err := requireSignedIn(caller); err != nil {
		return err
	}
	if !caller.IsSiteOwner {
		return Forbidden("Only the site owner can do that.")
	}
	return nil
}

// campaignGone maps a campaign row (colony, settings) that vanished between EnterCampaign and the
// read, because the campaign was deleted, to ErrNoSuchCampaign rather than a 500.
func campaignGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoSuchCampaign
	}
	return err
}

// sessionEnded answers a caller whose own account row is gone. Accounts are never deleted, so this
// happens only when a local database is reset under a running server (`mise run reset`). Every cached
// session then predates the reset, so the whole cache goes, and each browser's next request reads the
// database, finds no session and is signed out instead of acting on rows that no longer exist.
func (service *Service) sessionEnded() error {
	service.sessions.EvictAll()
	return ErrSessionEnded
}
