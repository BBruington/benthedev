package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// The secrecy and access test (spec §2.2, item 6). Each feature plants a canary in every DM-only value
// it stores (secrecy_fixtures_test.go). Every operation in the OpenAPI document and every route in the
// route table is then called as a player, the DM viewing as player, a pending member, a removed member
// and a visitor: dm routes must answer 403, member routes 403 for anyone not let in, member reads must
// succeed for an active caller, every /api/ and /auth/ response must say Cache-Control: no-store, and
// no response may contain a canary. Path parameters point at the fixtures' rows and required query
// parameters are filled from their schemas, so a new endpoint is swept without editing this file. Its
// DM-only data is covered only once its feature plants canaries: plant one for every new Dm field or
// hidden row.

type documentedOperation struct {
	method      string
	path        string
	access      AccessLevel
	requestBody map[string]any
	parameters  []map[string]any
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
			parameters, _ := operation["parameters"].([]any)
			for _, parameterValue := range parameters {
				if parameter, isParameter := parameterValue.(map[string]any); isParameter {
					documented.parameters = append(documented.parameters, parameter)
				}
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

// fillPath puts the fixture row named after each path parameter into the path, so handlers run against
// existing rows. A parameter no fixture names fails the test: add its ID in secrecy_fixtures_test.go.
func fillPath(t *testing.T, path string, fixtureIDs map[string]string) string {
	t.Helper()
	return pathParameter.ReplaceAllStringFunc(path, func(placeholder string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(placeholder, "{"), "}")
		id, known := fixtureIDs[name]
		if !known {
			t.Errorf("%s: no secrecy fixture names path parameter %q; return one from its feature's plant", path, name)
			return uuid.Nil.String()
		}
		return id
	})
}

// fillQuery adds every required query parameter: a fixture ID when one has its name, otherwise a value
// shaped by its schema.
func fillQuery(document map[string]any, operation documentedOperation, fixtureIDs map[string]string, memberID uuid.UUID) string {
	query := url.Values{}
	for _, parameter := range operation.parameters {
		if location, _ := parameter["in"].(string); location != "query" {
			continue
		}
		if required, _ := parameter["required"].(bool); !required {
			continue
		}
		name, _ := parameter["name"].(string)
		if id, known := fixtureIDs[name]; known {
			query.Set(name, id)
			continue
		}
		schema, _ := parameter["schema"].(map[string]any)
		query.Set(name, fmt.Sprint(sampleValue(document, schema, memberID)))
	}
	if len(query) == 0 {
		return ""
	}
	return "?" + query.Encode()
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
		// Core checks formats, so the schema only describes them; a valid sample lets the handler run.
		description, _ := schema["description"].(string)
		if strings.Contains(description, "Discord channel ID") {
			return "123456789012345678"
		}
		if strings.Contains(description, "IANA") {
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

// mustSucceed reports whether an active caller's request is a plain read that has to work. A member
// read that answers 404 or 422 never ran its data path, so the sweep would prove nothing about it.
func mustSucceed(method string, access AccessLevel, viewer core.Viewer, signedIn bool) bool {
	return signedIn && viewer.IsActive() && method == http.MethodGet && (access == AccessMember || access == AccessPending)
}

func checkSecrecy(t *testing.T, label, method, path string, access AccessLevel, viewer core.Viewer, signedIn bool, response *httptest.ResponseRecorder) {
	t.Helper()
	if refusal := expectedRefusal(access, viewer, signedIn); refusal != 0 && response.Code != refusal {
		t.Errorf("%s (%s): status %d, want %d; body: %s", label, access, response.Code, refusal, response.Body.String())
	}
	if response.Code >= http.StatusInternalServerError {
		t.Errorf("%s (%s): status %d; body: %s", label, access, response.Code, response.Body.String())
	}
	if mustSucceed(method, access, viewer, signedIn) && (response.Code < 200 || response.Code > 299) {
		t.Errorf("%s (%s): an active member's read answered %d; body: %s", label, access, response.Code, response.Body.String())
	}
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/auth/") {
		if cacheControl := response.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
			t.Errorf("%s (%s): Cache-Control %q, want no-store", label, access, cacheControl)
		}
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
	fixtureIDs := plantCanaries(t, testHarness)
	operations, document := documentedOperations(t, testHarness.server)
	dmViewingAsPlayer := testHarness.viewerOf(testHarness.dm)
	dmViewingAsPlayer.ViewAsPlayer = true

	for _, caller := range []struct {
		name         string
		member       store.Member
		viewer       core.Viewer
		signedIn     bool
		viewAsPlayer bool
	}{
		{name: "player", member: testHarness.player, viewer: testHarness.viewerOf(testHarness.player), signedIn: true},
		{name: "DM viewing as player", member: testHarness.dm, viewer: dmViewingAsPlayer, signedIn: true, viewAsPlayer: true},
		{name: "pending member", member: testHarness.pending, viewer: testHarness.viewerOf(testHarness.pending), signedIn: true},
		{name: "removed member", member: testHarness.removed, viewer: testHarness.viewerOf(testHarness.removed), signedIn: true},
		{name: "visitor", signedIn: false},
	} {
		t.Run(caller.name, func(t *testing.T) {
			target := testHarness.otherPlayer.ID
			// A fresh session per call, because the sweep also calls sign out.
			token := func() string {
				if !caller.signedIn {
					return ""
				}
				token := testHarness.signIn(caller.member.ID)
				if caller.viewAsPlayer {
					testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/me/view-as-player", requestOptions{
						token: token,
						body:  `{"enabled":true}`,
					}), http.StatusNoContent)
				}
				return token
			}

			for _, operation := range operations {
				if !operation.access.valid() {
					t.Errorf("%s %s declares no access level", operation.method, operation.path)
					continue
				}
				label := operation.method + " " + operation.path
				path := fillPath(t, operation.path, fixtureIDs)
				response := testHarness.do(operation.method, path+fillQuery(document, operation, fixtureIDs, target), requestOptions{
					token: token(),
					body:  sampleBody(document, operation.requestBody, target),
				})
				checkSecrecy(t, label, operation.method, path, operation.access, caller.viewer, caller.signedIn, response)
			}

			for _, route := range testHarness.server.routes {
				method, routePath, _ := strings.Cut(route.Pattern, " ")
				options := requestOptions{token: token()}
				if routePath == "/api/stream" {
					streamContext, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
					defer cancel()
					options.context = streamContext
				}
				path := fillPath(t, routePath, fixtureIDs)
				response := testHarness.do(method, path, options)
				checkSecrecy(t, route.Pattern, method, path, route.Access, caller.viewer, caller.signedIn, response)
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
	plantCanaries(t, testHarness)
	token := testHarness.signIn(testHarness.dm.ID)

	for _, fixture := range secrecyFixtures {
		if len(fixture.dmProof) == 0 {
			t.Errorf("secrecy fixture %s names no GET path where the DM sees its canaries", fixture.feature)
		}
		for _, path := range fixture.dmProof {
			response := testHarness.do(http.MethodGet, path, requestOptions{token: token})
			testHarness.expectStatus(response, http.StatusOK)
			if body := response.Body.String(); !strings.Contains(body, canary) && !strings.Contains(body, numericCanary) {
				t.Errorf("%s: the DM's %s lacks the canary: %s", fixture.feature, path, body)
			}
		}
	}
}

// TestStreamMessagesCarryOnlyTopics replays every SSE message a player, a pending member and a
// removed member receive while the DM works, against the same canaries.
func TestStreamMessagesCarryOnlyTopics(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
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
	if _, err := testHarness.core.UpdateSettings(testContext, dmViewer, core.SettingsInput{CampaignName: secretName, ColonyName: secretName, Timezone: "UTC"}); err != nil {
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
