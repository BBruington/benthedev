package core

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/BBruington/colony-planner/internal/store"
)

// Settings is the campaign singleton. Only the DM reads or writes it, through /settings. Every member
// reads the campaign and colony names on the board (ColonyBoard).
type Settings struct {
	CampaignName string `json:"campaignName"`
	// ColonyName is stored on the colony row, but the DM edits it here with the rest of the campaign.
	ColonyName               string  `json:"colonyName"`
	Timezone                 string  `json:"timezone" doc:"IANA time zone, e.g. America/New_York"`
	DiscordAnnounceChannelID *string `json:"discordAnnounceChannelId"`
	DiscordReminderChannelID *string `json:"discordReminderChannelId"`
	DiscordDmChannelID       *string `json:"discordDmChannelId"`
	// BotInviteURL is not stored: it comes from DISCORD_CLIENT_ID, and inputs never carry it.
	BotInviteURL *string `json:"botInviteUrl" doc:"Adds Beny Bot to a Discord server; null until DISCORD_CLIENT_ID is set"`
}

// SettingsInput replaces every setting at once (PUT semantics); a null or blank channel clears it.
// UpdateSettings checks every field and owns the messages, so there are no schema length or pattern
// tags: they would answer first, in the schema validator's words.
type SettingsInput struct {
	CampaignName             string  `json:"campaignName" doc:"1 to 80 characters after trimming"`
	ColonyName               string  `json:"colonyName" doc:"1 to 80 characters after trimming"`
	Timezone                 string  `json:"timezone" doc:"IANA time zone, e.g. America/New_York"`
	DiscordAnnounceChannelID *string `json:"discordAnnounceChannelId" doc:"Discord channel ID: 17 to 20 digits"`
	DiscordReminderChannelID *string `json:"discordReminderChannelId" doc:"Discord channel ID: 17 to 20 digits"`
	DiscordDmChannelID       *string `json:"discordDmChannelId" doc:"Discord channel ID: 17 to 20 digits"`
}

// DiscordChannels is what Beny Bot needs from settings to post. Unset channels are nil, and the bot
// skips a post whose channel is unset.
type DiscordChannels struct {
	AnnounceChannelID *string
	ReminderChannelID *string
	DmChannelID       *string
}

const (
	campaignNameMaxLength = 80
	colonyNameMaxLength   = 80
	timezoneMaxLength     = 64
)

// A Discord snowflake is 17 to 20 digits today.
var discordChannelIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)

func (service *Service) toSettings(row store.Settings, colonyName string) Settings {
	settings := Settings{
		CampaignName:             row.CampaignName,
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
	colony, err := service.queries.GetColony(requestContext)
	if err != nil {
		return Settings{}, err
	}
	return service.toSettings(row, colony.Name), nil
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
	var row store.Settings
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		var err error
		row, err = queries.UpdateSettings(requestContext, store.UpdateSettingsParams{
			CampaignName:             campaignName,
			Timezone:                 timezone,
			DiscordAnnounceChannelID: announceChannelID,
			DiscordReminderChannelID: reminderChannelID,
			DiscordDmChannelID:       dmChannelID,
		})
		if err != nil {
			return err
		}
		// A setting, not a colony mutation: the version and the undo slot stay as they are.
		_, err = queries.SetColonyName(requestContext, colonyName)
		return err
	})
	if err != nil {
		return Settings{}, err
	}
	// The board carries both names, so every member's top bar refreshes too.
	service.broker.Publish(TopicSettings, TopicColony)
	return service.toSettings(row, colonyName), nil
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
		return nil, &FieldError{Field: field, Message: "A channel ID is 17 to 20 digits. Copy it from Discord as described above."}
	}
	return &trimmed, nil
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
