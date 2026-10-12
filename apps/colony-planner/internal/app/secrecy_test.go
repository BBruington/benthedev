package app

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
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

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// The cross-campaign secrecy and access sweep (spec §2.2 item 6; docs/design/multi-campaign.md §4.4).
// Each feature plants tokens into both campaigns (secrecy_fixtures_test.go): canaryA in A's DM-only
// values, canaryB in every value of B its players see, canaryBdm in B's DM-only values, plus each
// campaign's invite and link codes and Discord IDs as literals. Every operation in the OpenAPI document
// and every route in the route table is then called by every kind of caller, path parameters filled
// with real rows and bodies shaped by their schemas, so a new endpoint is swept without editing this
// file. A leak is caught only if its value was planted: plant a token for every new Dm field or hidden
// row, in both campaigns.

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

// sweepRoute is one OpenAPI operation or one route-table route.
type sweepRoute struct {
	label     string
	method    string
	path      string
	access    AccessLevel
	operation *documentedOperation
}

// inCampaign reports whether the route is a campaign route: {campaignId} outside /api/admin/.
func (route sweepRoute) inCampaign() bool {
	return strings.Contains(route.path, "{"+campaignParameter+"}") && !strings.HasPrefix(route.path, adminPathPrefix)
}

func (route sweepRoute) isLeave() bool {
	return route.method == http.MethodPost && strings.HasSuffix(route.path, "/leave")
}

func (route sweepRoute) isStream() bool {
	return route.operation == nil && strings.HasSuffix(route.path, "/stream")
}

func sweepRoutes(t *testing.T, server *Server) ([]sweepRoute, map[string]any) {
	t.Helper()
	operations, document := documentedOperations(t, server)
	routes := make([]sweepRoute, 0, len(operations)+len(server.routes))
	for index := range operations {
		operation := &operations[index]
		if !operation.access.valid() {
			t.Errorf("%s %s declares no access level", operation.method, operation.path)
		}
		routes = append(routes, sweepRoute{
			label:     operation.method + " " + operation.path,
			method:    operation.method,
			path:      operation.path,
			access:    operation.access,
			operation: operation,
		})
	}
	for _, route := range server.routes {
		method, path, _ := strings.Cut(route.Pattern, " ")
		routes = append(routes, sweepRoute{label: route.Pattern, method: method, path: path, access: route.Access})
	}
	return routes, document
}

// campaignRoutesLeaveLast lists the campaign routes with every DELETE and then POST .../leave at the
// end, so the rows a fixture names stay for every other call and a caller who leaves does so last.
func campaignRoutesLeaveLast(routes []sweepRoute) []sweepRoute {
	var campaignRoutes, deletes, leave []sweepRoute
	for _, route := range routes {
		switch {
		case !route.inCampaign():
		case route.isLeave():
			leave = append(leave, route)
		case route.method == http.MethodDelete:
			deletes = append(deletes, route)
		default:
			campaignRoutes = append(campaignRoutes, route)
		}
	}
	return append(append(campaignRoutes, deletes...), leave...)
}

var pathParameter = regexp.MustCompile(`\{[^}]+\}`)

// fillPath puts the row named after each path parameter into the path. A parameter no fixture names
// fails the test: record one in its feature's plant (secrecy_fixtures_test.go).
func fillPath(t *testing.T, path string, pathIDs map[string]string) string {
	t.Helper()
	return pathParameter.ReplaceAllStringFunc(path, func(placeholder string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(placeholder, "{"), "}")
		id, known := pathIDs[name]
		if !known {
			t.Errorf("%s: no secrecy fixture names path parameter %q; record one in its feature's plant", path, name)
			return uuid.Nil.String()
		}
		return url.PathEscape(id)
	})
}

// queryDefaults keeps day ranges wide enough to reach the planted entries.
var queryDefaults = map[string]string{"fromDay": "0", "toDay": "30", "days": "1"}

// fillQuery adds every required query parameter, shaped by its schema.
func fillQuery(document map[string]any, operation *documentedOperation, sampleID uuid.UUID) string {
	if operation == nil {
		return ""
	}
	query := url.Values{}
	for _, parameter := range operation.parameters {
		if location, _ := parameter["in"].(string); location != "query" {
			continue
		}
		if required, _ := parameter["required"].(bool); !required {
			continue
		}
		name, _ := parameter["name"].(string)
		if value, known := queryDefaults[name]; known {
			query.Set(name, value)
			continue
		}
		schema, _ := parameter["schema"].(map[string]any)
		query.Set(name, fmt.Sprint(sampleValue(document, schema, sampleID)))
	}
	if len(query) == 0 {
		return ""
	}
	return "?" + query.Encode()
}

