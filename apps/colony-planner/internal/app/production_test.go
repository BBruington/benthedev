//go:build !devauth

package app

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// The production build as Railway runs it, behind its HTTPS proxy: TLS ends at the proxy, so the
// request reaching Go is plain http and only PUBLIC_URL says the site is https. No request here reads
// the database, so core has none.
func TestProductionServer(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := core.New(core.Options{Logger: logger, PublicURL: "https://colony.example"})
	authService, err := auth.New(auth.Options{
		Core:                service,
		Logger:              logger,
		PublicURL:           "https://colony.example",
		AppEnvironment:      "production",
		DiscordClientID:     "123456789012345678",
		DiscordClientSecret: "not-a-secret",
		SiteOwnerDiscordID:  "123456789012345679",
	})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	server, err := New(Options{Core: service, Auth: authService, Logger: logger})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	get := func(host, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		return recorder
	}

	if response := get("colony.example", "/"); response.Header().Get("Strict-Transport-Security") != "max-age=31536000" {
		t.Errorf("Strict-Transport-Security = %q, want a year without includeSubDomains", response.Header().Get("Strict-Transport-Security"))
	}
	if response := get("colony.example", "/auth/dev?as=dm"); response.Code != http.StatusNotFound || len(response.Result().Cookies()) > 0 {
		t.Errorf("/auth/dev = %d with cookies %v, want a 404 and none", response.Code, response.Result().Cookies())
	}
	if response := get("healthcheck.railway.app", "/healthz"); response.Code != http.StatusOK || response.Body.String() != "ok\n" {
		t.Errorf("Railway's health check = %d %q", response.Code, response.Body.String())
	}
	for _, host := range []string{"colony-production.up.railway.app", "localhost:8080", "10.0.0.7:8080"} {
		if response := get(host, "/"); response.Code != http.StatusMisdirectedRequest {
			t.Errorf("Host %q = %d, want 421", host, response.Code)
		}
	}

	signIn := get("colony.example", "/auth/discord/login")
	if signIn.Code != http.StatusFound {
		t.Fatalf("/auth/discord/login = %d", signIn.Code)
	}
	cookies := signIn.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "__Host-oauth_state" || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Path != "/" || cookies[0].Domain != "" {
		t.Fatalf("sign-in state cookie = %+v, want one Secure, HttpOnly __Host- cookie on / with no Domain", cookies)
	}
	if location := signIn.Header().Get("Location"); !strings.Contains(location, "redirect_uri=https%3A%2F%2Fcolony.example%2Fauth%2Fdiscord%2Fcallback") {
		t.Fatalf("Discord redirect = %q, want PUBLIC_URL's https callback", location)
	}
}
