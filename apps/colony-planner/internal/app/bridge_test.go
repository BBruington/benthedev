package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/BBruington/colony-planner/internal/core"
)

const (
	bridgeOriginA = "https://alphaworld.forge-vtt.com"
	bridgeOriginB = "https://betaworld.forge-vtt.com"
)

// pairOverHTTP is the whole pairing as the site and the module make it: the DM's code from Settings,
// then the module's POST /api/bridge/pair from the Forge page. It returns the token.
func (testHarness *harness) pairOverHTTP(campaign *campaignFixture, dm account, origin string) string {
	testHarness.t.Helper()
	created := testHarness.do(http.MethodPost, campaignPathOf(campaign, "/bridge/codes"), requestOptions{token: testHarness.signIn(dm), body: `{}`})
	testHarness.expectStatus(created, http.StatusCreated)
	var code core.BridgePairingCode
	if err := json.Unmarshal(created.Body.Bytes(), &code); err != nil {
		testHarness.t.Fatalf("decode the code: %v", err)
	}
	paired := testHarness.do(http.MethodPost, bridgePairPath, requestOptions{
		body:    `{"code":"` + code.Code + `"}`,
		headers: map[string]string{"Origin": origin, "Sec-Fetch-Site": "cross-site"},
	})
	testHarness.expectStatus(paired, http.StatusOK)
	if paired.Header().Get("Access-Control-Allow-Origin") != origin {
		testHarness.t.Fatalf("the pairing answer is unreadable from %s: %v", origin, paired.Header())
	}
	var pairing core.BridgePairing
	if err := json.Unmarshal(paired.Body.Bytes(), &pairing); err != nil || pairing.Token == "" || pairing.CampaignName != campaign.name {
		testHarness.t.Fatalf("pairing = %+v, %v", pairing, err)
	}
	return pairing.Token
}

