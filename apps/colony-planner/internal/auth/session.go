package auth

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// startSession signs the browser in as the account, replacing any session the request carried.
func (service *Service) startSession(writer http.ResponseWriter, request *http.Request, accountID uuid.UUID) error {
	requestContext := request.Context()
	if previous, signedIn := CallerFromContext(requestContext); signedIn {
		if err := service.options.Core.SignOut(requestContext, previous); err != nil {
			service.options.Logger.WarnContext(requestContext, "could not end the previous session", slog.Any("error", err))
		}
	}
	session, err := service.options.Core.CreateSession(requestContext, accountID)
	if err != nil {
		return err
	}
	service.setSessionCookie(writer, session.Token, session.ExpiresAt)
	return nil
}

func (service *Service) setSessionCookie(writer http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(writer, &http.Cookie{
		Name:     service.sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   service.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (service *Service) clearSessionCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{
		Name:     service.sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   service.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// Middleware resolves the session cookie to a Caller and stores it in the request context. Requests
// without a valid session carry no Caller; the access layer in internal/app then answers 401. A
// cookie that names no session is cleared so the browser stops sending it.
func (service *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(service.sessionCookieName)
		if err != nil || cookie.Value == "" || service.options.Core == nil {
			next.ServeHTTP(writer, request)
			return
		}
		tokenHash, valid := core.HashSessionToken(cookie.Value)
		if !valid {
			service.clearSessionCookie(writer)
			next.ServeHTTP(writer, request)
			return
		}
		caller, found, err := service.resolveSession(request.Context(), writer, cookie.Value, tokenHash)
		if err != nil {
			// Failing closed without clearing the cookie: a database blip must not sign anyone out.
			service.options.Logger.ErrorContext(request.Context(), "session lookup failed", slog.Any("error", err))
			WriteProblem(writer, http.StatusServiceUnavailable, "The server can't check your session right now. Try again in a moment.")
			return
		}
		if !found {
			service.clearSessionCookie(writer)
			next.ServeHTTP(writer, request)
			return
		}
		next.ServeHTTP(writer, request.WithContext(WithCaller(request.Context(), caller)))
	})
}

func (service *Service) resolveSession(requestContext context.Context, writer http.ResponseWriter, token string, tokenHash []byte) (core.Caller, bool, error) {
	now := time.Now()
	entry, cached := service.cache.lookup(tokenHash, now)
	if !cached {
		generation := service.cache.currentGeneration()
		record, found, extended, err := service.options.Core.LookupSession(requestContext, tokenHash)
		if err != nil || !found {
			return core.Caller{}, false, err
		}
		if extended {
			service.setSessionCookie(writer, token, record.ExpiresAt)
		}
		entry = cachedSession{
			accountID:     record.AccountID,
			discordUserID: record.DiscordUserID,
			viewAsPlayer:  record.ViewAsPlayer,
			expiresAt:     record.ExpiresAt,
			cachedAt:      now,
		}
		service.cache.store(tokenHash, entry, generation)
	}
	// The site owner is derived on every request, never cached or stored.
	return core.Caller{
		AccountID:        entry.accountID,
		DiscordUserID:    entry.discordUserID,
		IsSiteOwner:      service.options.Core.IsSiteOwner(entry.discordUserID),
		SessionTokenHash: tokenHash,
		ViewAsPlayer:     entry.viewAsPlayer,
	}, true, nil
}

// Logout ends the caller's current session (POST /auth/logout, account level).
func (service *Service) Logout() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		caller, _ := CallerFromContext(request.Context())
		if err := service.options.Core.SignOut(request.Context(), caller); err != nil {
			service.options.Logger.ErrorContext(request.Context(), "sign out failed", slog.Any("error", err))
			WriteProblem(writer, http.StatusInternalServerError, "Something went wrong.")
			return
		}
		service.clearSessionCookie(writer)
		writer.WriteHeader(http.StatusNoContent)
	})
}

// LogoutEverywhere ends every session of the caller (POST /auth/logout-everywhere, account level).
func (service *Service) LogoutEverywhere() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		caller, _ := CallerFromContext(request.Context())
		if err := service.options.Core.SignOutEverywhere(request.Context(), caller); err != nil {
			service.options.Logger.ErrorContext(request.Context(), "sign out everywhere failed", slog.Any("error", err))
			WriteProblem(writer, http.StatusInternalServerError, "Something went wrong.")
			return
		}
		service.clearSessionCookie(writer)
		writer.WriteHeader(http.StatusNoContent)
	})
}
