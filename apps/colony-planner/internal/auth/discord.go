package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"golang.org/x/oauth2"

	"github.com/BBruington/colony-planner/internal/core"
)

const oauthStateLifetime = 10 * time.Minute

// Where a failed sign-in lands; /login explains each error value.
const (
	loginPageDiscordFailed = "/login?error=discord"
)

var discordSnowflake = regexp.MustCompile(`^[0-9]{1,20}$`)

// DiscordLogin starts Discord OAuth2 with the `identify` scope only (GET /auth/discord/login, public).
// PKCE isn't relied on: the state cookie and the client secret protect the exchange (spec §8.5).
func (service *Service) DiscordLogin() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !service.DiscordEnabled() {
			WriteProblem(writer, http.StatusNotFound, "Discord sign-in is not set up on this server.")
			return
		}
		stateBytes := make([]byte, 32)
		if _, err := rand.Read(stateBytes); err != nil {
			WriteProblem(writer, http.StatusInternalServerError, "Something went wrong.")
			return
		}
		state := base64.RawURLEncoding.EncodeToString(stateBytes)
		http.SetCookie(writer, &http.Cookie{
			Name:     service.oauthStateCookieName,
			Value:    state,
			Path:     "/",
			MaxAge:   int(oauthStateLifetime.Seconds()),
			HttpOnly: true,
			Secure:   service.secureCookies,
			// Lax, so the cookie comes back on Discord's top-level redirect to the callback.
			SameSite: http.SameSiteLaxMode,
		})
		writer.Header().Set("Cache-Control", "no-store")
		// prompt=none skips Discord's consent screen for someone who has already authorized the app.
		authorizeURL := service.discordOAuth.AuthCodeURL(state, oauth2.SetAuthURLParam("prompt", "none"))
		http.Redirect(writer, request, authorizeURL, http.StatusFound)
	})
}

// DiscordCallback finishes Discord OAuth2 (GET /auth/discord/callback, public): it checks the state,
// exchanges the code, reads GET /users/@me, discards Discord's token, records the sign-in and starts a
// session.
func (service *Service) DiscordCallback() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !service.DiscordEnabled() {
			WriteProblem(writer, http.StatusNotFound, "Discord sign-in is not set up on this server.")
			return
		}
		requestContext := request.Context()
		writer.Header().Set("Cache-Control", "no-store")
		stateCookie, cookieError := request.Cookie(service.oauthStateCookieName)
		http.SetCookie(writer, &http.Cookie{
			Name:     service.oauthStateCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   service.secureCookies,
			SameSite: http.SameSiteLaxMode,
		})
		query := request.URL.Query()
		returnedState := query.Get("state")
		if cookieError != nil || stateCookie.Value == "" || returnedState == "" ||
			subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(returnedState)) != 1 {
			service.options.Logger.WarnContext(requestContext, "Discord sign-in refused: the state does not match")
			http.Redirect(writer, request, loginPageDiscordFailed, http.StatusSeeOther)
			return
		}
		if discordError := query.Get("error"); discordError != "" || query.Get("code") == "" {
			service.options.Logger.InfoContext(requestContext, "Discord sign-in was not completed", slog.String("discord_error", discordError))
			http.Redirect(writer, request, loginPageDiscordFailed, http.StatusSeeOther)
			return
		}

		identity, err := service.discordIdentity(requestContext, query.Get("code"))
		if err != nil {
			service.options.Logger.ErrorContext(requestContext, "Discord sign-in failed", slog.Any("error", err))
			http.Redirect(writer, request, loginPageDiscordFailed, http.StatusSeeOther)
			return
		}
		// A first sign-in only creates the account: joining is by invite link, so nobody is turned away here.
		accountID, err := service.options.Core.SignIn(requestContext, identity)
		if err != nil {
			service.options.Logger.ErrorContext(requestContext, "recording a Discord sign-in failed", slog.Any("error", err))
			http.Redirect(writer, request, loginPageDiscordFailed, http.StatusSeeOther)
			return
		}
		if err := service.startSession(writer, request, accountID); err != nil {
			service.options.Logger.ErrorContext(requestContext, "starting a session failed", slog.Any("error", err))
			http.Redirect(writer, request, loginPageDiscordFailed, http.StatusSeeOther)
			return
		}
		http.Redirect(writer, request, "/", http.StatusSeeOther)
	})
}

type discordUser struct {
	ID         string  `json:"id"`
	Username   string  `json:"username"`
	GlobalName *string `json:"global_name"`
	Avatar     *string `json:"avatar"`
}

// discordIdentity exchanges the code and asks Discord who signed in. The access token lives only in
// this function: nothing stores it.
func (service *Service) discordIdentity(requestContext context.Context, code string) (core.DiscordIdentity, error) {
	requestContext = context.WithValue(requestContext, oauth2.HTTPClient, service.httpClient)
	token, err := service.discordOAuth.Exchange(requestContext, code)
	if err != nil {
		return core.DiscordIdentity{}, fmt.Errorf("exchange the code: %w", err)
	}
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, service.discordUserURL, nil)
	if err != nil {
		return core.DiscordIdentity{}, err
	}
	token.SetAuthHeader(request)
	response, err := service.httpClient.Do(request)
	if err != nil {
		return core.DiscordIdentity{}, fmt.Errorf("read the Discord user: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return core.DiscordIdentity{}, fmt.Errorf("read the Discord user: status %d", response.StatusCode)
	}
	var user discordUser
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&user); err != nil {
		return core.DiscordIdentity{}, fmt.Errorf("decode the Discord user: %w", err)
	}
	if !discordSnowflake.MatchString(user.ID) || user.Username == "" {
		return core.DiscordIdentity{}, errors.New("the Discord user has no ID or username")
	}
	identity := core.DiscordIdentity{UserID: user.ID, Username: user.Username, DisplayName: user.Username}
	if user.GlobalName != nil && *user.GlobalName != "" {
		identity.DisplayName = *user.GlobalName
	}
	if user.Avatar != nil && *user.Avatar != "" {
		avatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png?size=128", user.ID, *user.Avatar)
		identity.AvatarURL = &avatarURL
	}
	return identity, nil
}