func requestBodySchema(requestBody map[string]any) map[string]any {
	content, _ := requestBody["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	return schema
}

// sampleBody builds a schema-shaped JSON body, so the handler behind validation runs too.
func sampleBody(document map[string]any, operation *documentedOperation, sampleID uuid.UUID) string {
	if operation == nil || operation.requestBody == nil {
		return ""
	}
	encoded, _ := json.Marshal(sampleValue(document, requestBodySchema(operation.requestBody), sampleID))
	return string(encoded)
}

func resolveSchema(document map[string]any, schema map[string]any) map[string]any {
	reference, isReference := schema["$ref"].(string)
	if !isReference {
		return schema
	}
	components, _ := document["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	resolved, _ := schemas[strings.TrimPrefix(reference, "#/components/schemas/")].(map[string]any)
	return resolved
}

func schemaType(schema map[string]any) string {
	if name, isString := schema["type"].(string); isString {
		return name
	}
	types, _ := schema["type"].([]any)
	for _, candidate := range types {
		if name, _ := candidate.(string); name != "null" {
			return name
		}
	}
	return ""
}

func sampleValue(document map[string]any, schema map[string]any, sampleID uuid.UUID) any {
	schema = resolveSchema(document, schema)
	if values, isEnum := schema["enum"].([]any); isEnum && len(values) > 0 {
		return values[0]
	}
	switch schemaType(schema) {
	case "object":
		object := map[string]any{}
		properties, _ := schema["properties"].(map[string]any)
		for name, property := range properties {
			propertySchema, _ := property.(map[string]any)
			object[name] = sampleValue(document, propertySchema, sampleID)
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
			return sampleID.String()
		}
		// Core checks formats, so the schema only describes them; a valid sample lets the handler run.
		if description, _ := schema["description"].(string); strings.Contains(description, "IANA") {
			return "America/New_York"
		}
		return "Sample"
	default:
		return nil
	}
}

// sweepCaller is one caller of a campaign pass and what they are in the campaign the pass calls.
type sweepCaller struct {
	name string
	// who is nil for a visitor.
	who          *account
	isMember     bool
	isDM         bool
	viewAsPlayer bool
	status       core.MemberStatus
	// forbidden are the values no response to this caller may hold.
	forbidden []string
	// token, when set, is reused for every call; otherwise each call signs in afresh (the sweep calls
	// sign out and view as player, which change a session).
	token string
}

func (testHarness *harness) tokenFor(caller sweepCaller, campaign *campaignFixture) string {
	testHarness.t.Helper()
	if caller.who == nil {
		return ""
	}
	if caller.token != "" {
		return caller.token
	}
	token := testHarness.signIn(*caller.who)
	if caller.viewAsPlayer {
		testHarness.expectStatus(testHarness.do(http.MethodPost, campaignPathOf(campaign, "/me/view-as-player"), requestOptions{
			token: token,
			body:  `{"enabled":true}`,
		}), http.StatusNoContent)
	}
	return token
}

// call performs one route as the caller with the given path IDs and body.
func (testHarness *harness) call(route sweepRoute, document map[string]any, token string, pathIDs map[string]string, body string) (string, *httptest.ResponseRecorder) {
	testHarness.t.Helper()
	path := fillPath(testHarness.t, route.path, pathIDs)
	options := requestOptions{token: token, body: body}
	if route.isStream() {
		streamContext, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		options.context = streamContext
	}
	return path, testHarness.do(route.method, path+fillQuery(document, route.operation, uuid.New()), options)
}

// expectedCampaignAnswer is the status the access layer must give a caller on a campaign route, or 0
// when the route may answer.
func expectedCampaignAnswer(access AccessLevel, caller sweepCaller, campaignActive bool) int {
	switch {
	case caller.who == nil:
		return http.StatusUnauthorized
	case !caller.isMember:
		return http.StatusNotFound
	case access == AccessPending:
		return 0
	case caller.status != core.MemberStatusActive || !campaignActive:
		return http.StatusForbidden
	case access == AccessDm && (!caller.isDM || caller.viewAsPlayer):
		return http.StatusForbidden
	}
	return 0
}

// checkResponse holds for every answer: no 5xx, no-store on /api/ and /auth/, and no forbidden value
// in the headers or body. ownValues are the caller's own (their Discord username on GET /api/me), which
// they may see even where they hold a token.
func checkResponse(t *testing.T, label string, path string, forbidden []string, response *httptest.ResponseRecorder, ownValues ...string) {
	t.Helper()
	if response.Code >= http.StatusInternalServerError {
		t.Errorf("%s: status %d; body: %s", label, response.Code, response.Body.String())
	}
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/auth/") {
		if cacheControl := response.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
			t.Errorf("%s: Cache-Control %q, want no-store", label, cacheControl)
		}
	}
	var headers strings.Builder
	_ = response.Header().Write(&headers)
	everything := headers.String() + response.Body.String()
	for _, own := range ownValues {
		everything = strings.ReplaceAll(everything, own, "")
	}
	for _, secret := range forbidden {
		if secret != "" && strings.Contains(everything, secret) {
			t.Errorf("%s: the response leaks %q: %s", label, secret, everything)
		}
	}
}

// sweepCampaign calls every campaign route of campaign as the caller and checks the access answer.
func (testHarness *harness) sweepCampaign(t *testing.T, routes []sweepRoute, document map[string]any, campaign *campaignFixture, caller sweepCaller, campaignActive bool) {
	t.Helper()
	for _, route := range campaignRoutesLeaveLast(routes) {
		token := testHarness.tokenFor(caller, campaign)
		path, response := testHarness.call(route, document, token, campaign.pathIDs, sampleBody(document, route.operation, uuid.New()))
		label := caller.name + ": " + route.label
		checkResponse(t, label, path, caller.forbidden, response)
		if route.isLeave() {
			// Leaving is checked by TestLeavingACampaign; here it only must not leak.
			continue
		}
		expected := expectedCampaignAnswer(route.access, caller, campaignActive)
		if expected != 0 && response.Code != expected {
			t.Errorf("%s (%s): status %d, want %d; body: %s", label, route.access, response.Code, expected, response.Body.String())
		}
		if expected == 0 && route.method == http.MethodGet && (route.access == AccessMember || route.access == AccessPending) &&
			(response.Code < 200 || response.Code > 299) {
			// A read that answers 404 or 422 never ran its data path, so the sweep would prove nothing.
			t.Errorf("%s (%s): a member's read answered %d; body: %s", label, route.access, response.Code, response.Body.String())
		}
	}
}

