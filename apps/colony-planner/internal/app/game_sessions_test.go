package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

var calendarInstant = regexp.MustCompile(`^(DTSTART|DTEND|DTSTAMP):\d{8}T\d{6}Z$`)

// checkCalendarText holds the feed to RFC 5545's framing: CRLF line endings, content lines folded
// at 75 octets, and every instant in UTC.
func checkCalendarText(t *testing.T, label string, text string) {
	t.Helper()
	if !strings.HasPrefix(text, "BEGIN:VCALENDAR\r\n") || !strings.HasSuffix(text, "END:VCALENDAR\r\n") {
		t.Errorf("%s: not one VCALENDAR: %q", label, text)
	}
	lines := strings.Split(strings.TrimSuffix(text, "\r\n"), "\r\n")
	for _, line := range lines {
		if strings.ContainsAny(line, "\r\n") {
			t.Errorf("%s: a bare line break in %q", label, line)
		}
		if len(line) > 75 {
			t.Errorf("%s: a %d-octet line %q", label, len(line), line)
		}
		name, _, _ := strings.Cut(line, ":")
		if (name == "DTSTART" || name == "DTEND" || name == "DTSTAMP") && !calendarInstant.MatchString(line) {
			t.Errorf("%s: %q isn't a UTC instant", label, line)
		}
	}
	if strings.Count(text, "BEGIN:VEVENT") == 0 {
		t.Errorf("%s: no events, so the checks above proved little: %q", label, text)
	}
}

// The ICS feed is token level (spec §5.11, docs/design/multi-campaign.md §2.6): no session cookie,
// one campaign's schedule per token, and a wrong, regenerated or paused campaign's token answers
// exactly like a path that was never a route.
func TestCalendarFeedServesOnlyItsTokensCampaign(t *testing.T) {
	testHarness := newHarness(t)
	plantCanaries(t, testHarness)
	campaignA, campaignB := testHarness.campaignA, testHarness.campaignB
	originalTokenA := campaignA.icsToken
	forbiddenInA := joinValues(testHarness.dmOnlyValues(campaignA), testHarness.dmOnlyValues(campaignB), testHarness.visibleValuesOfB())

	missing := testHarness.do(http.MethodGet, "/calendar/never/a-route", requestOptions{})
	testHarness.expectStatus(missing, http.StatusNotFound)
	readsAsMissing := func(label string, response *httptest.ResponseRecorder) {
		t.Helper()
		if response.Code != missing.Code || response.Body.String() != missing.Body.String() ||
			response.Header().Get("Content-Type") != missing.Header().Get("Content-Type") ||
			response.Header().Get("Cache-Control") != missing.Header().Get("Cache-Control") {
			t.Errorf("%s: %d %q (%s), want the missing route's %d %q (%s)", label, response.Code, response.Body.String(),
				response.Header().Get("Content-Type"), missing.Code, missing.Body.String(), missing.Header().Get("Content-Type"))
		}
	}
	feed := func(token string) *httptest.ResponseRecorder {
		return testHarness.do(http.MethodGet, "/calendar/"+token+".ics", requestOptions{})
	}

	feedA := feed(campaignA.icsToken)
	testHarness.expectStatus(feedA, http.StatusOK)
	if contentType := feedA.Header().Get("Content-Type"); contentType != "text/calendar; charset=utf-8" {
		t.Errorf("the feed's Content-Type = %q", contentType)
	}
	if cacheControl := feedA.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Errorf("the feed's Cache-Control = %q, want no-store", cacheControl)
	}
	checkResponse(t, "A's feed", "/calendar/", forbiddenInA, feedA)
	checkCalendarText(t, "A's feed", feedA.Body.String())
	if !strings.Contains(feedA.Body.String(), "SUMMARY:Friday Game\r\n") {
		t.Errorf("A's feed lacks A's series: %q", feedA.Body.String())
	}
	feedB := feed(campaignB.icsToken)
	testHarness.expectStatus(feedB, http.StatusOK)
	checkCalendarText(t, "B's feed", feedB.Body.String())
	if !strings.Contains(feedB.Body.String(), canaryB) {
		t.Errorf("B's feed lacks B's series, so A's feed lacking canaryB proves nothing: %q", feedB.Body.String())
	}

	readsAsMissing("an unknown token", feed(strings.Repeat("x", 43)))
	readsAsMissing("a token without .ics", testHarness.do(http.MethodGet, "/calendar/"+campaignA.icsToken, requestOptions{}))
	readsAsMissing("a token over 64 characters", feed(strings.Repeat("y", 65)))
	readsAsMissing("an empty token", feed(""))
	readsAsMissing("a token that isn't UTF-8", feed("%FF"))
	readsAsMissing("a token with a NUL byte", feed("%00"))
	readsAsMissing("a token-length run that isn't UTF-8", feed(strings.Repeat("%FF", 43)))

	// Every active member reads the link; a non-member reads no campaign.
	for _, who := range []account{testHarness.dmA, testHarness.ana, testHarness.dmB} {
		response := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/calendar"), requestOptions{token: testHarness.signIn(who)})
		testHarness.expectStatus(response, http.StatusOK)
		var link core.CalendarLink
		if err := json.Unmarshal(response.Body.Bytes(), &link); err != nil || link.Path != "/calendar/"+campaignA.icsToken+".ics" {
			t.Errorf("%s's calendar link = %+v, %v", who.name, link, err)
		}
	}
	nonMember := testHarness.do(http.MethodGet, campaignPathOf(campaignA, "/calendar"), requestOptions{token: testHarness.signIn(testHarness.bOnly)})
	testHarness.expectStatus(nonMember, http.StatusNotFound)

	regenerated := testHarness.do(http.MethodPost, campaignPathOf(campaignA, "/calendar/regenerate"), requestOptions{token: testHarness.signIn(testHarness.dmA)})
	testHarness.expectStatus(regenerated, http.StatusOK)
	var link core.CalendarLink
	if err := json.Unmarshal(regenerated.Body.Bytes(), &link); err != nil {
		t.Fatalf("decode the new link: %v", err)
	}
	newTokenA := strings.TrimSuffix(strings.TrimPrefix(link.Path, "/calendar/"), ".ics")
	if newTokenA == originalTokenA || newTokenA == "" {
		t.Fatalf("regenerating kept the token: %q", link.Path)
	}
	readsAsMissing("a regenerated-away token", feed(originalTokenA))
	testHarness.expectStatus(feed(newTokenA), http.StatusOK)
	testHarness.expectStatus(feed(campaignB.icsToken), http.StatusOK)

	testHarness.expectStatus(testHarness.do(http.MethodPost, "/api/admin/campaigns/"+campaignA.id.String()+"/suspend", requestOptions{token: testHarness.signIn(testHarness.owner)}), http.StatusOK)
	readsAsMissing("a paused campaign's token", feed(newTokenA))

	// An ICS token reads a schedule, so the request log names the route, never the token.
	logOutput := testHarness.logs.String()
	if !strings.Contains(logOutput, "/calendar/{icsToken}") {
		t.Errorf("the feed's requests weren't logged: the check below would prove nothing")
	}
	for _, token := range []string{originalTokenA, newTokenA, campaignB.icsToken} {
		if strings.Contains(logOutput, token) {
			t.Errorf("the server log carries the ICS token %q", token)
		}
	}
}

