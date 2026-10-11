package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"slices"
)

// config is every environment variable the binary reads; .env.example lists the same names. mise
// loads .env for local work, so nothing here parses files.
type config struct {
	appEnvironment      string
	databaseURL         string
	publicURL           string
	port                string
	discordClientID     string
	discordClientSecret string
	discordBotToken     string
	// legacyDiscordGuildID is read only for Beny Bot's one-time clean-up of the old guild-scoped
	// commands (docs/discord-dev-setup.md); then the author deletes it from .env.
	legacyDiscordGuildID string
	siteOwnerDiscordID   string
}

// errLegacyDmDiscordID stops every command that touches the database or serves while the M1 variable is
// still in .env: migration 0003 makes the site owner the DM of the first campaign, so the owner must be
// named by SITE_OWNER_DISCORD_ID (docs/design/multi-campaign.md §2.4).
var errLegacyDmDiscordID = errors.New("DM_DISCORD_ID is gone: rename it to SITE_OWNER_DISCORD_ID in .env (keep the value). " +
	"`mise run env:upgrade` renames it for you without printing it. Keep DISCORD_GUILD_ID until Beny Bot has run once " +
	"and logged \"old guild commands cleared\", then delete it (docs/discord-dev-setup.md)")

var discordUserIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)

var appEnvironments = []string{"development", "test", "production"}

func loadConfig() (config, error) {
	loaded := config{
		appEnvironment:       os.Getenv("APP_ENV"),
		databaseURL:          os.Getenv("DATABASE_URL"),
		publicURL:            os.Getenv("PUBLIC_URL"),
		port:                 os.Getenv("PORT"),
		discordClientID:      os.Getenv("DISCORD_CLIENT_ID"),
		discordClientSecret:  os.Getenv("DISCORD_CLIENT_SECRET"),
		discordBotToken:      os.Getenv("DISCORD_BOT_TOKEN"),
		legacyDiscordGuildID: os.Getenv("DISCORD_GUILD_ID"),
		siteOwnerDiscordID:   os.Getenv("SITE_OWNER_DISCORD_ID"),
	}
	if _, legacySet := os.LookupEnv("DM_DISCORD_ID"); legacySet {
		return config{}, errLegacyDmDiscordID
	}
	if loaded.siteOwnerDiscordID != "" && !discordUserIDPattern.MatchString(loaded.siteOwnerDiscordID) {
		return config{}, errors.New("SITE_OWNER_DISCORD_ID must be a Discord user ID: 17 to 20 digits")
	}
	if !slices.Contains(appEnvironments, loaded.appEnvironment) {
		return config{}, fmt.Errorf("APP_ENV must be one of %v (run `mise run setup` to create .env)", appEnvironments)
	}
	if loaded.databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is not set (run `mise run setup` to create .env)")
	}
	if loaded.port == "" {
		loaded.port = "8080"
	}
	return loaded, nil
}

// listenAddress keeps development servers on the loopback interface: a devauth build lets anyone
// sign in as anyone, so it must not be reachable from the network except through Vite (dev:phone).
func (loaded config) listenAddress() string {
	if loaded.appEnvironment == "production" {
		return ":" + loaded.port
	}
	return "127.0.0.1:" + loaded.port
}

func newLogger(appEnvironment string) *slog.Logger {
	if appEnvironment == "production" {
		return slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}