func joinValues(groups ...[]string) []string {
	var values []string
	for _, group := range groups {
		values = append(values, group...)
	}
	return values
}

// Passes 1 and 1b, and every account's view of the routes outside campaigns.
func TestPlayersSeeOnlyTheirCampaignsPlayerData(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	routes, document := sweepRoutes(t, testHarness.server)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	secretA, secretB := testHarness.dmOnlyValues(campaignA), testHarness.dmOnlyValues(campaignB)
	everything := joinValues(secretA, secretB, []string{canaryB})
	active, pending, removed := core.MemberStatusActive, core.MemberStatusPending, core.MemberStatusRemoved

	t.Run("pass 1: players of A", func(t *testing.T) {
		for _, caller := range []sweepCaller{
			{name: "ana", who: &testHarness.ana, isMember: true, status: active},
			{name: "otherA", who: &testHarness.otherA, isMember: true, status: active},
			// A DM elsewhere is a player here, with player rules.
			{name: "dmB (DM of B)", who: &testHarness.dmB, isMember: true, status: active},
			{name: "dmA viewing as player", who: &testHarness.dmA, isMember: true, isDM: true, viewAsPlayer: true, status: active},
			{name: "pendingA", who: &testHarness.pendingA, isMember: true, status: pending},
			{name: "removedA", who: &testHarness.removedA, isMember: true, status: removed},
			{name: "visitor"},
		} {
			caller.forbidden = everything
			testHarness.sweepCampaign(t, routes, document, campaignA, caller, true)
		}
	})

	t.Run("pass 1b: players of B", func(t *testing.T) {
		for _, caller := range []sweepCaller{
			// A's DM is a player in B: B's dm routes are closed to them.
			{name: "dmA (DM of A)", who: &testHarness.dmA, isMember: true, status: active},
			{name: "ana", who: &testHarness.ana, isMember: true, status: active},
		} {
			caller.forbidden = joinValues(secretB, secretA)
			testHarness.sweepCampaign(t, routes, document, campaignB, caller, true)
		}
	})

	t.Run("routes outside campaigns", func(t *testing.T) {
		// A bogus invite code: joining a campaign mid-sweep would change who is in it.
		pathIDs := map[string]string{
			"campaignId":        campaignA.id.String(),
			"campaignRequestId": campaignA.requestID.String(),
			"inviteCode":        "no-such-invite-code-0000000",
		}
		inB := []string{"dmA", "ana", "dmB", "bOnly"}
		usernames := map[string]string{"otherA": "bram." + canaryA, "pendingA": "emil." + canaryA, "removedA": "gus." + canaryA, "bOnly": "iko." + canaryBdm}
		for _, who := range []account{testHarness.owner, testHarness.dmA, testHarness.ana, testHarness.otherA, testHarness.pendingA, testHarness.removedA, testHarness.dmB, testHarness.bOnly, testHarness.outsider} {
			forbidden := everything
			if slices.Contains(inB, who.name) {
				// B's members see B's player-visible values on their own picker.
				forbidden = joinValues(secretA, secretB)
			}
			for _, route := range routes {
				if route.inCampaign() {
					continue
				}
				if route.path == "/api/login-options" && auth.DevAuthEnabled {
					// Dev sign-in lists every account with its memberships, by design (§7.3); only devauth
					// builds have it, and the build without the tag sweeps this route too.
					continue
				}
				token := testHarness.signIn(who)
				path, response := testHarness.call(route, document, token, pathIDs, sampleBody(document, route.operation, uuid.New()))
				label := who.name + ": " + route.label
				checkResponse(t, label, path, forbidden, response, usernames[who.name])
				if route.access == AccessOwner && !who.caller.IsSiteOwner && response.Code != http.StatusForbidden {
					t.Errorf("%s: a non-owner got %d from an owner route", label, response.Code)
				}
				if route.access == AccessAccount && route.method == http.MethodGet && !strings.Contains(route.path, "{") && response.Code != http.StatusOK {
					t.Errorf("%s: an account read answered %d; body: %s", label, response.Code, response.Body.String())
				}
			}
		}
	})
}