// The session reads every member makes on the board answer null, never an empty body, when nothing is
// planned, so a campaign without sessions still renders.
func TestBoardSessionReadsAnswerNullWhenNothingIsPlanned(t *testing.T) {
	testHarness := newHarness(t)
	token := testHarness.signIn(testHarness.ana)
	for _, suffix := range []string{"/sessions/next", "/sessions/latest-recap"} {
		response := testHarness.do(http.MethodGet, campaignPathOf(testHarness.campaignA, suffix), requestOptions{token: token})
		testHarness.expectStatus(response, http.StatusOK)
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v; body %q", suffix, err, response.Body.String())
		}
		if session, present := body["session"]; !present || session != nil {
			t.Errorf("%s = %s, want {\"session\":null}", suffix, response.Body.String())
		}
	}
	missing := testHarness.do(http.MethodGet, campaignPathOf(testHarness.campaignA, "/sessions/"+uuid.NewString()), requestOptions{token: token})
	testHarness.expectStatus(missing, http.StatusNotFound)
}

// The event location is a required field like the rest of Settings (full-record PUT): a body that
// leaves it out is refused instead of resetting the location the DM set, and a blank one means Online.
func TestSettingsKeepTheEventLocationABodyLeavesOut(t *testing.T) {
	testHarness := newHarness(t)
	settingsPath := campaignPathOf(testHarness.campaignA, "/settings")
	dmToken := testHarness.signIn(testHarness.dmA)
	location := func() string {
		t.Helper()
		response := testHarness.do(http.MethodGet, settingsPath, requestOptions{token: dmToken})
		testHarness.expectStatus(response, http.StatusOK)
		var settings core.Settings
		if err := json.Unmarshal(response.Body.Bytes(), &settings); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		return settings.DiscordEventLocation
	}
	if got := location(); got != "Online" {
		t.Fatalf("a new campaign's event location = %q, want Online", got)
	}
	const withoutLocation = `{"campaignName":"Campaign Alpha","colonyName":"Port Alpha","timezone":"UTC","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null`
	testHarness.expectStatus(testHarness.do(http.MethodPut, settingsPath, requestOptions{token: dmToken, body: withoutLocation + `,"discordEventLocation":"Foundry, table 2"}`}), http.StatusOK)
	testHarness.expectStatus(testHarness.do(http.MethodPut, settingsPath, requestOptions{token: dmToken, body: withoutLocation + `}`}), http.StatusUnprocessableEntity)
	if got := location(); got != "Foundry, table 2" {
		t.Fatalf("after a body without it the event location = %q, want the DM's", got)
	}
	testHarness.expectStatus(testHarness.do(http.MethodPut, settingsPath, requestOptions{token: dmToken, body: withoutLocation + `,"discordEventLocation":"  "}`}), http.StatusOK)
	if got := location(); got != "Online" {
		t.Fatalf("a blank event location = %q, want Online", got)
	}
}

// The table's time zone has no secret: players read it on the board, so they see the table's clock by
// name rather than as a UTC offset.
func TestPlayersReadTheTablesTimeZoneOnTheBoard(t *testing.T) {
	testHarness := newHarness(t)
	settingsPath := campaignPathOf(testHarness.campaignA, "/settings")
	body := `{"campaignName":"Campaign Alpha","colonyName":"Port Alpha","timezone":"America/New_York","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null,"discordEventLocation":"Online"}`
	testHarness.expectStatus(testHarness.do(http.MethodPut, settingsPath, requestOptions{token: testHarness.signIn(testHarness.dmA), body: body}), http.StatusOK)
	response := testHarness.do(http.MethodGet, campaignPathOf(testHarness.campaignA, "/colony"), requestOptions{token: testHarness.signIn(testHarness.ana)})
	testHarness.expectStatus(response, http.StatusOK)
	var board core.ColonyBoard
	if err := json.Unmarshal(response.Body.Bytes(), &board); err != nil || board.Timezone != "America/New_York" {
		t.Fatalf("a player's board time zone = %q, %v", board.Timezone, err)
	}
}
