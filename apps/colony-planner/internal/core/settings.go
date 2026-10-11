package core

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// Settings is one campaign's settings. Only its DM reads or writes them, through Settings. Every
// member reads the campaign and colony names on the board (ColonyBoard).
type Settings struct {
	// CampaignName is directory data on the campaign row: the site owner sees it.
	CampaignName string `json:"campaignName"`
	// ColonyName is stored on the colony row, but the DM edits it here with the rest of the campaign.
	ColonyName               string  `json:"colonyName"`
	Timezone                 string  `json:"timezone" doc:"IANA time zone, e.g. America/New_York"`
	DiscordAnnounceChannelID *string `json:"discordAnnounceChannelId"`
	DiscordReminderChannelID *string `json:"discordReminderChannelId"`
	DiscordDmChannelID       *string `json:"discordDmChannelId"`
	// BotInviteURL is not stored: it comes from DISCORD_CLIENT_ID, and inputs never carry it.
	BotInviteURL *string `json:"botInviteUrl" doc:"Adds Beny Bot to a Discord server; null until DISCORD_CLIENT_ID is set"`
	// DiscordServer is the linked Discord server, or null when none is linked.
	DiscordServer *LinkedDiscordServer `json:"discordServer" doc:"null when no Discord server is linked"`
}

// SettingsInput replaces every setting at once (PUT semantics); a null or blank channel clears it.
// UpdateSettings checks every field and owns the messages, so there are no schema length or pattern
// tags: they would answer first, in the schema validator's words. A channel must be a text channel of
// the linked Discord server.
type SettingsInput struct {
	CampaignName             string  `json:"campaignName" doc:"1 to 80 characters after trimming; the site owner sees it"`
	ColonyName               string  `json:"colonyName" doc:"1 to 80 characters after trimming"`
	Timezone                 string  `json:"timezone" doc:"IANA time zone, e.g. America/New_York"`
	DiscordAnnounceChannelID *string `json:"discordAnnounceChannelId" doc:"A text channel of the linked Discord server"`
	DiscordReminderChannelID *string `json:"discordReminderChannelId" doc:"A text channel of the linked Discord server"`
	DiscordDmChannelID       *string `json:"discordDmChannelId" doc:"A text channel of the linked Discord server"`
}

const (
	colonyNameMaxLength = 80
	timezoneMaxLength   = 64
)

// A Discord snowflake is 17 to 20 digits today.
var discordChannelIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)

func (service *Service) toSettings(row store.Settings, campaignName string, colonyName string, link *store.DiscordGuildLink) Settings {
	settings := Settings{
		CampaignName:             campaignName,
		ColonyName:               colonyName,
		Timezone:                 row.Timezone,
		DiscordAnnounceChannelID: row.DiscordAnnounceChannelID,
		DiscordReminderChannelID: row.DiscordReminderChannelID,
		DiscordDmChannelID:       row.DiscordDmChannelID,
	}
	if service.botInviteURL != "" {
		inviteURL := service.botInviteURL
		settings.BotInviteURL = &inviteURL
	}
	if link != nil {
		settings.DiscordServer = &LinkedDiscordServer{GuildID: link.GuildID, GuildName: link.GuildName, LinkedAt: link.LinkedAt.UTC()}
	}
	return settings
}

// readSettings reads everything Settings shows, with the given queries.
func (service *Service) readSettings(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) (Settings, error) {
	row, err := queries.GetSettings(requestContext, campaignID)
	if err != nil {
		return Settings{}, campaignGone(err)
	}
	campaign, err := queries.GetCampaign(requestContext, campaignID)
	if err != nil {
		return Settings{}, campaignGone(err)
	}
	colony, err := queries.GetColony(requestContext, campaignID)
	if err != nil {
		return Settings{}, campaignGone(err)
	}
	link, err := optionalGuildLink(queries.GetGuildLink(requestContext, campaignID))
	if err != nil {
		return Settings{}, err
	}
	return service.toSettings(row, campaign.Name, colony.Name, link), nil
}