// Passes 2 to 6, with campaign B's rows fingerprinted before and after: nothing campaign A's DM, B's
// members, an outsider or the site owner does from A's routes or the admin routes changes B.
func TestCampaignsStaySealed(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	routes, document := sweepRoutes(t, testHarness.server)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	secretA, secretB := testHarness.dmOnlyValues(campaignA), testHarness.dmOnlyValues(campaignB)
	everything := joinValues(secretA, secretB, []string{canaryB})
	notB := joinValues(secretB, []string{canaryB})
	// B-only accounts' sessions are fingerprinted, so they are made before and reused.
	bOnlyToken := testHarness.signIn(testHarness.bOnly)
	before := testHarness.fingerprint(campaignB, testHarness.bOnly)

	t.Run("pass 2: non-members get 404 from every campaign route", func(t *testing.T) {
		for _, caller := range []sweepCaller{
			{name: "bOnly", who: &testHarness.bOnly, token: bOnlyToken},
			{name: "outsider", who: &testHarness.outsider},
			{name: "owner", who: &testHarness.owner},
		} {
			caller.forbidden = everything
			testHarness.sweepCampaign(t, routes, document, campaignA, caller, true)
			// An unparsable, empty or unknown campaign ID reads exactly like a campaign the caller isn't in.
			for _, campaignID := range []string{"not-a-uuid", uuid.Nil.String(), uuid.New().String(), campaignA.id.String() + "x"} {
				pathIDs := maps.Clone(campaignA.pathIDs)
				pathIDs["campaignId"] = campaignID
				for _, route := range campaignRoutesLeaveLast(routes) {
					path, response := testHarness.call(route, document, testHarness.tokenFor(caller, campaignA), pathIDs, sampleBody(document, route.operation, uuid.New()))
					checkResponse(t, caller.name+": "+route.label, path, everything, response)
					if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), core.ErrNoSuchCampaign.Message) {
						t.Errorf("%s: %s with campaign %q = %d, want 404 No such campaign; body: %s", caller.name, route.label, campaignID, response.Code, response.Body.String())
					}
				}
			}
		}
	})

	t.Run("pass 6: the owner on the admin routes", func(t *testing.T) {
		// Admin {campaignId} routes act on A, so B's rows stay as they were.
		pathIDs := map[string]string{"campaignId": campaignA.id.String(), "campaignRequestId": campaignA.requestID.String()}
		for _, route := range routes {
			if route.access != AccessOwner {
				continue
			}
			path, response := testHarness.call(route, document, testHarness.signIn(testHarness.owner), pathIDs, sampleBody(document, route.operation, uuid.New()))
			checkResponse(t, "owner: "+route.label, path, everything, response)
			if route.method == http.MethodGet && response.Code != http.StatusOK {
				t.Errorf("owner: %s answered %d; body: %s", route.label, response.Code, response.Body.String())
			}
		}
		list := testHarness.do(http.MethodGet, "/api/admin/campaigns", requestOptions{token: testHarness.signIn(testHarness.owner)})
		testHarness.expectStatus(list, http.StatusOK)
		for _, directoryValue := range []string{campaignA.slug, campaignB.slug, "Hollis", "hollis"} {
			if !strings.Contains(list.Body.String(), directoryValue) {
				t.Errorf("the owner's campaign list lacks %q: %s", directoryValue, list.Body.String())
			}
		}
	})

	t.Run("pass 3: A's DM runs every operation", func(t *testing.T) {
		caller := sweepCaller{name: "dmA", who: &testHarness.dmA, isMember: true, isDM: true, status: core.MemberStatusActive, forbidden: notB}
		testHarness.sweepCampaign(t, routes, document, campaignA, caller, true)
	})

	t.Run("pass 4: A's DM with B's IDs and keys", func(t *testing.T) {
		testHarness.sweepMixedIDs(t, routes, document, notB)
	})

	t.Run("pass 5: A suspended", func(t *testing.T) {
		testHarness.refreshInviteCode(campaignA, testHarness.dmA)
		testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/admin/campaigns/"+campaignA.id.String()+"/suspend", requestOptions{token: testHarness.signIn(testHarness.owner)}), http.StatusOK)
		for _, caller := range []sweepCaller{
			{name: "dmA", who: &testHarness.dmA, isMember: true, isDM: true, status: core.MemberStatusActive, forbidden: notB},
			{name: "ana", who: &testHarness.ana, isMember: true, status: core.MemberStatusActive, forbidden: everything},
		} {
			testHarness.sweepCampaign(t, routes, document, campaignA, caller, false)
		}
		outsiderToken := testHarness.signIn(testHarness.outsider)
		invite := testHarness.do(http.MethodGet, "/api/invites/"+campaignA.inviteCode, requestOptions{token: outsiderToken})
		testHarness.expectStatus(invite, http.StatusOK)
		checkResponse(t, "outsider: the paused invite", "/api/invites/", everything, invite)
		var info core.InviteInfo
		if err := json.Unmarshal(invite.Body.Bytes(), &info); err != nil || info.CampaignStatus != core.CampaignStatusSuspended || info.Membership != nil {
			t.Errorf("the paused invite = %+v, %v; want SUSPENDED and no membership", info, err)
		}
		join := testHarness.do(http.MethodPost, "/api/invites/"+campaignA.inviteCode+"/join", requestOptions{token: outsiderToken})
		testHarness.expectStatus(join, http.StatusConflict)
		checkResponse(t, "outsider: joining a paused campaign", "/api/invites/", everything, join)
		mine, err := testHarness.core.MyCampaigns(testHarness.context, testHarness.outsider.caller)
		if err != nil || len(mine.Campaigns) != 0 {
			t.Errorf("joining a paused campaign made a membership: %+v, %v", mine, err)
		}
	})

	if after := testHarness.fingerprint(campaignB, testHarness.bOnly); !maps.Equal(before, after) {
		for name, hash := range after {
			if before[name] != hash {
				t.Errorf("campaign B's %s changed during passes 2 to 6", name)
			}
		}
	}

	// The log is the operator's (the site owner's) view: it carries IDs (Discord server IDs among
	// them), kinds, counts and statuses, never campaign content, an invite code or a /link code, which
	// only the campaign's DM may read (Invariant 14).
	logOutput := testHarness.logs.String()
	if !strings.Contains(logOutput, "/api/invites/{inviteCode}/join") {
		t.Errorf("the sweep's invite requests weren't logged: the log check below would prove nothing")
	}
	for _, forbidden := range joinValues([]string{canaryA, canaryB, campaignA.inviteCode, campaignB.inviteCode}, campaignA.linkCodes, campaignB.linkCodes) {
		if strings.Contains(logOutput, forbidden) {
			t.Errorf("the server log carries %q", forbidden)
		}
	}
}

// bodyField is one value in a request body that pass 4 replaces with campaign B's.
type bodyField struct {
	path []string
	// kind is "uuid" (an ID), "key" (a role key) or "map" (an object keyed by resource keys).
	kind string
}

