package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

const testDmDiscordID = "200000000000000002"

type recordingNotifier struct {
	mutex   sync.Mutex
	waiting []core.WaitingMember
}

func (notifier *recordingNotifier) MemberWaiting(_ context.Context, member core.WaitingMember) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.waiting = append(notifier.waiting, member)
}

func (notifier *recordingNotifier) count() int {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	return len(notifier.waiting)
}

type authHarness struct {
	t        *testing.T
	database *dbtest.Database
	core     *core.Service
	auth     *Service
	notifier *recordingNotifier
}

func newAuthHarness(t *testing.T, options Options) *authHarness {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	cache := NewSessionCache()
	notifier := &recordingNotifier{}
	service := core.New(core.Options{
		Pool:        database.Pool,
		Notifier:    notifier,
		Logger:      logger,
		DmDiscordID: testDmDiscordID,
		Sessions:    cache,
	})
	options.Core = service
	options.Logger = logger
	options.SessionCache = cache
	options.DmDiscordID = testDmDiscordID
	if options.PublicURL == "" {
		options.PublicURL = "http://localhost:5173"
	}
	if options.AppEnvironment == "" {
		options.AppEnvironment = "test"
	}
	authService, err := New(options)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return &authHarness{t: t, database: database, core: service, auth: authService, notifier: notifier}
}

func (harness *authHarness) createMember(discordUserID, displayName string, status store.MemberStatus) store.Member {
	harness.t.Helper()
	member, err := store.New(harness.database.Pool).CreateMember(context.Background(), store.CreateMemberParams{
		DiscordUserID:   discordUserID,
		DiscordUsername: strings.ToLower(displayName),
		DisplayName:     displayName,
		Status:          status,
	})
	if err != nil {
		harness.t.Fatalf("create member: %v", err)
	}
	return member
}

// serve runs handler behind the session middleware, as the real server does.
func (harness *authHarness) serve(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	harness.auth.Middleware(handler).ServeHTTP(recorder, request)
	return recorder
}

// viewerFor resolves a session cookie the way every request does.
func (harness *authHarness) viewerFor(cookie *http.Cookie) (core.Viewer, bool) {
	harness.t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.AddCookie(cookie)
	var viewer core.Viewer
	var signedIn bool
	harness.serve(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		viewer, signedIn = ViewerFromContext(request.Context())
	}), request)
	return viewer, signedIn
}

func responseCookie(response *httptest.ResponseRecorder, name string) *http.Cookie {
	var found *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			found = cookie
		}
	}
	return found
}

func (harness *authHarness) countMembers() int64 {
	harness.t.Helper()
	count, err := store.New(harness.database.Pool).CountMembers(context.Background())
	if err != nil {
		harness.t.Fatalf("count members: %v", err)
	}
	return count
}
