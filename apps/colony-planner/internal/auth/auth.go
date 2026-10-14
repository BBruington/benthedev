// Package auth signs people in (Discord OAuth2, or dev sign-in in devauth builds), carries the session
// token in a cookie, caches session lookups in memory, and resolves every request to a core.Caller.
// It talks only to internal/core: core owns the member and session data (spec §8.6).
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/BBruington/colony-planner/internal/core"
)

const defaultDiscordBaseURL = "https://discord.com"

type Options struct {
	Core   *core.Service
	Logger *slog.Logger
	// SessionCache is shared with core, which evicts from it; New makes one when it is nil.
	SessionCache *SessionCache
	// PublicURL is the address the browser uses; its scheme picks the cookie names.
	PublicURL           string
	AppEnvironment      string
	DiscordClientID     string
	DiscordClientSecret string
	// SiteOwnerDiscordID is already resolved by EffectiveSiteOwnerDiscordID.
	SiteOwnerDiscordID string
	// DiscordBaseURL replaces https://discord.com in tests, which run a fake Discord.
	DiscordBaseURL string
	// HTTPClient makes the calls to Discord; the default gives up after 10 seconds.
	HTTPClient *http.Client
}

type Service struct {
	options              Options
	cache                *SessionCache
	secureCookies        bool
	publicHost           string
	sessionCookieName    string
	oauthStateCookieName string
	discordOAuth         *oauth2.Config
	discordUserURL       string
	httpClient           *http.Client
}

// CheckBuild refuses a devauth build in production. serve calls it before touching the database; New
// checks again so no caller can skip it.
func CheckBuild(appEnvironment string) error {
	return checkDevAuthAllowed(appEnvironment)
}

// CheckPublicURL refuses a PUBLIC_URL the session cookie can't rely on; serve calls it before touching
// the database, and New checks again.
func CheckPublicURL(publicURL string, appEnvironment string) error {
	_, err := parsePublicURL(publicURL, appEnvironment)
	return err
}

func New(options Options) (*Service, error) {
	if err := CheckBuild(options.AppEnvironment); err != nil {
		return nil, err
	}
	publicURL, err := parsePublicURL(options.PublicURL, options.AppEnvironment)
	if err != nil {
		return nil, err
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.SessionCache == nil {
		options.SessionCache = NewSessionCache()
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	discordBaseURL := strings.TrimSuffix(options.DiscordBaseURL, "/")
	if discordBaseURL == "" {
		discordBaseURL = defaultDiscordBaseURL
	}
	service := &Service{
		options:              options,
		cache:                options.SessionCache,
		secureCookies:        publicURL.Scheme == "https",
		publicHost:           strings.ToLower(publicURL.Hostname()),
		sessionCookieName:    cookieName(publicURL, "colony_session"),
		oauthStateCookieName: cookieName(publicURL, "oauth_state"),
		// Sign-in asks for the `identify` scope only: there is no Discord server check (F4).
		discordOAuth: &oauth2.Config{
			ClientID:     options.DiscordClientID,
			ClientSecret: options.DiscordClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:   discordBaseURL + "/oauth2/authorize",
				TokenURL:  discordBaseURL + "/api/oauth2/token",
				AuthStyle: oauth2.AuthStyleInHeader,
			},
			RedirectURL: publicURL.JoinPath("auth", "discord", "callback").String(),
			Scopes:      []string{"identify"},
		},
		discordUserURL: discordBaseURL + "/api/v10/users/@me",
		httpClient:     options.HTTPClient,
	}
	return service, nil
}

// HTTPSOnly reports whether PUBLIC_URL is https, which production requires.
func (service *Service) HTTPSOnly() bool {
	return service.secureCookies
}

// parsePublicURL accepts an http or https address with a host. Production must use https: over plain
// http the session cookie would lose its Secure flag and __Host- prefix without a word (spec §8.5). An
// empty PUBLIC_URL is allowed outside production for `colony openapi`, which serves nothing; serve
// requires it.
func parsePublicURL(publicURL string, appEnvironment string) (*url.URL, error) {
	parsed, err := url.Parse(publicURL)
	if err != nil {
		return nil, fmt.Errorf("auth: PUBLIC_URL is not a URL: %w", err)
	}
	if appEnvironment == "production" {
		if parsed.Scheme != "https" || parsed.Host == "" {
			return nil, errors.New("auth: with APP_ENV=production, PUBLIC_URL must be an https address such as https://colony.example.com")
		}
		return parsed, nil
	}
	if publicURL == "" {
		return parsed, nil
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("auth: PUBLIC_URL must be an http or https address such as http://localhost:5173, not %q", publicURL)
	}
	return parsed, nil
}

// cookieName uses the __Host- prefix whenever the site is served over https. Browsers refuse
// __Host- cookies without Secure, so plain-http local development uses the unprefixed name.
func cookieName(publicURL *url.URL, name string) string {
	if publicURL.Scheme == "https" {
		return "__Host-" + name
	}
	return name
}

// DiscordEnabled reports whether "Sign in with Discord" is offered; without Beny Bot Dev's OAuth2 values
// the button is hidden and dev sign-in replaces it.
func (service *Service) DiscordEnabled() bool {
	return service.options.DiscordClientID != "" && service.options.DiscordClientSecret != ""
}

// SessionCookieName is colony_session over http and __Host-colony_session over https.
func (service *Service) SessionCookieName() string {
	return service.sessionCookieName
}

type callerContextKey struct{}

// WithCaller stores the signed-in Caller. A campaign Viewer is not auth's to build: internal/app gets
// one from core.EnterCampaign for the request's {campaignId}.
func WithCaller(requestContext context.Context, caller core.Caller) context.Context {
	return context.WithValue(requestContext, callerContextKey{}, caller)
}

func CallerFromContext(requestContext context.Context) (core.Caller, bool) {
	caller, ok := requestContext.Value(callerContextKey{}).(core.Caller)
	return caller, ok
}
