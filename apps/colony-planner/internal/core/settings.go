package core

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BBruington/colony-planner/internal/store"
)

// Settings is the campaign singleton. Only the DM reads or writes it, through /settings.
type Settings struct {
	CampaignName             string  `json:"campaignName"`
	Timezone                 string  `json:"timezone" doc:"IANA time zone, e.g. America/New_York"`
	DiscordAnnounceChannelID *string `json:"discordAnnounceChannelId"`
	DiscordReminderChannelID *string `json:"discordReminderChannelId"`
	DiscordDmChannelID       *string `json:"discordDmChannelId"`
	// BotInviteURL is not stored: it comes from DISCORD_CLIENT_ID, and inputs never carry it.
	BotInviteURL *string `json:"botInviteUrl" doc:"Adds Beny Bot to a Discord server; null until DISCORD_CLIENT_ID is set"`
}

// SettingsInput replaces every setting at once (PUT semantics); a null channel clears it.
type SettingsInput struct {
	CampaignName             string  `json:"campaignName" minLength:"1" maxLength:"80"`
	Timezone                 string  `json:"timezone" minLength:"1" maxLength:"64" doc:"IANA time zone, e.g. America/New_York"`
	DiscordAnnounceChannelID *string `json:"discordAnnounceChannelId" pattern:"^[0-9]{17,20}$"`
	DiscordReminderChannelID *string `json:"discordReminderChannelId" pattern:"^[0-9]{17,20}$"`
	DiscordDmChannelID       *string `json:"discordDmChannelId" pattern:"^[0-9]{17,20}$"`
}

// DiscordChannels is what Beny Bot needs from settings to post. Unset channels are nil, and the bot
// skips a post whose channel is unset.
type DiscordChannels struct {
	AnnounceChannelID *string
	ReminderChannelID *string
	DmChannelID       *string
}

const campaignNameMaxLength = 80

func (service *Service) toSettings(row store.Settings) Settings {
	settings := Settings{
		CampaignName:             row.CampaignName,
		Timezone:                 row.Timezone,
		DiscordAnnounceChannelID: row.DiscordAnnounceChannelID,
		DiscordReminderChannelID: row.DiscordReminderChannelID,
		DiscordDmChannelID:       row.DiscordDmChannelID,
	}
	if service.botInviteURL != "" {
		inviteURL := service.botInviteURL
		settings.BotInviteURL = &inviteURL
	}
	return settings
}

func (service *Service) GetSettings(requestContext context.Context, viewer Viewer) (Settings, error) {
	if err := requireDm(viewer); err != nil {
		return Settings{}, err
	}
	row, err := service.queries.GetSettings(requestContext)
	if err != nil {
		return Settings{}, err
	}
	return service.toSettings(row), nil
}

func (service *Service) UpdateSettings(requestContext context.Context, viewer Viewer, input SettingsInput) (Settings, error) {
	if err := requireDm(viewer); err != nil {
		return Settings{}, err
	}
	campaignName := strings.TrimSpace(input.CampaignName)
	timezone := strings.TrimSpace(input.Timezone)
	var fieldErrors []FieldError
	if campaignName == "" {
		fieldErrors = append(fieldErrors, FieldError{Field: "campaignName", Message: "Give the campaign a name."})
	} else if utf8.RuneCountInString(campaignName) > campaignNameMaxLength {
		fieldErrors = append(fieldErrors, FieldError{Field: "campaignName", Message: "Use at most 80 characters."})
	}
	// time.LoadLocation also accepts "" and "Local", which mean this machine's zone, not an IANA name.
	if _, err := time.LoadLocation(timezone); err != nil || timezone == "" || timezone == "Local" {
		fieldErrors = append(fieldErrors, FieldError{Field: "timezone", Message: "Use an IANA time zone, such as America/New_York."})
	}
	if len(fieldErrors) > 0 {
		return Settings{}, Invalid("Some settings need fixing.", fieldErrors...)
	}
	row, err := service.queries.UpdateSettings(requestContext, store.UpdateSettingsParams{
		CampaignName:             campaignName,
		Timezone:                 timezone,
		DiscordAnnounceChannelID: blankToNil(input.DiscordAnnounceChannelID),
		DiscordReminderChannelID: blankToNil(input.DiscordReminderChannelID),
		DiscordDmChannelID:       blankToNil(input.DiscordDmChannelID),
	})
	if err != nil {
		return Settings{}, err
	}
	service.broker.Publish(TopicSettings)
	return service.toSettings(row), nil
}

func blankToNil(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// DiscordChannels is an internal read for Beny Bot; it is not exposed over HTTP.
func (service *Service) DiscordChannels(requestContext context.Context) (DiscordChannels, error) {
	settings, err := service.queries.GetSettings(requestContext)
	if err != nil {
		return DiscordChannels{}, err
	}
	return DiscordChannels{
		AnnounceChannelID: settings.DiscordAnnounceChannelID,
		ReminderChannelID: settings.DiscordReminderChannelID,
		DmChannelID:       settings.DiscordDmChannelID,
	}, nil
}
