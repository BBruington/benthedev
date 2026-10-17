package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An invite code lets anyone ask to join and an ICS token reads a campaign's schedule, and only the
// campaign may read them, so the request log (the operator's view) names the route, not the secret
// (Invariant 14).
func TestRequestLogKeepsInviteCodesOut(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	handler := logRequests(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}), logger)
	const inviteCode = "-zZOCQTWaiKX0GF2o3SnTg"
	const icsToken = "q2Vb7mJx0dYc9LwPz4RtKe1NhUa8SfGi3Oo6Xy5Bn_A"
	for _, path := range []string{
		"/api/invites/" + inviteCode,
		"/api/invites/" + inviteCode + "/join",
		"/join/" + inviteCode,
		"/calendar/" + icsToken + ".ics",
		"//calendar/" + icsToken + ".ics",
		"/calendar//" + icsToken + ".ics",
		"/x/../calendar/" + icsToken + ".ics",
		"/join/../calendar/" + icsToken + ".ics",
		"/./join/" + inviteCode,
		"/api/campaigns/0b9d8a64-7d0e-4c43-9d4a-3c2f1d1b7a10/colony",
	} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	logged := output.String()
	if strings.Contains(logged, inviteCode) || strings.Contains(logged, icsToken) {
		t.Fatalf("the request log carries an invite code or an ICS token:\n%s", logged)
	}
	for _, expected := range []string{
		"path=/api/invites/{inviteCode} ",
		"path=/api/invites/{inviteCode}/join ",
		"path=/join/{inviteCode} ",
		"path=/calendar/{icsToken} ",
		"path=/api/campaigns/0b9d8a64-7d0e-4c43-9d4a-3c2f1d1b7a10/colony ",
	} {
		if !strings.Contains(logged, expected) {
			t.Errorf("the request log lacks %q:\n%s", expected, logged)
		}
	}
}