func bridgeHeaders(token, origin string) map[string]string {
	headers := map[string]string{"Origin": origin, "Sec-Fetch-Site": "cross-site"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}

// bridgeTokenRoutes are every token-level bridge route with a body it accepts.
var bridgeTokenRoutes = []struct{ method, path, body string }{
	{http.MethodGet, "/api/bridge/outbox?after=0", ""},
	{http.MethodPost, "/api/bridge/acks", `{"through":0}`},
	{http.MethodPost, "/api/bridge/events", `{"events":[{"id":"6f8d8b0e-0a59-4a4b-9bb5-1d1f2a6c5e01","kind":"TIME_ADVANCED","occurredAt":"2026-11-12T10:00:00Z","payload":{"days":1}}]}`},
	{http.MethodGet, "/api/bridge/stream", ""},
}

func (testHarness *harness) callBridge(method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	testHarness.t.Helper()
	options := requestOptions{body: body, headers: headers}
	if strings.HasSuffix(path, "/stream") {
		streamContext, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		options.context = streamContext
	}
	return testHarness.do(method, path, options)
}

func expectBridgeUnauthorized(t *testing.T, label string, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusUnauthorized {
		t.Errorf("%s: status %d, want 401; body: %s", label, response.Code, response.Body.String())
		return
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["title"] != "Unauthorized" || body["detail"] != nil {
		t.Errorf("%s: body %s, want a bare Unauthorized", label, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("%s: a 401 allowed an origin", label)
	}
}

func TestEveryBadTokenIsUnauthorized(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.pairOverHTTP(testHarness.campaignA, testHarness.dmA, bridgeOriginA)
	replaced := testHarness.pairOverHTTP(testHarness.campaignA, testHarness.dmA, bridgeOriginA)
	revokedB := testHarness.pairOverHTTP(testHarness.campaignB, testHarness.dmB, bridgeOriginB)
	testHarness.expectStatus(testHarness.do(http.MethodDelete, campaignPathOf(testHarness.campaignB, "/bridge"), requestOptions{token: testHarness.signIn(testHarness.dmB)}), http.StatusNoContent)
	bad := map[string]map[string]string{
		"no header":        {"Origin": bridgeOriginA},
		"no token":         {"Origin": bridgeOriginA, "Authorization": "Bearer"},
		"another scheme":   {"Origin": bridgeOriginA, "Authorization": "Basic " + replaced},
		"malformed":        bridgeHeaders("not-a-token", bridgeOriginA),
		"unknown":          bridgeHeaders(strings.Repeat("A", 43), bridgeOriginA),
		"replaced by pair": bridgeHeaders(token, bridgeOriginA),
		"revoked":          bridgeHeaders(revokedB, bridgeOriginB),
	}
	for name, headers := range bad {
		for _, route := range append(bridgeTokenRoutes, struct{ method, path, body string }{http.MethodGet, "/api/bridge/nowhere", ""}) {
			expectBridgeUnauthorized(t, name+": "+route.method+" "+route.path, testHarness.callBridge(route.method, route.path, route.body, headers))
		}
	}
	for _, route := range bridgeTokenRoutes {
		response := testHarness.callBridge(route.method, route.path, route.body, bridgeHeaders(replaced, bridgeOriginA))
		if response.Code >= 300 {
			t.Errorf("the live token: %s %s = %d; body: %s", route.method, route.path, response.Code, response.Body.String())
		}
	}
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/admin/campaigns/"+testHarness.campaignA.id.String()+"/suspend", requestOptions{token: testHarness.signIn(testHarness.owner)}), http.StatusOK)
	for _, route := range bridgeTokenRoutes {
		expectBridgeUnauthorized(t, "suspended: "+route.method+" "+route.path, testHarness.callBridge(route.method, route.path, route.body, bridgeHeaders(replaced, bridgeOriginA)))
	}
}

func TestTheSessionCookieMeansNothingUnderTheBridge(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.pairOverHTTP(testHarness.campaignA, testHarness.dmA, bridgeOriginA)
	dmCookie := testHarness.signIn(testHarness.dmA)
	// A signed-in DM's cookie alone opens nothing.
	expectBridgeUnauthorized(t, "cookie only", testHarness.do(http.MethodGet, "/api/bridge/outbox", requestOptions{token: dmCookie}))
	// A cookie that names no session is left alone there, while anywhere else it is cleared, which shows
	// the middleware never read it.
	stale := testHarness.do(http.MethodGet, "/api/bridge/outbox", requestOptions{token: "stale-cookie", headers: bridgeHeaders(token, bridgeOriginA)})
	testHarness.expectStatus(stale, http.StatusOK)
	if cookies := stale.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("the bridge touched the session cookie: %v", cookies)
	}
	elsewhere := testHarness.do(http.MethodGet, "/api/me", requestOptions{token: "stale-cookie"})
	if len(elsewhere.Header().Values("Set-Cookie")) == 0 {
		t.Fatal("outside the bridge a stale cookie should be cleared, so the check above proves nothing")
	}

	// CrossOriginProtection is bypassed for the bridge only: a cross-site POST reaches the bridge, while
	// the same request to a cookie route is refused.
	crossSite := map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}
	events := testHarness.do(http.MethodPost, "/api/bridge/events", requestOptions{
		body:    `{"events":[{"id":"7f8d8b0e-0a59-4a4b-9bb5-1d1f2a6c5e02","kind":"TIME_ADVANCED","occurredAt":"2026-11-12T10:00:00Z","payload":{"days":2}}]}`,
		headers: map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site", "Authorization": "Bearer " + token},
	})
	testHarness.expectStatus(events, http.StatusOK)
	if events.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("another origin may read a bridge answer: %v", events.Header())
	}
	for _, path := range []string{campaignPathOf(testHarness.campaignA, "/bridge/codes"), "/api/bridgex", "/api/campaigns/" + testHarness.campaignA.id.String() + "/bridge"} {
		response := testHarness.do(http.MethodPost, path, requestOptions{token: dmCookie, body: `{}`, headers: crossSite})
		if response.Code != http.StatusForbidden {
			t.Errorf("a cross-site POST to %s answered %d, want 403", path, response.Code)
		}
	}
	// Dot segments and escaped slashes never carry a cookie through the bypass to a cookie route.
	campaignID := testHarness.campaignA.id.String()
	for _, path := range []string{
		"/api/bridge/../campaigns/" + campaignID + "/bridge/codes",
		"/api/campaigns/" + campaignID + "/../../bridge/events",
		"/api/bridge%2F..%2Fcampaigns%2F" + campaignID + "%2Fbridge%2Fcodes",
	} {
		response := testHarness.do(http.MethodPost, path, requestOptions{token: dmCookie, body: `{}`, headers: crossSite})
		if response.Code >= 200 && response.Code < 300 {
			t.Errorf("a cross-site POST to %s with the DM's cookie answered %d", path, response.Code)
		}
	}
	status, err := testHarness.core.GetBridgeStatus(testHarness.context, testHarness.enter(testHarness.dmA, testHarness.campaignA.id))
	if err != nil || status.Code != nil {
		t.Fatalf("a path trick made a pairing code: %+v, %v", status.Code, err)
	}
}

