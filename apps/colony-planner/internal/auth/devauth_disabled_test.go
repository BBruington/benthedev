//go:build !devauth

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProductionBuildHasNoDevSignIn(t *testing.T) {
	service, err := New(Options{AppEnvironment: "production", PublicURL: "https://colony.example"})
	if err != nil {
		t.Fatalf("a production build refused APP_ENV=production: %v", err)
	}
	if service.DevSignIn() != nil {
		t.Fatal("a production build offers /auth/dev")
	}
	if DevAuthEnabled {
		t.Fatal("DevAuthEnabled is true without the devauth tag")
	}
	if EffectiveDmDiscordID("") != "" {
		t.Fatal("a production build made up a DM")
	}
}

// Over plain http the session cookie would lose Secure and its __Host- prefix (spec §8.5).
func TestProductionRefusesAPlainHTTPPublicURL(t *testing.T) {
	for _, publicURL := range []string{"http://colony.example", "https://", "colony.example", ""} {
		if _, err := New(Options{AppEnvironment: "production", PublicURL: publicURL}); err == nil {
			t.Errorf("a production build accepted PUBLIC_URL %q", publicURL)
		}
	}
	service, err := New(Options{AppEnvironment: "production", PublicURL: "https://colony.example"})
	if err != nil {
		t.Fatalf("a production build refused an https PUBLIC_URL: %v", err)
	}
	if service.SessionCookieName() != "__Host-colony_session" || !service.secureCookies {
		t.Fatalf("production cookie = %q, Secure = %v", service.SessionCookieName(), service.secureCookies)
	}
}

// Production answers whatever Host the platform routes to it; DNS rebinding gains nothing there.
func TestProductionAcceptsAnyHost(t *testing.T) {
	service, err := New(Options{AppEnvironment: "production", PublicURL: "https://colony.example"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler := service.RequireKnownHost(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Host = "colony-production.up.railway.app"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("production refused Host %q: %d", request.Host, recorder.Code)
	}
}
