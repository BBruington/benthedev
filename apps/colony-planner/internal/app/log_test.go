package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An invite code lets anyone ask to join, and only the campaign's DM may read it, so the request log
// (the operator's view) names the route, not the code (Invariant 14).
func TestRequestLogKeepsInviteCodesOut(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	handler := logRequests(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}), logger)
	const inviteCode = "-zZOCQTWaiKX0GF2o3SnTg"
	for _, path := range []string{
		"/api/invites/" + inviteCode,
		"/api/invites/" + inviteCode + "/join",
		"/join/" + inviteCode,
		"/api/campaigns/0b9d8a64-7d0e-4c43-9d4a-3c2f1d1b7a10/colony",
	} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	logged := output.String()
	if strings.Contains(logged, inviteCode) {
		t.Fatalf("the request log carries an invite code:\n%s", logged)
	}
	for _, expected := range []string{
		"path=/api/invites/{inviteCode} ",
		"path=/api/invites/{inviteCode}/join ",
		"path=/join/{inviteCode} ",
		"path=/api/campaigns/0b9d8a64-7d0e-4c43-9d4a-3c2f1d1b7a10/colony ",
	} {
		if !strings.Contains(logged, expected) {
			t.Errorf("the request log lacks %q:\n%s", expected, logged)
		}
	}
}