func TestBridgeCORS(t *testing.T) {
	testHarness := newHarness(t)
	tokenA := testHarness.pairOverHTTP(testHarness.campaignA, testHarness.dmA, bridgeOriginA)
	testHarness.pairOverHTTP(testHarness.campaignB, testHarness.dmB, bridgeOriginB)
	preflight := func(path, origin string) *httptest.ResponseRecorder {
		return testHarness.do(http.MethodOptions, path, requestOptions{headers: map[string]string{
			"Origin": origin, "Access-Control-Request-Method": "GET", "Access-Control-Request-Headers": "authorization",
		}})
	}
	allowed := preflight("/api/bridge/outbox", bridgeOriginA)
	testHarness.expectStatus(allowed, http.StatusNoContent)
	header := allowed.Header()
	if header.Get("Access-Control-Allow-Origin") != bridgeOriginA || header.Get("Access-Control-Max-Age") != "7200" ||
		header.Get("Access-Control-Allow-Headers") != "Authorization, Content-Type" || header.Get("Access-Control-Allow-Credentials") != "" ||
		!strings.Contains(header.Get("Vary"), "Origin") {
		t.Fatalf("preflight headers = %v", header)
	}
	for _, origin := range []string{"https://unlinked.forge-vtt.com", "https://evil.example", ""} {
		refused := preflight("/api/bridge/outbox", origin)
		if refused.Code != http.StatusForbidden || refused.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("preflight from %q = %d %v", origin, refused.Code, refused.Header())
		}
	}
	// Pairing comes before any link, so its preflight answers any origin a module may pair from.
	testHarness.expectStatus(preflight(bridgePairPath, "https://unlinked.forge-vtt.com"), http.StatusNoContent)
	if refused := preflight(bridgePairPath, "https://evil.example"); refused.Code != http.StatusForbidden {
		t.Errorf("a pairing preflight from another site = %d", refused.Code)
	}

	// A real answer allows only the token's own link origin, even another active link's.
	own := testHarness.do(http.MethodGet, "/api/bridge/outbox", requestOptions{headers: bridgeHeaders(tokenA, bridgeOriginA)})
	testHarness.expectStatus(own, http.StatusOK)
	if own.Header().Get("Access-Control-Allow-Origin") != bridgeOriginA || own.Header().Get("Access-Control-Allow-Credentials") != "" || !strings.Contains(own.Header().Get("Vary"), "Origin") {
		t.Fatalf("own origin headers = %v", own.Header())
	}
	other := testHarness.do(http.MethodGet, "/api/bridge/outbox", requestOptions{headers: bridgeHeaders(tokenA, bridgeOriginB)})
	testHarness.expectStatus(other, http.StatusOK)
	if other.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("B's origin may read A's answer: %v", other.Header())
	}
	noOrigin := testHarness.do(http.MethodPost, bridgePairPath, requestOptions{body: `{"code":"AAA-AAA"}`})
	testHarness.expectStatus(noOrigin, http.StatusForbidden)
	wrongCode := testHarness.do(http.MethodPost, bridgePairPath, requestOptions{body: `{"code":"AAA-AAA"}`, headers: map[string]string{"Origin": bridgeOriginA}})
	testHarness.expectStatus(wrongCode, http.StatusBadRequest)
	if !strings.Contains(wrongCode.Body.String(), "That code didn't work.") || wrongCode.Header().Get("Access-Control-Allow-Origin") != bridgeOriginA {
		t.Fatalf("a wrong code answered %s with %v", wrongCode.Body.String(), wrongCode.Header())
	}
}

