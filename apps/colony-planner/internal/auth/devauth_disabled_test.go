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
	if EffectiveSiteOwnerDiscordID("") != "" {
		t.Fatal("a production build made up a site owner")
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

// Production answers only PUBLIC_URL's host, and Railway's deploy health check only on /healthz.
func TestProductionAnswersOnlyPublicURLsHost(t *testing.T) {
	service, err := New(Options{AppEnvironment: "production", PublicURL: "https://Colony.Example"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler := service.RequireKnownHost(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	for _, testCase := range []struct {
		method, host, path string
		status             int
	}{
		{http.MethodGet, "colony.example", "/api/me", http.StatusNoContent},
		{http.MethodGet, "COLONY.EXAMPLE:443", "/healthz", http.StatusNoContent},
		{http.MethodPost, "colony.example", "/auth/logout", http.StatusNoContent},
		{http.MethodGet, "healthcheck.railway.app", "/healthz", http.StatusNoContent},
		{http.MethodGet, "healthcheck.railway.app", "/api/me", http.StatusMisdirectedRequest},
		{http.MethodGet, "healthcheck.railway.app", "/", http.StatusMisdirectedRequest},
		{http.MethodPost, "healthcheck.railway.app", "/healthz", http.StatusMisdirectedRequest},
		{http.MethodGet, "colony-production.up.railway.app", "/", http.StatusMisdirectedRequest},
		{http.MethodGet, "colony-production.up.railway.app", "/healthz", http.StatusMisdirectedRequest},
		{http.MethodGet, "localhost:8080", "/api/me", http.StatusMisdirectedRequest},
		{http.MethodGet, "10.0.0.7", "/healthz", http.StatusMisdirectedRequest},
		{http.MethodGet, "colony.example.attacker.test", "/", http.StatusMisdirectedRequest},
		{http.MethodGet, "", "/", http.StatusMisdirectedRequest},
	} {
		request := httptest.NewRequest(testCase.method, testCase.path, nil)
		request.Host = testCase.host
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != testCase.status {
			t.Errorf("%s %s with Host %q = %d, want %d", testCase.method, testCase.path, testCase.host, recorder.Code, testCase.status)
		}
	}
}