func optionalGuildLink(link store.DiscordGuildLink, err error) (*store.DiscordGuildLink, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (service *Service) GetSettings(requestContext context.Context, viewer Viewer) (Settings, error) {
	if err := requireDm(viewer); err != nil {
		return Settings{}, err
	}
	return service.readSettings(requestContext, service.queries, viewer.campaignID)
}

func (service *Service) UpdateSettings(requestContext context.Context, viewer Viewer, input SettingsInput) (Settings, error) {
	if err := requireDm(viewer); err != nil {
		return Settings{}, err
	}
	var fieldErrors []FieldError
	campaignName, fieldError := cleanText("campaignName", input.CampaignName, campaignNameMaxLength)
	switch {
	case fieldError != nil:
		fieldErrors = append(fieldErrors, *fieldError)
	case campaignName == "":
		fieldErrors = append(fieldErrors, FieldError{Field: "campaignName", Message: "Give the campaign a name."})
	}
	colonyName, fieldError := cleanText("colonyName", input.ColonyName, colonyNameMaxLength)
	switch {
	case fieldError != nil:
		fieldErrors = append(fieldErrors, *fieldError)
	case colonyName == "":
		fieldErrors = append(fieldErrors, FieldError{Field: "colonyName", Message: "Give the colony a name."})
	}
	timezone, fieldError := cleanText("timezone", input.Timezone, timezoneMaxLength)
	// time.LoadLocation also accepts "" and "Local", which mean this machine's zone, not an IANA name.
	if fieldError == nil {
		if _, err := time.LoadLocation(timezone); err != nil || timezone == "" || timezone == "Local" {
			fieldError = &FieldError{Field: "timezone", Message: "Use an IANA time zone, such as America/New_York."}
		}
	}
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	announceChannelID, announceError := discordChannelID("discordAnnounceChannelId", input.DiscordAnnounceChannelID)
	reminderChannelID, reminderError := discordChannelID("discordReminderChannelId", input.DiscordReminderChannelID)
	dmChannelID, dmChannelError := discordChannelID("discordDmChannelId", input.DiscordDmChannelID)
	for _, channelError := range []*FieldError{announceError, reminderError, dmChannelError} {
		if channelError != nil {
			fieldErrors = append(fieldErrors, *channelError)
		}
	}
	if len(fieldErrors) > 0 {
		return Settings{}, Invalid("Some settings need fixing.", fieldErrors...)
	}
	channels := map[string]*string{
		"discordAnnounceChannelId": announceChannelID,
		"discordReminderChannelId": reminderChannelID,
		"discordDmChannelId":       dmChannelID,
	}
	var settings Settings
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		current, err := queries.GetSettings(requestContext, viewer.campaignID)
		if err != nil {
			return campaignGone(err)
		}
		link, err := optionalGuildLink(queries.GetGuildLink(requestContext, viewer.campaignID))
		if err != nil {
			return err
		}
		stored := map[string]*string{
			"discordAnnounceChannelId": current.DiscordAnnounceChannelID,
			"discordReminderChannelId": current.DiscordReminderChannelID,
			"discordDmChannelId":       current.DiscordDmChannelID,
		}
		if channelErrors := service.checkChannels(requestContext, link, channels, stored); len(channelErrors) > 0 {
			return Invalid("Some settings need fixing.", channelErrors...)
		}
		if _, err := queries.UpdateSettings(requestContext, store.UpdateSettingsParams{
			CampaignID:               viewer.campaignID,
			Timezone:                 timezone,
			DiscordAnnounceChannelID: announceChannelID,
			DiscordReminderChannelID: reminderChannelID,
			DiscordDmChannelID:       dmChannelID,
		}); err != nil {
			return err
		}
		if _, err := queries.SetCampaignName(requestContext, store.SetCampaignNameParams{CampaignID: viewer.campaignID, Name: campaignName}); err != nil {
			return campaignGone(err)
		}
		// A setting, not a colony mutation: the version and the undo slot stay as they are.
		if _, err := queries.SetColonyName(requestContext, store.SetColonyNameParams{CampaignID: viewer.campaignID, Name: colonyName}); err != nil {
			return campaignGone(err)
		}
		settings, err = service.readSettings(requestContext, queries, viewer.campaignID)
		return err
	})
	if err != nil {
		return Settings{}, err
	}
	// The board carries both names, so every member's top bar refreshes too.
	service.broker.Publish(viewer.campaignID, TopicSettings, TopicColony, TopicMe)
	return settings, nil
}

// checkChannels accepts a channel only from the linked server's text channels, so a DM can never make
// Beny Bot post into someone else's server. When that list is unknown (Beny Bot is off, or Discord's
// list can't be read right now), the stored channels are accepted unchanged, so the DM can still save
// the rest of Settings, and only a change is refused.
func (service *Service) checkChannels(requestContext context.Context, link *store.DiscordGuildLink, channels map[string]*string, stored map[string]*string) []FieldError {
	var fieldErrors []FieldError
	var allowed map[string]bool
	listUnreadable := false
	for _, field := range slices.Sorted(maps.Keys(channels)) {
		channelID := channels[field]
		if channelID == nil {
			continue
		}
		if link == nil {
			fieldErrors = append(fieldErrors, FieldError{Field: field, Message: "Link a Discord server first."})
			continue
		}
		unchanged := stored[field] != nil && *stored[field] == *channelID
		if service.directory == nil {
			if !unchanged {
				fieldErrors = append(fieldErrors, FieldError{Field: field, Message: "Beny Bot is off, so channels can't be checked. Try again when it is on."})
			}
			continue
		}
		if allowed == nil {
			allowed = map[string]bool{}
			textChannels, err := service.directory.TextChannels(requestContext, link.GuildID)
			if err != nil {
				listUnreadable = true
				service.logger.WarnContext(requestContext, "could not list a linked server's channels", slog.String("guildId", link.GuildID), slog.Any("error", err))
			}
			for _, channel := range textChannels {
				allowed[channel.ID] = true
			}
		}
		if listUnreadable {
			if !unchanged {
				fieldErrors = append(fieldErrors, FieldError{Field: field, Message: "Discord's channel list couldn't be read right now. Try again in a moment."})
			}
			continue
		}
		if !allowed[*channelID] {
			fieldErrors = append(fieldErrors, FieldError{Field: field, Message: "Pick a text channel of the linked server."})
		}
	}
	return fieldErrors
}

// discordChannelID trims a channel ID; null or blank clears the channel.
func discordChannelID(field string, value *string) (*string, *FieldError) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if !discordChannelIDPattern.MatchString(trimmed) {
		return nil, &FieldError{Field: field, Message: "Pick a text channel of the linked server."}
	}
	return &trimmed, nil
}
