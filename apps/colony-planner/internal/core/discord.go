package core

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/BBruington/colony-planner/internal/store"
)

// DiscordDirectory is core's one outward seam for reading Discord: Beny Bot implements it from its
// gateway state. Core uses it to accept only channels of a campaign's linked server. TextChannels
// returns ErrDiscordListUnavailable (possibly wrapped) when it can't tell what the server's channels are.
type DiscordDirectory interface {
	TextChannels(requestContext context.Context, guildID string) ([]DiscordChannel, error)
}

// ErrDiscordListUnavailable means the linked server's channel list can't be read right now: Beny Bot
// hasn't heard about the server since it connected, Discord reports it unavailable, or Beny Bot is no
// longer in it. An empty list would instead say the server has no text channels, and Settings would
// then refuse even the channels it already stores.
var ErrDiscordListUnavailable = errors.New("discord: the linked server's channel list is unavailable")

// DiscordChannel is one text channel of a linked server, for the Settings channel picker.
type DiscordChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LinkedDiscordServer is the Settings card's "Linked to <server name>".
type LinkedDiscordServer struct {
	_         struct{}  `nullable:"true"`
	GuildID   string    `json:"guildId"`
	GuildName string    `json:"guildName"`
	LinkedAt  time.Time `json:"linkedAt"`
}