// mixedIDFields lists the body fields whose values name other rows; a field not classified here
// fails the sweep, so a new one gets a substitution (docs/design/multi-campaign.md §4.4, pass 4).
var mixedIDFields = map[string]string{
	"arrivalEntryId": "uuid",
	"groupId":        "uuid",
	"currentPlaceId": "uuid",
	"originPlaceId":  "uuid",
	"roleKey":        "key",
	"fromRoleKey":    "key",
	"toRoleKey":      "key",
	"deltas":         "map",
	"setTo":          "map",
}

func bodyFields(t *testing.T, label string, document map[string]any, schema map[string]any, prefix []string) []bodyField {
	t.Helper()
	schema = resolveSchema(document, schema)
	properties, _ := schema["properties"].(map[string]any)
	var fields []bodyField
	for name, property := range properties {
		propertySchema := resolveSchema(document, property.(map[string]any))
		path := append(slices.Clone(prefix), name)
		format, _ := propertySchema["format"].(string)
		_, isMap := propertySchema["additionalProperties"].(map[string]any)
		isKey := schemaType(propertySchema) == "string" && (strings.HasSuffix(name, "Key") || name == "key")
		switch {
		case format == "uuid", isMap, isKey:
			kind, classified := mixedIDFields[name]
			if !classified {
				t.Errorf("%s: body field %s names other rows but pass 4 doesn't classify it; add it to mixedIDFields", label, strings.Join(path, "."))
				continue
			}
			fields = append(fields, bodyField{path: path, kind: kind})
		case schemaType(propertySchema) == "object":
			fields = append(fields, bodyFields(t, label, document, propertySchema, path)...)
		}
	}
	return fields
}

func setBodyField(body map[string]any, path []string, value any) {
	for _, name := range path[:len(path)-1] {
		next, _ := body[name].(map[string]any)
		if next == nil {
			next = map[string]any{}
			body[name] = next
		}
		body = next
	}
	body[path[len(path)-1]] = value
}

// sweepMixedIDs is pass 4: A's DM calls every A operation with, one at a time, each other path
// parameter set to B's real row, and each body field that names a row set to B's (B's arrival, B's
// undo group, B's own resource key). Each must answer 404, 409 or 422 and leak nothing of B. Where a
// valid body exists, the same request with A's own values must succeed, so the refusal is the ID's.
func (testHarness *harness) sweepMixedIDs(t *testing.T, routes []sweepRoute, document map[string]any, forbidden []string) {
	t.Helper()
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	dmViewerA := testHarness.enter(testHarness.dmA, campaignA.id)
	dmViewerB := testHarness.enter(testHarness.dmB, campaignB.id)
	boardOf := func(viewer core.Viewer) core.ColonyBoard {
		board, err := testHarness.core.GetColonyBoard(testHarness.context, viewer)
		if err != nil {
			t.Fatalf("read a board: %v", err)
		}
		return board
	}
	lastGroupB := boardOf(dmViewerB)
	if lastGroupB.Dm == nil || lastGroupB.Dm.LastChange == nil {
		t.Fatal("campaign B has no undo slot to borrow")
	}
	valuesOfB := map[string]any{
		"arrivalEntryId": campaignB.arrivalEntryID.String(),
		"groupId":        lastGroupB.Dm.LastChange.GroupID.String(),
		"currentPlaceId": campaignB.keptPlaceID.String(),
		"originPlaceId":  campaignB.keptPlaceID.String(),
		"roleKey":        campaignB.pathIDs["roleKey"],
		"fromRoleKey":    campaignB.pathIDs["roleKey"],
		"toRoleKey":      campaignB.pathIDs["roleKey"],
	}
	keyOfB := campaignB.pathIDs["resourceKey"]
	roleKeyA := campaignA.pathIDs["roleKey"]
	npcBody := func(revealed bool) map[string]any {
		return map[string]any{
			"name": "Pass Four", "knownName": "", "revealed": revealed, "aliases": []string{}, "labels": []string{},
			"race": "", "characterClass": "", "level": 2, "age": "", "appearance": "", "personality": "",
			"backstory": "", "playerNotes": "", "dmNotes": "", "status": string(core.NpcStatusAlive),
			"health": string(core.NpcHealthHealthy), "relation": string(core.NpcRelationUnknown),
			"currentPlaceId": campaignA.keptPlaceID.String(), "originPlaceId": campaignA.keptPlaceID.String(),
		}
	}

	// Valid bodies in A, built at call time because they carry the colony's current version.
	validBodies := map[string]func() map[string]any{
		"/colony/adjust-preview": func() map[string]any {
			return map[string]any{
				"expectedVersion": boardOf(dmViewerA).Version, "source": string(core.AdjustSourceShipment),
				"arrivalEntryId": campaignA.arrivalEntryID.String(), "deltas": map[string]any{"gold": 1},
				"setTo": map[string]any{}, "colonists": 0, "reason": "", "public": false,
			}
		},
		"/colony/undo": func() map[string]any {
			board := boardOf(dmViewerA)
			if board.Dm == nil || board.Dm.LastChange == nil {
				t.Fatal("campaign A has no undo slot")
			}
			return map[string]any{"groupId": board.Dm.LastChange.GroupID.String()}
		},
	}
	validBodies["/colony/adjust"] = validBodies["/colony/adjust-preview"]
	validBodies["/npcs"] = func() map[string]any { return npcBody(false) }
	validBodies["/npcs/{npcId}"] = func() map[string]any { return npcBody(true) }
	// The {npcId} row is a named colonist, so join-colony has no valid body; change-role proves roleKey.
	validBodies["/npcs/{npcId}/change-role"] = func() map[string]any {
		return map[string]any{"expectedVersion": boardOf(dmViewerA).Version, "roleKey": "HEALER", "public": false}
	}
	validBodies["/people/transfer"] = func() map[string]any {
		return map[string]any{
			"expectedVersion": boardOf(dmViewerA).Version, "fromRoleKey": "UNASSIGNED", "toRoleKey": roleKeyA,
			"count": 1, "public": false,
		}
	}
	validBodies["/people/set-count"] = func() map[string]any {
		return map[string]any{"expectedVersion": boardOf(dmViewerA).Version, "roleKey": roleKeyA, "count": 2, "public": false}
	}
	validBodyFor := func(route sweepRoute) (func() map[string]any, bool) {
		suffix := strings.TrimPrefix(route.path, "/api/campaigns/{campaignId}")
		build, found := validBodies[suffix]
		return build, found
	}

	expectRefused := func(label, path string, response *httptest.ResponseRecorder) {
		t.Helper()
		checkResponse(t, label, path, forbidden, response)
		if !slices.Contains([]int{http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity}, response.Code) {
			t.Errorf("%s: status %d, want 404, 409 or 422; body: %s", label, response.Code, response.Body.String())
		}
	}

	substituted := map[string]bool{}
	for _, route := range campaignRoutesLeaveLast(routes) {
		if route.isStream() || route.isLeave() {
			continue
		}
		// Every other path parameter, one at a time, from B.
		for _, name := range pathParameter.FindAllString(route.path, -1) {
			name = strings.Trim(name, "{}")
			if name == campaignParameter {
				continue
			}
			pathIDs := maps.Clone(campaignA.pathIDs)
			pathIDs[name] = campaignB.pathIDs[name]
			path, response := testHarness.call(route, document, testHarness.signIn(testHarness.dmA), pathIDs, sampleBody(document, route.operation, uuid.New()))
			expectRefused("dmA: "+route.label+" with B's "+name, path, response)
		}
		if route.operation == nil || route.operation.requestBody == nil {
			continue
		}
		fields := bodyFields(t, route.label, document, requestBodySchema(route.operation.requestBody), nil)
		build, hasValidBody := validBodyFor(route)
		baseline := func() map[string]any {
			if hasValidBody {
				return build()
			}
			var sample map[string]any
			_ = json.Unmarshal([]byte(sampleBody(document, route.operation, uuid.New())), &sample)
			return sample
		}
		for _, field := range fields {
			body := baseline()
			fieldName := field.path[len(field.path)-1]
			substituted[fieldName] = true
			if field.kind == "map" {
				setBodyField(body, field.path, map[string]any{keyOfB: 1})
			} else {
				setBodyField(body, field.path, valuesOfB[fieldName])
			}
			encoded, _ := json.Marshal(body)
			path, response := testHarness.call(route, document, testHarness.signIn(testHarness.dmA), campaignA.pathIDs, string(encoded))
			expectRefused("dmA: "+route.label+" with B's "+strings.Join(field.path, "."), path, response)
		}
		if hasValidBody {
			encoded, _ := json.Marshal(build())
			path, response := testHarness.call(route, document, testHarness.signIn(testHarness.dmA), campaignA.pathIDs, string(encoded))
			checkResponse(t, "dmA: "+route.label+" with A's own values", path, forbidden, response)
			if response.Code < 200 || response.Code > 299 {
				t.Errorf("dmA: %s with A's own values answered %d, so pass 4 proves nothing; body: %s", route.label, response.Code, response.Body.String())
			}
		}
	}
	for fieldName := range mixedIDFields {
		if !substituted[fieldName] {
			t.Errorf("pass 4 found no body field %q to fill with B's value; the schema walk missed it", fieldName)
		}
	}
}

