package app

import (
	"bufio"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/db/dbtest"
	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

const (
	testDmDiscordID = "100000000000000001"
	testPublicURL   = "http://localhost:5173"
	// testHost is the Host header a browser sends through the Vite proxy.
	testHost = "localhost:5173"
	// canary is written into every DM-only value the fixtures hold; no player response may contain it.
	canary = "CANARY-a91f"
	// numericCanary fills DM-only fields that accept only digits (Discord channel IDs).
	numericCanary = "990099009900990099"
)

type recordingNotifier struct {
	mutex   sync.Mutex
	waiting []core.WaitingMember
}

func (notifier *recordingNotifier) MemberWaiting(_ context.Context, member core.WaitingMember) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.waiting = append(notifier.waiting, member)
}

// harness is the whole server on a fresh test schema, with one member of every kind.
type harness struct {
	t        *testing.T
	database *dbtest.Database
	core     *core.Service
	auth     *auth.Service
	cache    *auth.SessionCache
	server   *Server
	handler  http.Handler
	notifier *recordingNotifier

	dm          store.Member
	player      store.Member
	otherPlayer store.Member
	pending     store.Member
	removed     store.Member
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	cache := auth.NewSessionCache()
	notifier := &recordingNotifier{}
	service := core.New(core.Options{
		Pool:        database.Pool,
		Notifier:    notifier,
		Logger:      logger,
		DmDiscordID: testDmDiscordID,
		Sessions:    cache,
	})
	authService, err := auth.New(auth.Options{
		Core:           service,
		Logger:         logger,
		SessionCache:   cache,
		PublicURL:      testPublicURL,
		AppEnvironment: "test",
		DmDiscordID:    testDmDiscordID,
	})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	server, err := New(Options{
		Core:            service,
		Auth:            authService,
		Logger:          logger,
		StreamHeartbeat: 50 * time.Millisecond,
		StreamLifetime:  10 * time.Second,
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	testHarness := &harness{
		t:        t,
		database: database,
		core:     service,
		auth:     authService,
		cache:    cache,
		server:   server,
		handler:  server.Handler(),
		notifier: notifier,
	}
	testHarness.dm = testHarness.createMember(testDmDiscordID, "dm."+canary, "Dee the DM", "", store.MemberStatusACTIVE)
	testHarness.player = testHarness.createMember(canary+"-ana", "ana."+canary, "Ana", "Thorin", store.MemberStatusACTIVE)
	testHarness.otherPlayer = testHarness.createMember(canary+"-bram", "bram."+canary, "Bram", "Sela", store.MemberStatusACTIVE)
	testHarness.pending = testHarness.createMember(canary+"-emil", "emil."+canary, "Emil", "", store.MemberStatusPENDING)
	testHarness.removed = testHarness.createMember(canary+"-gus", "gus."+canary, "Gus", "", store.MemberStatusREMOVED)
	return testHarness
}

func (testHarness *harness) createMember(discordUserID, username, displayName, characterName string, status store.MemberStatus) store.Member {
	testHarness.t.Helper()
	parameters := store.CreateMemberParams{
		DiscordUserID:   discordUserID,
		DiscordUsername: username,
		DisplayName:     displayName,
		Status:          status,
	}
	if characterName != "" {
		parameters.CharacterName = &characterName
	}
	member, err := store.New(testHarness.database.Pool).CreateMember(context.Background(), parameters)
	if err != nil {
		testHarness.t.Fatalf("create member %s: %v", displayName, err)
	}
	return member
}

// signIn starts a web session for the member and returns its cookie value.
func (testHarness *harness) signIn(memberID uuid.UUID) string {
	testHarness.t.Helper()
	session, err := testHarness.core.CreateSession(context.Background(), memberID)
	if err != nil {
		testHarness.t.Fatalf("create session: %v", err)
	}
	return session.Token
}

func (testHarness *harness) viewerOf(member store.Member) core.Viewer {
	return testHarness.core.NewViewer(core.ViewerIdentity{
		MemberID:      member.ID,
		DiscordUserID: member.DiscordUserID,
		Status:        member.Status,
	})
}

type requestOptions struct {
	token   string
	body    string
	headers map[string]string
	context context.Context
	// host replaces testHost.
	host string
}

func (testHarness *harness) do(method, path string, options requestOptions) *httptest.ResponseRecorder {
	testHarness.t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(options.body))
	request.Host = testHost
	if options.host != "" {
		request.Host = options.host
	}
	if options.context != nil {
		request = request.WithContext(options.context)
	}
	if options.body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if options.token != "" {
		request.AddCookie(&http.Cookie{Name: testHarness.auth.SessionCookieName(), Value: options.token})
	}
	for name, value := range options.headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	testHarness.handler.ServeHTTP(recorder, request)
	return recorder
}

func (testHarness *harness) expectStatus(response *httptest.ResponseRecorder, status int) {
	testHarness.t.Helper()
	if response.Code != status {
		testHarness.t.Fatalf("status = %d, want %d; body: %s", response.Code, status, response.Body.String())
	}
}

// streamEvent is one SSE frame: a message (id, event, data) or a comment.
type streamEvent struct {
	id      string
	event   string
	data    string
	comment string
	raw     string
}

// liveStream is GET /api/stream over a real HTTP connection.
type liveStream struct {
	events chan streamEvent
	cancel context.CancelFunc
	status int
}

func (testHarness *harness) openStream(token string, lastEventID string) *liveStream {
	testHarness.t.Helper()
	server := httptest.NewServer(testHarness.handler)
	testHarness.t.Cleanup(server.Close)
	streamContext, cancel := context.WithCancel(context.Background())
	testHarness.t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(streamContext, http.MethodGet, server.URL+"/api/stream", nil)
	if err != nil {
		testHarness.t.Fatalf("build stream request: %v", err)
	}
	request.AddCookie(&http.Cookie{Name: testHarness.auth.SessionCookieName(), Value: token})
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		testHarness.t.Fatalf("open stream: %v", err)
	}
	stream := &liveStream{events: make(chan streamEvent, 64), cancel: cancel, status: response.StatusCode}
	go func() {
		defer close(stream.events)
		defer func() { _ = response.Body.Close() }()
		reader := bufio.NewReader(response.Body)
		var current streamEvent
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			current.raw += line
			line = strings.TrimRight(line, "\n")
			switch {
			case line == "":
				if current.raw != "\n" {
					stream.events <- current
				}
				current = streamEvent{}
			case strings.HasPrefix(line, ":"):
				current.comment = strings.TrimSpace(strings.TrimPrefix(line, ":"))
			case strings.HasPrefix(line, "id: "):
				current.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				current.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return stream
}

// nextMessage skips heartbeats and returns the next `event:` message, or false if the stream ended or
// nothing arrived in time.
func (stream *liveStream) nextMessage(timeout time.Duration) (streamEvent, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case event, open := <-stream.events:
			if !open {
				return streamEvent{}, false
			}
			if event.event != "" {
				return event, true
			}
		case <-deadline:
			return streamEvent{}, false
		}
	}
}

// waitForEnd reports whether the server closed the stream within timeout.
func (stream *liveStream) waitForEnd(timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case _, open := <-stream.events:
			if !open {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
