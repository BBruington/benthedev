package main

import (
	"errors"
	"io"
	"os"
	"strings"
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
		"DM_DISCORD_ID":         nil,
		"SITE_OWNER_DISCORD_ID": nil,
		"DISCORD_GUILD_ID":      nil,
	}
}

// Every command that touches the database or serves refuses while the M1 variable is set, and says
// exactly what to do, without echoing the value.
func TestLegacyDmDiscordIDIsRefused(t *testing.T) {
	const legacyValue = "123456789012345678"
	for name, value := range map[string]string{"a value": legacyValue, "an empty line": ""} {
		t.Run(name, func(t *testing.T) {
			environment := baseEnvironment()
			environment["DM_DISCORD_ID"] = text(value)
			setEnvironment(t, environment)
			_, err := loadConfig()
			if !errors.Is(err, errLegacyDmDiscordID) {
				t.Fatalf("loadConfig with DM_DISCORD_ID set = %v; want the rename message", err)
			}
			for _, instruction := range []string{"SITE_OWNER_DISCORD_ID", "keep the value", "mise run env:upgrade", "DISCORD_GUILD_ID"} {
				if !strings.Contains(err.Error(), instruction) {
					t.Errorf("the refusal doesn't mention %q: %v", instruction, err)
				}
			}
			if strings.Contains(err.Error(), legacyValue) {
				t.Error("the refusal printed the value")
			}
			// The subcommands that refuse do so before anything else, the database included.
			for _, subcommand := range [][]string{{"serve"}, {"migrate", "up"}, {"seed"}, {"reset"}} {
				if err := run(subcommand); !errors.Is(err, errLegacyDmDiscordID) {
					t.Errorf("colony %s = %v; want the rename message", strings.Join(subcommand, " "), err)
				}
			}
		})
	}
}

// `colony openapi` (mise run gen, and so mise run check) needs no configuration and must keep working
// with the author's old .env loaded.
func TestOpenAPIIgnoresTheLegacyVariable(t *testing.T) {
	environment := baseEnvironment()
	environment["DM_DISCORD_ID"] = text("123456789012345678")
	environment["DISCORD_GUILD_ID"] = text("123456789012345679")
	setEnvironment(t, environment)
	if err := printOpenAPI(io.Discard); err != nil {
		t.Fatalf("colony openapi with DM_DISCORD_ID set: %v", err)
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