// fingerprint hashes a campaign's rows: every row of every table with a campaign_id column (found from
// information_schema, so new tables are covered), the campaign row, and the accounts whose only
// memberships are there (named up front, since deleting another campaign would add to them) with their
// sessions, leaving out the bookkeeping columns sign-in touches.
func (testHarness *harness) fingerprint(campaign *campaignFixture, onlyThere ...account) map[string]string {
	testHarness.t.Helper()
	pool := testHarness.database.Pool
	rows, err := pool.Query(testHarness.context, `SELECT DISTINCT table_name FROM information_schema.columns
		WHERE column_name = 'campaign_id' AND table_schema = current_schema() ORDER BY table_name`)
	if err != nil {
		testHarness.t.Fatalf("list campaign tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			testHarness.t.Fatalf("scan: %v", err)
		}
		tables = append(tables, table)
	}
	rows.Close()
	for _, required := range []string{"colony", "npc", "place", "role", "timeline_event"} {
		if !slices.Contains(tables, required) {
			testHarness.t.Fatalf("found only %v with a campaign_id column; %s is missing", tables, required)
		}
	}
	const rowsAsText = `SELECT coalesce(string_agg(row_text, '|' ORDER BY row_text), '') FROM (
		SELECT (to_jsonb(row_value) - 'last_seen_at' - 'expires_at')::text AS row_text FROM %s row_value WHERE %s) hashed`
	hashes := map[string]string{}
	hash := func(name, table, condition string, argument any) {
		var text string
		if err := pool.QueryRow(testHarness.context, fmt.Sprintf(rowsAsText, table, condition), argument).Scan(&text); err != nil {
			testHarness.t.Fatalf("fingerprint %s: %v", name, err)
		}
		hashes[name] = text
	}
	for _, table := range tables {
		hash(table, table, "campaign_id = $1", campaign.id)
	}
	hash("campaign", "campaign", "id = $1", campaign.id)
	accountIDs := make([]uuid.UUID, 0, len(onlyThere))
	for _, who := range onlyThere {
		memberships, err := testHarness.core.MyCampaigns(testHarness.context, who.caller)
		if err != nil || len(memberships.Campaigns) != 1 || memberships.Campaigns[0].Campaign.ID != campaign.id {
			testHarness.t.Fatalf("%s is not only in %s: %+v, %v", who.name, campaign.name, memberships, err)
		}
		accountIDs = append(accountIDs, who.caller.AccountID)
	}
	hash("B-only accounts", "account", "id = ANY($1)", accountIDs)
	hash("B-only accounts' sessions", "web_session", "account_id = ANY($1)", accountIDs)
	return hashes
}

