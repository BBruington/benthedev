package main

import (
	"io"
	"os"
	"testing"
)

// setEnvironment sets each variable for the test, or unsets it when the value is nil, and restores the
// author's environment afterwards (mise loads .env into every `mise exec`, so tests never rely on it).
func setEnvironment(t *testing.T, values map[string]*string) {
	t.Helper()
	for name, value := range values {
		t.Setenv(name, "")
		if value == nil {
			if err := os.Unsetenv(name); err != nil {
				t.Fatalf("unset %s: %v", name, err)
			}
			continue
		}
		t.Setenv(name, *value)
	}
}

func text(value string) *string {
	return &value
}

func baseEnvironment() map[string]*string {
	return map[string]*string{
		"APP_ENV":               text("test"),
		"DATABASE_URL":          text("postgres://colony@127.0.0.1:1/none"),
		"PORT":                  nil,
		"SITE_OWNER_DISCORD_ID": nil,
		"DISCORD_GUILD_ID":      nil,
	}
}

// `colony openapi` (mise run gen, and so mise run check) reads no environment variable and needs no
// database.
func TestOpenAPINeedsNoConfiguration(t *testing.T) {
	environment := baseEnvironment()
	environment["APP_ENV"] = nil
	environment["DATABASE_URL"] = nil
	setEnvironment(t, environment)
	if err := printOpenAPI(io.Discard); err != nil {
		t.Fatalf("colony openapi without configuration: %v", err)
	}
}

func TestSiteOwnerDiscordIDMustBeADiscordID(t *testing.T) {
	environment := baseEnvironment()
	environment["SITE_OWNER_DISCORD_ID"] = text("seed:dm")
	setEnvironment(t, environment)
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig accepted a SITE_OWNER_DISCORD_ID that is not a Discord user ID")
	}

	environment["SITE_OWNER_DISCORD_ID"] = text("123456789012345678")
	environment["DISCORD_GUILD_ID"] = text("123456789012345679")
	setEnvironment(t, environment)
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if loaded.siteOwnerDiscordID != "123456789012345678" || loaded.legacyDiscordGuildID != "123456789012345679" || loaded.port != "8080" {
		t.Fatalf("loaded = %+v", loaded)
	}
}
