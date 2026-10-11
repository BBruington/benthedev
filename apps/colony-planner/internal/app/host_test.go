package app

import (
	"net/http"
	"testing"

	"github.com/BBruington/colony-planner/internal/auth"
)

// A page on a domain its owner re-points at 127.0.0.1 (DNS rebinding) reaches the local server as a
// same-origin page, so outside production only the hosts this server is known by get an answer.
func TestUnknownHostsAreRefused(t *testing.T) {
	testHarness := newHarness(t)
	dmToken := testHarness.signIn(testHarness.dm.ID)

	for _, host := range []string{"rebind.attacker.example:8080", "rebind.attacker.example", "localhost.attacker.example:5173"} {
		for _, path := range []string{"/healthz", "/api/me", "/api/members", "/auth/dev?as=dm", "/login"} {
			response := testHarness.do(http.MethodGet, path, requestOptions{token: dmToken, host: host, headers: map[string]string{"Sec-Fetch-Site": "same-origin"}})
			if response.Code != http.StatusMisdirectedRequest {
				t.Errorf("GET %s with Host %q = %d, want 421", path, host, response.Code)
			}
			if cookies := response.Result().Cookies(); len(cookies) > 0 {
				t.Errorf("GET %s with Host %q set cookies: %v", path, host, cookies)
			}
		}
		settings := testHarness.do(http.MethodPut, "/api/settings", requestOptions{
			token: dmToken, host: host,
			headers: map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://" + host},
			body:    `{"campaignName":"Owned","colonyName":"Port Eldari","timezone":"UTC","discordAnnounceChannelId":null,"discordReminderChannelId":null,"discordDmChannelId":null}`,
		})
		testHarness.expectStatus(settings, http.StatusMisdirectedRequest)
	}

	// localhost in any case, IP literals (dev:phone's Wi-Fi address through Vite) and PUBLIC_URL's host.
	for _, host := range []string{"localhost:5173", "LOCALHOST:5173", "localhost", "127.0.0.1:8080", "[::1]:8080", "192.168.1.23:5173"} {
		testHarness.expectStatus(testHarness.do(http.MethodGet, "/api/members", requestOptions{token: dmToken, host: host}), http.StatusOK)
	}
	if auth.DevAuthEnabled {
		signIn := testHarness.do(http.MethodGet, "/auth/dev?as=player", requestOptions{host: "192.168.1.23:5173"})
		testHarness.expectStatus(signIn, http.StatusSeeOther)
	}
}