// LinkCode is returned only by the DM operation that creates it.
type LinkCode struct {
	Code      string    `json:"code" doc:"Shown as BENY-XXXXX-XXXXX; run /link code:<it> in the server"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// LinkGuildInput is what Beny Bot's /link hands core. The bot has already checked Manage Server.
type LinkGuildInput struct {
	Code          string
	GuildID       string
	GuildName     string
	DiscordUserID string
}

// LinkedGuild is /link's answer: "Linked this server to <campaign name>."
type LinkedGuild struct {
	CampaignID   uuid.UUID
	CampaignName string
}

// DiscordTarget is what Beny Bot needs to post for one campaign: its linked server and channels.
// Unset channels are nil, and the bot skips a post whose channel is unset or not in GuildID.
type DiscordTarget struct {
	CampaignSlug      string
	GuildID           string
	AnnounceChannelID *string
	ReminderChannelID *string
	DmChannelID       *string
}

var (
	ErrLinkCodeInvalid = &Error{Kind: KindInvalid, Message: "That code doesn't work. Get a new one in your campaign's Settings."}
	// ErrGuildAlreadyLinked never names the other campaign.
	ErrGuildAlreadyLinked    = &Error{Kind: KindConflict, Message: "This server is already linked to a campaign. Its DM can unlink it in Settings, or run /unlink here."}
	ErrCampaignAlreadyLinked = &Error{Kind: KindConflict, Message: "Unlink the other server in Settings first."}
)

// CreateLinkCode gives the DM a new 15-minute, single-use code for /link, replacing any older one.
func (service *Service) CreateLinkCode(requestContext context.Context, viewer Viewer) (LinkCode, error) {
	if err := requireDm(viewer); err != nil {
		return LinkCode{}, err
	}
	code, expiresAt, err := service.linkCodes.issue(viewer.campaignID)
	if err != nil {
		return LinkCode{}, err
	}
	return LinkCode{Code: formatLinkCode(code), ExpiresAt: expiresAt}, nil
}

// LinkGuild is Beny Bot's /link: the code is the DM's consent, Manage Server (checked by the bot) the
// server's. A server links to one campaign and a campaign to one server; linking clears the campaign's
// channels, which belonged to no server or another one. Only a link that is made uses the code up, so
// after any refusal (this server or the campaign already linked, a paused campaign) the same code works
// again until it expires. Pausing or deleting a campaign still forgets its code (linkCodeStore.drop).
func (service *Service) LinkGuild(requestContext context.Context, input LinkGuildInput) (LinkedGuild, error) {
	campaignID, live := service.linkCodes.lookup(input.Code)
	if !live {
		return LinkedGuild{}, ErrLinkCodeInvalid
	}
	guildName, fieldError := cleanText("guildName", input.GuildName, 100)
	if fieldError != nil || guildName == "" {
		guildName = "Discord server"
	}
	var linked LinkedGuild
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		campaign, err := queries.GetCampaignForUpdate(requestContext, campaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLinkCodeInvalid
		}
		if err != nil {
			return err
		}
		if campaign.Status != store.CampaignStatusACTIVE {
			return ErrCampaignSuspended
		}
		if _, err := queries.GetGuildLinkByGuild(requestContext, input.GuildID); err == nil {
			return ErrGuildAlreadyLinked
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := queries.GetGuildLink(requestContext, campaignID); err == nil {
			return ErrCampaignAlreadyLinked
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := queries.CreateGuildLink(requestContext, store.CreateGuildLinkParams{
			CampaignID:            campaignID,
			GuildID:               input.GuildID,
			GuildName:             guildName,
			LinkedByDiscordUserID: input.DiscordUserID,
		}); err != nil {
			var postgresError *pgconn.PgError
			if errors.As(err, &postgresError) && postgresError.Code == "23505" {
				return ErrGuildAlreadyLinked
			}
			if errors.As(err, &postgresError) && postgresError.Code == "23514" {
				return Invalid("That isn't a Discord server.")
			}
			return err
		}
		if err := queries.ClearSettingsChannels(requestContext, campaignID); err != nil {
			return err
		}
		// Claimed last, under the campaign's row lock: two /link runs with one code take that lock in
		// turn, so the second finds the campaign linked or the code gone. Only a failed commit after this
		// uses a code up without a link; the DM then makes a new one.
		if !service.linkCodes.claim(input.Code, campaignID) {
			return ErrLinkCodeInvalid
		}
		linked = LinkedGuild{CampaignID: campaignID, CampaignName: campaign.Name}
		return nil
	})
	if err != nil {
		return LinkedGuild{}, err
	}
	service.logger.InfoContext(requestContext, "discord server linked", slog.String("campaignId", campaignID.String()), slog.String("guildId", input.GuildID))
	service.broker.Publish(campaignID, TopicSettings)
	return linked, nil
}

// UnlinkGuild is the DM's Unlink on Settings.
func (service *Service) UnlinkGuild(requestContext context.Context, viewer Viewer) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	return service.unlinkCampaign(requestContext, viewer.campaignID)
}

func (service *Service) unlinkCampaign(requestContext context.Context, campaignID uuid.UUID) error {
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.DeleteGuildLink(requestContext, campaignID); err != nil {
			return err
		}
		return queries.ClearSettingsChannels(requestContext, campaignID)
	})
	if err != nil {
		return err
	}
	service.broker.Publish(campaignID, TopicSettings)
	return nil
}

// UnlinkGuildByID unlinks a server from whatever campaign it is linked to: /unlink run there by a
// Manage Server member, or Beny Bot removed from it (GuildDelete). Unknown servers are ignored.
func (service *Service) UnlinkGuildByID(requestContext context.Context, guildID string) (bool, error) {
	var campaignID uuid.UUID
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		var err error
		campaignID, err = queries.DeleteGuildLinkByGuild(requestContext, guildID)
		if err != nil {
			return err
		}
		return queries.ClearSettingsChannels(requestContext, campaignID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	service.logger.InfoContext(requestContext, "discord server unlinked", slog.String("campaignId", campaignID.String()), slog.String("guildId", guildID))
	service.broker.Publish(campaignID, TopicSettings)
	return true, nil
}

// ReconcileGuilds runs on every Ready: a linked server missing from Beny Bot's guild list removed the
// bot while it was offline, so it is unlinked. It returns how many were unlinked. A Ready that lists
// no servers at all while some are linked is taken as incomplete, not as the bot having been removed
// everywhere, so it unlinks nothing. Should the bot really be in no server, posts there are skipped
// anyway (the bot can't see their channels) and each DM can unlink in Settings.
func (service *Service) ReconcileGuilds(requestContext context.Context, presentGuildIDs []string) (int, error) {
	links, err := service.queries.ListGuildLinks(requestContext)
	if err != nil {
		return 0, err
	}
	if len(presentGuildIDs) == 0 && len(links) > 0 {
		service.logger.WarnContext(requestContext, "Ready listed no Discord servers; kept every link", slog.Int("linked", len(links)))
		return 0, nil
	}
	present := make(map[string]bool, len(presentGuildIDs))
	for _, guildID := range presentGuildIDs {
		present[guildID] = true
	}
	unlinked := 0
	for _, link := range links {
		if present[link.GuildID] {
			continue
		}
		if removed, err := service.UnlinkGuildByID(requestContext, link.GuildID); err != nil {
			return unlinked, err
		} else if removed {
			unlinked++
		}
	}
	return unlinked, nil
}

// DiscordChannelOptions lists the linked server's text channels for the Settings picker; empty when
// no server is linked, Beny Bot is off or the list can't be read right now (the picker then says so).
func (service *Service) DiscordChannelOptions(requestContext context.Context, viewer Viewer) ([]DiscordChannel, error) {
	if err := requireDm(viewer); err != nil {
		return nil, err
	}
	link, err := optionalGuildLink(service.queries.GetGuildLink(requestContext, viewer.campaignID))
	if err != nil || link == nil || service.directory == nil {
		return []DiscordChannel{}, err
	}
	channels, err := service.directory.TextChannels(requestContext, link.GuildID)
	if errors.Is(err, ErrDiscordListUnavailable) {
		return []DiscordChannel{}, nil
	}
	if err != nil {
		return nil, err
	}
	if channels == nil {
		channels = []DiscordChannel{}
	}
	return channels, nil
}

// DiscordTargetFor is an internal read for Beny Bot's postToCampaign; it is not exposed over HTTP.
// found is false when the campaign has no linked server or is suspended: then nothing is posted.
func (service *Service) DiscordTargetFor(requestContext context.Context, campaignID uuid.UUID) (target DiscordTarget, found bool, err error) {
	campaign, err := service.queries.GetCampaign(requestContext, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DiscordTarget{}, false, nil
	}
	if err != nil {
		return DiscordTarget{}, false, err
	}
	if campaign.Status != store.CampaignStatusACTIVE {
		return DiscordTarget{}, false, nil
	}
	link, err := optionalGuildLink(service.queries.GetGuildLink(requestContext, campaignID))
	if err != nil || link == nil {
		return DiscordTarget{}, false, err
	}
	settings, err := service.queries.GetSettings(requestContext, campaignID)
	if err != nil {
		return DiscordTarget{}, false, campaignGone(err)
	}
	return DiscordTarget{
		CampaignSlug:      campaign.Slug,
		GuildID:           link.GuildID,
		AnnounceChannelID: settings.DiscordAnnounceChannelID,
		ReminderChannelID: settings.DiscordReminderChannelID,
		DmChannelID:       settings.DiscordDmChannelID,
	}, true, nil
}