func TestBridgeRateLimitsAndBodyCap(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.pairOverHTTP(testHarness.campaignA, testHarness.dmA, bridgeOriginA)
	for count := 1; count <= bridgeLinkRequestsPerMinute; count++ {
		response := testHarness.do(http.MethodGet, "/api/bridge/outbox", requestOptions{headers: bridgeHeaders(token, bridgeOriginA)})
		if response.Code != http.StatusOK {
			t.Fatalf("request %d answered %d", count, response.Code)
		}
	}
	limited := testHarness.do(http.MethodGet, "/api/bridge/outbox", requestOptions{headers: bridgeHeaders(token, bridgeOriginA)})
	testHarness.expectStatus(limited, http.StatusTooManyRequests)
	if seconds, err := strconv.Atoi(limited.Header().Get("Retry-After")); err != nil || seconds < 1 || seconds > 60 {
		t.Fatalf("Retry-After = %q", limited.Header().Get("Retry-After"))
	}
	if limited.Header().Get("Access-Control-Allow-Origin") != bridgeOriginA {
		t.Fatal("the module can't read its own 429")
	}

	pair := func(address string) int {
		return testHarness.do(http.MethodPost, bridgePairPath, requestOptions{
			body: `{"code":"AAA-AAA"}`, headers: map[string]string{"Origin": bridgeOriginA, "X-Forwarded-For": "198.51.100.7, " + address},
		}).Code
	}
	for count := 1; count <= bridgePairRequestsPerMinute; count++ {
		if status := pair("203.0.113.9"); status != http.StatusBadRequest {
			t.Fatalf("pair %d answered %d", count, status)
		}
	}
	if status := pair("203.0.113.9"); status != http.StatusTooManyRequests {
		t.Fatalf("pair past the limit answered %d", status)
	}
	if status := pair("203.0.113.10"); status != http.StatusBadRequest {
		t.Fatalf("another address answered %d", status)
	}

	other := testHarness.pairOverHTTP(testHarness.campaignB, testHarness.dmB, bridgeOriginB)
	tooLarge := `{"events":[],"padding":"` + strings.Repeat("x", bridgeMaxBodyBytes) + `"}`
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/bridge/events", requestOptions{body: tooLarge, headers: bridgeHeaders(other, bridgeOriginB)}), http.StatusRequestEntityTooLarge)
	chunked := httptest.NewRequest(http.MethodPost, "/api/bridge/events", strings.NewReader(tooLarge))
	chunked.Host = testHost
	chunked.ContentLength = -1
	chunked.Header.Set("Content-Type", "application/json")
	for name, value := range bridgeHeaders(other, bridgeOriginB) {
		chunked.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	testHarness.handler.ServeHTTP(recorder, chunked)
	testHarness.expectStatus(recorder, http.StatusRequestEntityTooLarge)
}

func TestTheBridgeStream(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.pairOverHTTP(testHarness.campaignA, testHarness.dmA, bridgeOriginA)
	server := httptest.NewServer(testHarness.handler)
	t.Cleanup(server.Close)
	streamContext, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(streamContext, http.MethodGet, server.URL+"/api/bridge/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range bridgeHeaders(token, bridgeOriginA) {
		request.Header.Set(name, value)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("open the bridge stream: %v", err)
	}
	if response.Header.Get("Access-Control-Allow-Origin") != bridgeOriginA || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream headers = %v", response.Header)
	}
	stream := readStream(response, cancel)
	outboxSeq := func() int64 {
		t.Helper()
		message, received := stream.nextMessage(2 * time.Second)
		if !received || message.event != "outbox" {
			t.Fatalf("no outbox message: %+v", message)
		}
		var data map[string]int64
		if err := json.Unmarshal([]byte(message.data), &data); err != nil {
			t.Fatalf("data %q: %v", message.data, err)
		}
		return data["seq"]
	}
	first := outboxSeq()
	board, err := testHarness.core.GetColonyBoard(testHarness.context, testHarness.enter(testHarness.dmA, testHarness.campaignA.id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testHarness.core.Advance(testHarness.context, testHarness.enter(testHarness.dmA, testHarness.campaignA.id), core.AdvanceInput{Days: 1, ExpectedVersion: board.Version}); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if next := outboxSeq(); next <= first {
		t.Fatalf("seq after an advance = %d, want more than %d", next, first)
	}
	// Pings keep it open; acknowledging everything sends nothing new.
	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/bridge/acks", requestOptions{body: `{"through":1000}`, headers: bridgeHeaders(token, bridgeOriginA)}), http.StatusNoContent)
	sawPing := false
	deadline := time.After(500 * time.Millisecond)
	for !sawPing {
		select {
		case event := <-stream.events:
			if event.event != "" {
				t.Fatalf("an unexpected message after the ack: %+v", event)
			}
			sawPing = event.comment == "ping"
		case <-deadline:
			t.Fatal("no ping")
		}
	}
	testHarness.expectStatus(testHarness.do(http.MethodDelete, campaignPathOf(testHarness.campaignA, "/bridge"), requestOptions{token: testHarness.signIn(testHarness.dmA)}), http.StatusNoContent)
	if !stream.waitForEnd(2 * time.Second) {
		t.Fatal("revoking left the stream open")
	}
}

func TestTheModuleIsServed(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, entry := range []struct{ name, content string }{
		{"module.json", `{"id":"colony-bridge","version":"0.3.1","manifest":"","download":""}`},
		{"scripts/colony-bridge.js", "export {};"},
	} {
		file, err := writer.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.Write([]byte(entry.content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	module, err := buildServedModule(fstest.MapFS{
		"dist/module.json":       {Data: []byte(`{"id":"colony-bridge","version":"0.3.1","manifest":"","download":""}`)},
		"dist/colony-bridge.zip": {Data: archive.Bytes()},
	}, "https://colony.example/")
	if err != nil {
		t.Fatalf("build the served module: %v", err)
	}
	manifest := httptest.NewRecorder()
	foundryManifest(module).ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, foundryManifestPath, nil))
	var decoded map[string]string
	if err := json.Unmarshal(manifest.Body.Bytes(), &decoded); err != nil || manifest.Code != http.StatusOK {
		t.Fatalf("manifest = %d %s, %v", manifest.Code, manifest.Body.String(), err)
	}
	if decoded["manifest"] != "https://colony.example/foundry/module.json" || decoded["download"] != "https://colony.example/foundry/colony-bridge.zip" || decoded["version"] != "0.3.1" {
		t.Fatalf("manifest = %v", decoded)
	}

	// The zip Foundry installs carries the same manifest, so the installed module finds this server.
	served := httptest.NewRecorder()
	foundryZip(module).ServeHTTP(served, httptest.NewRequest(http.MethodGet, foundryZipPath, nil))
	if served.Code != http.StatusOK || served.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("zip = %d %v", served.Code, served.Header())
	}
	reader, err := zip.NewReader(bytes.NewReader(served.Body.Bytes()), int64(served.Body.Len()))
	if err != nil || len(reader.File) != 2 {
		t.Fatalf("served zip: %v, %d entries", err, len(reader.File))
	}
	contents := map[string]string{}
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		read, _ := io.ReadAll(opened)
		_ = opened.Close()
		contents[file.Name] = string(read)
	}
	if contents["module.json"] != manifest.Body.String() || contents["scripts/colony-bridge.js"] != "export {};" {
		t.Fatalf("the zip's entries = %v, want the served manifest and the script unchanged", contents)
	}

	empty, err := buildServedModule(fstest.MapFS{"dist/.gitkeep": {}}, "https://colony.example")
	if err != nil {
		t.Fatal(err)
	}
	for name, handler := range map[string]http.Handler{"manifest": foundryManifest(empty), "zip": foundryZip(empty)} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "no Foundry module") {
			t.Errorf("%s without a build = %d %s", name, recorder.Code, recorder.Body.String())
		}
	}
}
