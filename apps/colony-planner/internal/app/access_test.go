package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	authService, err := auth.New(auth.Options{AppEnvironment: "test", PublicURL: "http://localhost:5173"})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	server, err := New(Options{Core: core.New(core.Options{}), Auth: authService, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	return server
}

func TestEveryOperationDeclaresAnAccessLevel(t *testing.T) {
	server := newTestServer(t)
	for path, pathItem := range server.OpenAPI().Paths {
		for method, operation := range pathOperations(pathItem) {
			if !operationAccess(operation).valid() {
				t.Errorf("%s %s has no valid %s", method, path, accessExtension)
			}
		}
	}
}

func TestAccessCheckRejectsUndeclaredRoutes(t *testing.T) {
	api := humago.New(http.NewServeMux(), humaConfig())
	huma.Register(api, huma.Operation{OperationID: "sneaky", Method: http.MethodGet, Path: "/api/sneaky"},
		func(context.Context, *struct{}) (*struct{}, error) { return &struct{}{}, nil })
	routes := []Route{{Pattern: "GET /img/{id}", Handler: http.NotFoundHandler()}}

	err := checkAccessDeclarations(api, routes)
	if err == nil {
		t.Fatal("expected the access check to fail")
	}
	for _, expected := range []string{"GET /api/sneaky", "GET /img/{id}"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not name %q", err, expected)
		}
	}
}

func TestAccessDecision(t *testing.T) {
	pending := core.Viewer{MemberID: uuid.New(), Status: store.MemberStatusPENDING}
	removed := core.Viewer{MemberID: uuid.New(), Status: store.MemberStatusREMOVED}
	player := core.Viewer{MemberID: uuid.New(), Status: store.MemberStatusACTIVE}
	dm := core.Viewer{MemberID: uuid.New(), Status: store.MemberStatusACTIVE, IsDM: true}
	dmViewingAsPlayer := core.Viewer{MemberID: uuid.New(), Status: store.MemberStatusACTIVE, IsDM: true, ViewAsPlayer: true}

	for _, testCase := range []struct {
		name     string
		level    AccessLevel
		viewer   core.Viewer
		signedIn bool
		status   int
	}{
		{"public admits visitors", AccessPublic, core.Viewer{}, false, 0},
		{"token admits visitors", AccessToken, core.Viewer{}, false, 0},
		{"pending refuses visitors", AccessPending, core.Viewer{}, false, http.StatusUnauthorized},
		{"pending admits pending", AccessPending, pending, true, 0},
		{"pending admits removed", AccessPending, removed, true, 0},
		{"member refuses visitors", AccessMember, core.Viewer{}, false, http.StatusUnauthorized},
		{"member refuses pending", AccessMember, pending, true, http.StatusForbidden},
		{"member refuses removed", AccessMember, removed, true, http.StatusForbidden},
		{"member admits players", AccessMember, player, true, 0},
		{"member admits the DM viewing as player", AccessMember, dmViewingAsPlayer, true, 0},
		{"dm refuses visitors", AccessDm, core.Viewer{}, false, http.StatusUnauthorized},
		{"dm refuses players", AccessDm, player, true, http.StatusForbidden},
		{"dm refuses the DM viewing as player", AccessDm, dmViewingAsPlayer, true, http.StatusForbidden},
		{"dm admits the DM", AccessDm, dm, true, 0},
		{"a missing level fails closed", AccessLevel(""), dm, true, http.StatusInternalServerError},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			status, _ := accessDecision(testCase.level, testCase.viewer, testCase.signedIn)
			if status != testCase.status {
				t.Fatalf("status = %d, want %d", status, testCase.status)
			}
		})
	}
}

func TestServerRoutes(t *testing.T) {
	handler := newTestServer(t).Handler()

	for _, testCase := range []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		headers     map[string]string
		status      int
		contains    string
		notContains string
	}{
		{name: "health check is public", method: http.MethodGet, path: "/healthz", status: http.StatusOK, contains: "ok"},
		{name: "me needs a session", method: http.MethodGet, path: "/api/me", status: http.StatusUnauthorized},
		{name: "members need a session", method: http.MethodGet, path: "/api/members", status: http.StatusUnauthorized},
		{name: "the OpenAPI document is dm-only", method: http.MethodGet, path: "/api/openapi.json", status: http.StatusUnauthorized},
		{name: "the docs page is dm-only", method: http.MethodGet, path: "/api/docs", status: http.StatusUnauthorized},
		{name: "huma's default docs route is off", method: http.MethodGet, path: "/docs", status: http.StatusOK, notContains: "elements-api"},
		{name: "huma's default OpenAPI route is off", method: http.MethodGet, path: "/openapi.json", status: http.StatusOK, notContains: `"openapi"`},
		{name: "the stream needs a session", method: http.MethodGet, path: "/api/stream", status: http.StatusUnauthorized},
		{name: "unknown API paths are 404", method: http.MethodGet, path: "/api/nothing-here", status: http.StatusNotFound},
		{name: "client-side routes get the SPA shell", method: http.MethodGet, path: "/players", status: http.StatusOK, contains: "<html"},
		{name: "logout needs a session", method: http.MethodPost, path: "/auth/logout", status: http.StatusUnauthorized},
		{
			name: "bodies must be JSON", method: http.MethodPut, path: "/api/me/profile",
			contentType: "text/plain", body: "characterName=Thorin", status: http.StatusUnsupportedMediaType,
		},
		{
			name: "cross-origin mutations are refused", method: http.MethodPost, path: "/auth/logout",
			headers: map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"},
			status:  http.StatusForbidden,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			request.Host = testHost
			if testCase.contentType != "" {
				request.Header.Set("Content-Type", testCase.contentType)
			}
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d; body: %s", recorder.Code, testCase.status, recorder.Body.String())
			}
			if testCase.contains != "" && !strings.Contains(recorder.Body.String(), testCase.contains) {
				t.Fatalf("body does not contain %q: %s", testCase.contains, recorder.Body.String())
			}
			if testCase.notContains != "" && strings.Contains(recorder.Body.String(), testCase.notContains) {
				t.Fatalf("body contains %q: %s", testCase.notContains, recorder.Body.String())
			}
		})
	}
}
