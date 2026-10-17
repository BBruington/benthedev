package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireKnownHost(t *testing.T) {
	service, err := New(Options{AppEnvironment: "development", PublicURL: "https://Colony.Example"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler := service.RequireKnownHost(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	for host, status := range map[string]int{
		"localhost:5173":               http.StatusNoContent,
		"Localhost":                    http.StatusNoContent,
		"127.0.0.1:8080":               http.StatusNoContent,
		"[::1]:8080":                   http.StatusNoContent,
		"::1":                          http.StatusNoContent,
		"10.0.0.7:5173":                http.StatusNoContent,
		"colony.example":               http.StatusNoContent,
		"colony.example:443":           http.StatusNoContent,
		"rebind.attacker.example:8080": http.StatusMisdirectedRequest,
		"colony.example.attacker.test": http.StatusMisdirectedRequest,
		"localhost.attacker.example":   http.StatusMisdirectedRequest,
		"":                             http.StatusMisdirectedRequest,
	} {
		request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		request.Host = host
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != status {
			t.Errorf("Host %q = %d, want %d", host, recorder.Code, status)
		}
	}
}

func TestPublicURLMustBeAnHTTPAddress(t *testing.T) {
	for _, testCase := range []struct {
		appEnvironment string
		publicURL      string
		allowed        bool
	}{
		{"development", "http://localhost:5173", true},
		{"development", "https://colony.example", true},
		{"development", "", true},
		{"development", "localhost:5173", false},
		{"development", "ftp://colony.example", false},
		{"test", "http://", false},
		{"production", "http://colony.example", false},
		{"production", "https://", false},
		{"production", "", false},
	} {
		err := CheckPublicURL(testCase.publicURL, testCase.appEnvironment)
		if (err == nil) != testCase.allowed {
			t.Errorf("%s with PUBLIC_URL %q: allowed = %v, want %v (%v)", testCase.appEnvironment, testCase.publicURL, err == nil, testCase.allowed, err)
		}
	}
}
