package app

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testRequestReadTimeout = 300 * time.Millisecond

// startAppServer serves the app through HTTPServer, so the test sees the production read limits.
func (testHarness *harness) startAppServer() *httptest.Server {
	testHarness.t.Helper()
	server := httptest.NewUnstartedServer(nil)
	server.Config = testHarness.server.HTTPServer(nil)
	server.Start()
	testHarness.t.Cleanup(server.Close)
	return server
}

// A body that stalls is cut off at the read timeout, even when the request is refused without reading
// it (net/http drains an unread body before it answers), while an event stream outlives the timeout.
func TestStalledBodiesEndAtTheReadTimeoutAndStreamsOutliveIt(t *testing.T) {
	testHarness := newHarness(t)
	server := testHarness.startAppServer()

	// A raw connection, so the test controls when the body stops and can always hang up.
	connection, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	_, err = fmt.Fprintf(connection, "POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n8\r\n{\"days\":",
		campaignPathOf(testHarness.campaignA, "/colony/advance"), server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_ = connection.SetReadDeadline(started.Add(5 * time.Second))
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatalf("a signed-out request whose body stalls got no answer: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a signed-out request whose body stalls = %d, want 401", response.StatusCode)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("a stalled body held its connection for %v", elapsed)
	}

	stream := testHarness.openStreamOn(server, testHarness.campaignA, testHarness.signIn(testHarness.ana), "")
	if stream.status != http.StatusOK {
		t.Fatalf("stream = %d, want 200", stream.status)
	}
	until := time.Now().Add(4 * testRequestReadTimeout)
	for time.Now().Before(until) {
		select {
		case _, open := <-stream.events:
			if !open {
				t.Fatal("the stream ended at the read timeout")
			}
		case <-time.After(time.Second):
			t.Fatal("the stream stopped sending heartbeats")
		}
	}
}