// TestDmSeesTheCanaries proves the fixtures really hold DM-only data in both campaigns, so the sweep
// can fail.
func TestDmSeesTheCanaries(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	for _, side := range testHarness.sides() {
		token := testHarness.signIn(side.dm)
		secrets := testHarness.dmOnlyValues(side.campaign)
		for _, fixture := range secrecyFixtures {
			if len(fixture.dmProof) == 0 {
				t.Errorf("secrecy fixture %s names no GET path where the DM sees its values", fixture.feature)
			}
			for _, suffix := range fixture.dmProof {
				response := testHarness.do(http.MethodGet, campaignPathOf(side.campaign, suffix), requestOptions{token: token})
				testHarness.expectStatus(response, http.StatusOK)
				if !slices.ContainsFunc(secrets, func(secret string) bool { return strings.Contains(response.Body.String(), secret) }) {
					t.Errorf("%s in %s: the DM's %s lacks every DM-only value: %s", fixture.feature, side.campaign.name, suffix, response.Body.String())
				}
			}
		}
	}
}

// Exactly three campaign routes admit a membership the DM hasn't let in, or one in a paused campaign:
// the caller's own .../me, the stream (so the waiting page opens live) and leaving (decision C5).
func TestOnlyThreeRoutesAdmitPendingMembers(t *testing.T) {
	server := newTestServer(t)
	routes, _ := sweepRoutes(t, server)
	var pendingLevel []string
	for _, route := range routes {
		if route.access == AccessPending {
			pendingLevel = append(pendingLevel, route.label)
		}
	}
	slices.Sort(pendingLevel)
	want := []string{
		"GET /api/campaigns/{campaignId}/me",
		"GET /api/campaigns/{campaignId}/stream",
		"POST /api/campaigns/{campaignId}/leave",
	}
	if !slices.Equal(pendingLevel, want) {
		t.Fatalf("pending-level routes = %v, want %v", pendingLevel, want)
	}
}

