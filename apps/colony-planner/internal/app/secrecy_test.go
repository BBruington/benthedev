package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

// The secrecy and access test (spec §2.2, item 6). Every DM-only value in the fixtures holds a canary.
// Every operation in the OpenAPI document and every route in the route table is called as a player, a
// pending member and a removed member: dm routes must answer 403, member routes 403 for anyone not
// let in, and no response may contain a canary. New endpoints are covered without editing this test.

func plantSettingsCanaries(t *testing.T, testHarness *harness) {
	t.Helper()
	channelID := numericCanary
	if _, err := store.New(testHarness.database.Pool).UpdateSettings(context.Background(), store.UpdateSettingsParams{
		CampaignName:             "Test Campaign",
		Timezone:                 "UTC",
		DiscordAnnounceChannelID: &channelID,
		DiscordReminderChannelID: &channelID,
		DiscordDmChannelID:       &channelID,
	}); err != nil {
		t.Fatalf("plant settings canaries: %v", err)
	}
}

type documentedOperation struct {
	method      string
	path        string
	access      AccessLevel
	requestBody map[string]any
}

func documentedOperations(t *testing.T, server *Server) ([]documentedOperation, map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(server.OpenAPI())
	if err != nil {
		t.Fatalf("encode the OpenAPI document: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode the OpenAPI document: %v", err)
	}
	paths, _ := document["paths"].(map[string]any)
	var operations []documentedOperation
	for path, pathItem := range paths {
		methods, _ := pathItem.(map[string]any)
		for method, operationValue := range methods {
			operation, isOperation := operationValue.(map[string]any)
			if !isOperation {
				continue
			}
			access, _ := operation[accessExtension].(string)
			documented := documentedOperation{method: strings.ToUpper(method), path: path, access: AccessLevel(access)}
			if requestBody, hasBody := operation["requestBody"].(map[string]any); hasBody {
				documented.requestBody = requestBody
			}
			operations = append(operations, documented)
		}
	}
	sort.Slice(operations, func(first, second int) bool {
		return operations[first].path+operations[first].method < operations[second].path+operations[second].method
	})
	if len(operations) == 0 {
		t.Fatal("the OpenAPI document lists no operations")
	}
	return operations, document
}

var pathParameter = regexp.MustCompile(`\{[^}]+\}`)

// fillPath puts a real member's ID into every path parameter, so handlers run against existing rows.
func fillPath(path string, memberID uuid.UUID) string {
	return pathParameter.ReplaceAllString(path, memberID.String())
}

