package main

import (
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
	setEnvironment(t, environment)
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if loaded.siteOwnerDiscordID != "123456789012345678" || loaded.port != "8080" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

// M1's DISCORD_GUILD_ID is no longer read, so an .env that still has it starts the same as one without.
func TestAnOldDiscordGuildIDIsIgnored(t *testing.T) {
	environment := baseEnvironment()
	setEnvironment(t, environment)
	without, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig without DISCORD_GUILD_ID: %v", err)
	}
	environment["DISCORD_GUILD_ID"] = text("123456789012345679")
	setEnvironment(t, environment)
	with, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig with DISCORD_GUILD_ID: %v", err)
	}
	if with != without {
		t.Fatalf("DISCORD_GUILD_ID changed the configuration: %+v, want %+v", with, without)
	}
}

// Railway routes to $PORT on the container's interfaces, so production listens on all of them; anything
// else stays on the loopback interface, since a devauth build lets anyone sign in as anyone.
func TestProductionListensOnEveryInterfaceAtPort(t *testing.T) {
	environment := baseEnvironment()
	environment["PORT"] = text("31337")
	environment["DATABASE_URL"] = text("postgres://colony@127.0.0.1:1/none?sslmode=verify-full")
	for appEnvironment, address := range map[string]string{
		"production":  ":31337",
		"development": "127.0.0.1:31337",
		"test":        "127.0.0.1:31337",
	} {
		environment["APP_ENV"] = text(appEnvironment)
		setEnvironment(t, environment)
		loaded, err := loadConfig()
		if err != nil {
			t.Fatalf("loadConfig with APP_ENV=%s: %v", appEnvironment, err)
		}
		if got := loaded.listenAddress(); got != address {
			t.Errorf("APP_ENV=%s listens on %q, want %q", appEnvironment, got, address)
		}
	}
}

// Railway reaches Neon over the public internet, and pgx's sslmode=require accepts any certificate, so
// production starts only with a DATABASE_URL that checks the database's certificate and host name.
func TestProductionRefusesADatabaseURLThatSkipsTheCertificateCheck(t *testing.T) {
	const neon = "postgresql://neondb_owner:not-a-password@ep-example-123456.us-east-1.aws.neon.tech/colony"
	environment := baseEnvironment()
	environment["APP_ENV"] = text("production")
	for _, refused := range []string{
		neon,
		neon + "?sslmode=require",
		neon + "?sslmode=require&channel_binding=require",
		neon + "?sslmode=prefer",
		neon + "?sslmode=disable",
		neon + "?sslmode=verify-ca",
	} {
		environment["DATABASE_URL"] = text(refused)
		setEnvironment(t, environment)
		_, err := loadConfig()
		if err == nil {
			t.Errorf("production accepted %q", refused)
			continue
		}
		if strings.Contains(err.Error(), "not-a-password") {
			t.Errorf("the refusal shows the password: %v", err)
		}
	}
	for _, accepted := range []string{
		neon + "?sslmode=verify-full",
		neon + "?sslmode=verify-full&channel_binding=require",
	} {
		environment["DATABASE_URL"] = text(accepted)
		setEnvironment(t, environment)
		if _, err := loadConfig(); err != nil {
			t.Errorf("production refused %q: %v", accepted, err)
		}
	}

	environment["APP_ENV"] = text("development")
	environment["DATABASE_URL"] = text("postgres://colony@127.0.0.1:54329/colony?sslmode=disable")
	setEnvironment(t, environment)
	if _, err := loadConfig(); err != nil {
		t.Errorf("development refused the local container's DATABASE_URL: %v", err)
	}
}