// TestStreamMessagesCarryOnlyTopics replays every SSE message A's players, pending and removed members
// receive while A's DM works, against the tokens.
func TestStreamMessagesCarryOnlyTopics(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	campaignA := testHarness.campaignA
	dmViewer := testHarness.enter(testHarness.dmA, campaignA.id)
	forbidden := joinValues(testHarness.dmOnlyValues(campaignA), testHarness.dmOnlyValues(testHarness.campaignB), []string{canaryB})

	streams := map[string]*liveStream{
		"player":  testHarness.openStream(campaignA, testHarness.signIn(testHarness.ana), ""),
		"pending": testHarness.openStream(campaignA, testHarness.signIn(testHarness.pendingA), ""),
		"removed": testHarness.openStream(campaignA, testHarness.signIn(testHarness.removedA), ""),
	}
	for name, stream := range streams {
		if _, received := stream.nextMessage(2 * time.Second); !received {
			t.Fatalf("%s: no catch-up message on connect", name)
		}
	}

	secretName := canaryA + " name"
	if _, err := testHarness.core.UpdateSettings(testHarness.context, dmViewer, core.SettingsInput{CampaignName: "Campaign Alpha", ColonyName: secretName, Timezone: "UTC"}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	for _, accountName := range []string{"pendingA", "removedA"} {
		if _, err := testHarness.core.SetCharacterName(testHarness.context, dmViewer, campaignA.members[accountName], core.CharacterNameInput{CharacterName: &secretName}); err != nil {
			t.Fatalf("set character name: %v", err)
		}
	}

	known := core.AllTopics()
	for name, stream := range streams {
		var topics []core.Topic
		for {
			message, received := stream.nextMessage(300 * time.Millisecond)
			if !received {
				break
			}
			for _, secret := range forbidden {
				if strings.Contains(message.raw, secret) {
					t.Errorf("%s: a stream message leaks %q: %q", name, secret, message.raw)
				}
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
		if name != "player" && fmt.Sprint(topics) != fmt.Sprint([]core.Topic{core.TopicMe}) {
			t.Errorf("%s: should hear only its own me topic once, got %v", name, topics)
		}
	}
}

// TestPlayersFindOnlyWhatTheyKnow is the M2 sweep's targeted half, run as every kind of player of each
// campaign: a true name hidden behind a known name, and every value of a hidden NPC or place, finds
// nothing in search; filters return no hidden NPC; a hidden row's page reads exactly like a missing
// one; and /people counts a hidden named colonist without listing them. A search that wrongly matched
// a true name would answer with only the known name, which no canary catches, so this counts results.
func TestPlayersFindOnlyWhatTheyKnow(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	playersOf := map[*campaignFixture][]sweepCaller{
		campaignA: {
			{name: "ana", who: &testHarness.ana},
			{name: "dmB (DM of B)", who: &testHarness.dmB},
			{name: "dmA viewing as player", who: &testHarness.dmA, viewAsPlayer: true},
		},
		campaignB: {
			{name: "ana", who: &testHarness.ana},
			{name: "dmA (DM of A)", who: &testHarness.dmA},
			{name: "dmB viewing as player", who: &testHarness.dmB, viewAsPlayer: true},
		},
	}
	for _, side := range testHarness.sides() {
		campaign := side.campaign
		forbidden := joinValues(testHarness.dmOnlyValues(campaignA), testHarness.dmOnlyValues(campaignB))
		if campaign == campaignA {
			forbidden = append(forbidden, canaryB)
		}
		read := func(label, token, suffix string, into any, ownValues ...string) *httptest.ResponseRecorder {
			t.Helper()
			path := campaignPathOf(campaign, suffix)
			response := testHarness.do(http.MethodGet, path, requestOptions{token: token})
			checkResponse(t, label+": GET "+suffix, path, forbidden, response, ownValues...)
			if into != nil && response.Code == http.StatusOK {
				if err := json.Unmarshal(response.Body.Bytes(), into); err != nil {
					t.Fatalf("%s: decode %s: %v", label, suffix, err)
				}
			}
			return response
		}
		npcCount := func(label, token, suffix string) int {
			t.Helper()
			var list core.NpcList
			testHarness.expectStatus(read(label, token, suffix, &list), http.StatusOK)
			return len(list.Npcs)
		}

		dmViewer := testHarness.enter(side.dm, campaign.id)
		dmToken := testHarness.signIn(side.dm)
		// Controls: the DM finds what players must not, so a zero below is the projection's doing.
		for _, search := range []string{"Selwyn", "Mason", "Corrow", "Quiet"} {
			found, err := testHarness.core.ListNpcs(testHarness.context, dmViewer, core.NpcFilter{Search: search})
			if err != nil || len(found.Npcs) == 0 {
				t.Errorf("the DM of %s searching %q found no NPC (%v)", campaign.name, search, err)
			}
		}
		dmWorkforce, err := testHarness.core.GetWorkforce(testHarness.context, dmViewer)
		if err != nil {
			t.Fatalf("the DM's /people: %v", err)
		}
		dmTotals := map[string]int64{}
		for _, role := range dmWorkforce.Roles {
			dmTotals[role.Key] = role.Total
		}
		missingNpc := testHarness.do(http.MethodGet, campaignPathOf(campaign, "/npcs/"+uuid.NewString()), requestOptions{token: dmToken})
		missingPlace := testHarness.do(http.MethodGet, campaignPathOf(campaign, "/places/"+uuid.NewString()), requestOptions{token: dmToken})

		for _, caller := range playersOf[campaign] {
			token := testHarness.tokenFor(caller, campaign)
			label := caller.name + " in " + campaign.name
			for _, search := range []string{"Selwyn", "Mason", "Corrow", "Quiet", side.dmSecret} {
				if count := npcCount(label, token, "/npcs?search="+url.QueryEscape(search)); count != 0 {
					t.Errorf("%s: searching %q found %d NPCs, want none", label, search, count)
				}
			}
			for _, search := range []string{"Hooded", "Hood"} {
				if count := npcCount(label, token, "/npcs?search="+search); count != 1 {
					t.Errorf("%s: searching %q found %d NPCs, want the revealed one", label, search, count)
				}
			}
			for _, filter := range []string{
				"/npcs?revealed=false",
				"/npcs?colonistsOnly=true",
				"/npcs?roleKey=" + url.QueryEscape(campaign.pathIDs["roleKey"]),
				"/npcs?placeId=" + campaign.pathIDs["placeId"],
				"/npcs?status=MISSING",
			} {
				testHarness.expectStatus(read(label, token, filter, nil), http.StatusOK)
			}
			for _, search := range []string{"Chapel", side.dmSecret} {
				var places core.PlaceList
				testHarness.expectStatus(read(label, token, "/places?search="+url.QueryEscape(search), &places), http.StatusOK)
				if len(places.Places) != 0 {
					t.Errorf("%s: searching places for %q found %d, want none", label, search, len(places.Places))
				}
			}

			hiddenNpcID, hiddenPlaceID := campaign.hiddenNpcID.String(), campaign.keptPlaceID.String()
			for _, hiddenRow := range []struct {
				suffix  string
				id      string
				missing *httptest.ResponseRecorder
			}{
				{"/npcs/" + hiddenNpcID, hiddenNpcID, missingNpc},
				{"/places/" + hiddenPlaceID, hiddenPlaceID, missingPlace},
			} {
				response := read(label, token, hiddenRow.suffix, nil, hiddenRow.id)
				if response.Code != http.StatusNotFound || response.Body.String() != hiddenRow.missing.Body.String() {
					t.Errorf("%s: a hidden row answered %d %s; a missing one answers %d %s", label, response.Code, response.Body.String(), hiddenRow.missing.Code, hiddenRow.missing.Body.String())
				}
			}

			var workforce core.Workforce
			testHarness.expectStatus(read(label, token, "/people", &workforce), http.StatusOK)
			if workforce.Colonists != dmWorkforce.Colonists {
				t.Errorf("%s: /people counts %d colonists, the DM %d", label, workforce.Colonists, dmWorkforce.Colonists)
			}
			for _, role := range workforce.Roles {
				if role.Total != dmTotals[role.Key] {
					t.Errorf("%s: %s totals %d, the DM's %d", label, role.Key, role.Total, dmTotals[role.Key])
				}
				if role.UnnamedCount+role.NamedCount != role.Total || int64(len(role.NamedColonists)) != role.NamedCount {
					t.Errorf("%s: %s's counts don't add up: %+v", label, role.Key, role)
				}
			}
		}
	}
}