// sampleBody builds a schema-shaped JSON body, so the handler behind validation runs too.
func sampleBody(document map[string]any, requestBody map[string]any, memberID uuid.UUID) string {
	if requestBody == nil {
		return ""
	}
	content, _ := requestBody["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	encoded, _ := json.Marshal(sampleValue(document, schema, memberID))
	return string(encoded)
}

func sampleValue(document map[string]any, schema map[string]any, memberID uuid.UUID) any {
	if reference, isReference := schema["$ref"].(string); isReference {
		components, _ := document["components"].(map[string]any)
		schemas, _ := components["schemas"].(map[string]any)
		resolved, _ := schemas[strings.TrimPrefix(reference, "#/components/schemas/")].(map[string]any)
		return sampleValue(document, resolved, memberID)
	}
	if values, isEnum := schema["enum"].([]any); isEnum && len(values) > 0 {
		return values[0]
	}
	schemaType, _ := schema["type"].(string)
	if types, isList := schema["type"].([]any); isList {
		for _, candidate := range types {
			if name, _ := candidate.(string); name != "null" {
				schemaType = name
				break
			}
		}
	}
	switch schemaType {
	case "object":
		object := map[string]any{}
		properties, _ := schema["properties"].(map[string]any)
		for name, property := range properties {
			propertySchema, _ := property.(map[string]any)
			object[name] = sampleValue(document, propertySchema, memberID)
		}
		return object
	case "array":
		return []any{}
	case "integer", "number":
		return 1
	case "boolean":
		return true
	case "string":
		if format, _ := schema["format"].(string); format == "uuid" {
			return memberID.String()
		}
		if pattern, _ := schema["pattern"].(string); pattern == "^[0-9]{17,20}$" {
			return "123456789012345678"
		}
		if name, _ := schema["description"].(string); strings.Contains(name, "IANA") {
			return "America/New_York"
		}
		return "Sample"
	default:
		return nil
	}
}

// expectedRefusal is the status access control must give, or 0 when the route may answer.
func expectedRefusal(access AccessLevel, viewer core.Viewer, signedIn bool) int {
	switch access {
	case AccessPublic, AccessToken:
		return 0
	}
	if !signedIn {
		return http.StatusUnauthorized
	}
	switch access {
	case AccessDm:
		return http.StatusForbidden
	case AccessMember:
		if !viewer.IsActive() {
			return http.StatusForbidden
		}
	}
	return 0
}

func checkSecrecy(t *testing.T, label string, access AccessLevel, viewer core.Viewer, signedIn bool, response *httptest.ResponseRecorder) {
	t.Helper()
	if refusal := expectedRefusal(access, viewer, signedIn); refusal != 0 && response.Code != refusal {
		t.Errorf("%s (%s): status %d, want %d; body: %s", label, access, response.Code, refusal, response.Body.String())
	}
	if response.Code >= http.StatusInternalServerError {
		t.Errorf("%s (%s): status %d; body: %s", label, access, response.Code, response.Body.String())
	}
	var headers strings.Builder
	_ = response.Header().Write(&headers)
	everything := headers.String() + response.Body.String()
	for _, secret := range []string{canary, numericCanary} {
		if strings.Contains(everything, secret) {
			t.Errorf("%s (%s): the response leaks DM-only data %q: %s", label, access, secret, everything)
		}
	}
}

func TestEveryRouteKeepsDmDataSecret(t *testing.T) {
	testHarness := newHarness(t)
	plantSettingsCanaries(t, testHarness)
	operations, document := documentedOperations(t, testHarness.server)

	for _, caller := range []struct {
		name     string
		member   store.Member
		signedIn bool
	}{
		{name: "player", member: testHarness.player, signedIn: true},
		{name: "pending member", member: testHarness.pending, signedIn: true},
		{name: "removed member", member: testHarness.removed, signedIn: true},
		{name: "visitor", signedIn: false},
	} {
		t.Run(caller.name, func(t *testing.T) {
			viewer := testHarness.viewerOf(caller.member)
			// Another member's ID, so member-targeted operations aim at someone else's row.
			target := testHarness.otherPlayer.ID
			token := func() string {
				if !caller.signedIn {
					return ""
				}
				return testHarness.signIn(caller.member.ID)
			}

			for _, operation := range operations {
				if !operation.access.valid() {
					t.Errorf("%s %s declares no access level", operation.method, operation.path)
					continue
				}
				label := operation.method + " " + operation.path
				response := testHarness.do(operation.method, fillPath(operation.path, target), requestOptions{
					token: token(),
					body:  sampleBody(document, operation.requestBody, target),
				})
				checkSecrecy(t, label, operation.access, viewer, caller.signedIn, response)
			}

			for _, route := range testHarness.server.routes {
				method, path, _ := strings.Cut(route.Pattern, " ")
				options := requestOptions{token: token()}
				if path == "/api/stream" {
					streamContext, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
					defer cancel()
					options.context = streamContext
				}
				response := testHarness.do(method, fillPath(path, target), options)
				checkSecrecy(t, route.Pattern, route.Access, viewer, caller.signedIn, response)
			}
		})
	}
}

// Only GET /api/me, GET /api/stream and POST /auth/logout admit pending and removed members (spec
// §2.2); anything else at that level would show data to people the DM hasn't let in.
func TestOnlyThreeRoutesAdmitPendingMembers(t *testing.T) {
	server := newTestServer(t)
	operations, _ := documentedOperations(t, server)
	var pendingLevel []string
	for _, operation := range operations {
		if operation.access == AccessPending {
			pendingLevel = append(pendingLevel, operation.method+" "+operation.path)
		}
	}
	for _, route := range server.routes {
		if route.Access == AccessPending {
			pendingLevel = append(pendingLevel, route.Pattern)
		}
	}
	slices.Sort(pendingLevel)
	if want := []string{"GET /api/me", "GET /api/stream", "POST /auth/logout"}; !slices.Equal(pendingLevel, want) {
		t.Fatalf("pending-level routes = %v, want %v", pendingLevel, want)
	}
}

// TestDmSeesTheCanaries proves the fixtures really hold DM-only data, so the test above can fail.
func TestDmSeesTheCanaries(t *testing.T) {
	testHarness := newHarness(t)
	plantSettingsCanaries(t, testHarness)
	token := testHarness.signIn(testHarness.dm.ID)

	members := testHarness.do(http.MethodGet, "/api/members", requestOptions{token: token})
	testHarness.expectStatus(members, http.StatusOK)
	if !strings.Contains(members.Body.String(), canary) {
		t.Fatalf("the DM's member list lacks the canary: %s", members.Body.String())
	}
	settings := testHarness.do(http.MethodGet, "/api/settings", requestOptions{token: token})
	testHarness.expectStatus(settings, http.StatusOK)
	if !strings.Contains(settings.Body.String(), numericCanary) {
		t.Fatalf("the DM's settings lack the canary: %s", settings.Body.String())
	}
}

// TestStreamMessagesCarryOnlyTopics replays every SSE message a player, a pending member and a
// removed member receive while the DM works, against the same canaries.
func TestStreamMessagesCarryOnlyTopics(t *testing.T) {
	testHarness := newHarness(t)
	plantSettingsCanaries(t, testHarness)
	dmViewer := testHarness.viewerOf(testHarness.dm)
	testContext := context.Background()

	streams := map[string]*liveStream{
		"player":  testHarness.openStream(testHarness.signIn(testHarness.player.ID), ""),
		"pending": testHarness.openStream(testHarness.signIn(testHarness.pending.ID), ""),
		"removed": testHarness.openStream(testHarness.signIn(testHarness.removed.ID), ""),
	}
	for name, stream := range streams {
		if _, received := stream.nextMessage(2 * time.Second); !received {
			t.Fatalf("%s: no catch-up message on connect", name)
		}
	}

	secretName := canary + " name"
	if _, err := testHarness.core.UpdateSettings(testContext, dmViewer, core.SettingsInput{CampaignName: secretName, Timezone: "UTC"}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if _, err := testHarness.core.SetCharacterName(testContext, dmViewer, testHarness.pending.ID, core.CharacterNameInput{CharacterName: &secretName}); err != nil {
		t.Fatalf("set character name: %v", err)
	}
	if _, err := testHarness.core.SetCharacterName(testContext, dmViewer, testHarness.removed.ID, core.CharacterNameInput{CharacterName: &secretName}); err != nil {
		t.Fatalf("set character name: %v", err)
	}

	known := core.AllTopics()
	for name, stream := range streams {
		var topics []core.Topic
		for {
			message, received := stream.nextMessage(300 * time.Millisecond)
			if !received {
				break
			}
			if strings.Contains(message.raw, canary) || strings.Contains(message.raw, numericCanary) {
				t.Errorf("%s: a stream message leaks DM-only data: %q", name, message.raw)
			}
			if message.event != "invalidate" {
				t.Errorf("%s: unexpected event %q", name, message.event)
			}
			var decoded core.InvalidateMessage
			if err := json.Unmarshal([]byte(message.data), &decoded); err != nil {
				t.Errorf("%s: undecodable data %q: %v", name, message.data, err)
			}
			for _, topic := range decoded.Topics {
				if !slices.Contains(known, topic) {
					t.Errorf("%s: unknown topic %q", name, topic)
				}
			}
			topics = append(topics, decoded.Topics...)
		}
		if name == "player" && !slices.Contains(topics, core.TopicSettings) {
			t.Errorf("player: expected a settings invalidation, got %v", topics)
		}
		if name != "player" {
			if fmt.Sprint(topics) != fmt.Sprint([]core.Topic{core.TopicMe}) {
				t.Errorf("%s: should hear only its own me topic once, got %v", name, topics)
			}
		}
	}
}
