package app

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BBruington/colony-planner/internal/core"
)

func gunzip(t *testing.T, compressed []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("not a gzip body: %v", err)
	}
	plain, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	return plain
}

func TestCompressionFollowsTheContentType(t *testing.T) {
	body := strings.Repeat(`{"q":1,"r":2,"terrain":"FOREST","label":""},`, 100)
	for _, testCase := range []struct {
		name, contentType, acceptEncoding string
		status                            int
		compressed                        bool
	}{
		{"JSON", "application/json", "gzip, deflate, br, zstd", http.StatusOK, true},
		{"a problem", "application/problem+json", "gzip", http.StatusNotFound, true},
		{"the SPA's page", "text/html; charset=utf-8", "gzip", http.StatusOK, true},
		{"a script", "text/javascript; charset=utf-8", "gzip;q=0.5", http.StatusOK, true},
		{"a stylesheet", "text/css; charset=utf-8", "GZIP", http.StatusOK, true},
		{"the manifest", "application/manifest+json", "gzip", http.StatusOK, true},
		{"gzip refused", "application/json", "gzip;q=0, br", http.StatusOK, false},
		{"no Accept-Encoding", "application/json", "", http.StatusOK, false},
		{"an image", "image/webp", "gzip", http.StatusOK, false},
		{"a font", "font/woff2", "gzip", http.StatusOK, false},
		{"the event stream", "text/event-stream", "gzip", http.StatusOK, false},
		{"a calendar", "text/calendar; charset=utf-8", "gzip", http.StatusOK, false},
	} {
		handler := compressResponses(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", testCase.contentType)
			writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
			writer.WriteHeader(testCase.status)
			_, _ = io.WriteString(writer, body)
		}))
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Accept-Encoding", testCase.acceptEncoding)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != testCase.status {
			t.Errorf("%s: status = %d, want %d", testCase.name, recorder.Code, testCase.status)
		}
		compressed := recorder.Header().Get("Content-Encoding") == "gzip"
		if compressed != testCase.compressed {
			t.Errorf("%s: compressed = %v, want %v", testCase.name, compressed, testCase.compressed)
			continue
		}
		received := recorder.Body.Bytes()
		if compressed {
			if recorder.Header().Get("Content-Length") != "" {
				t.Errorf("%s: a compressed answer kept the uncompressed Content-Length", testCase.name)
			}
			received = gunzip(t, received)
		}
		if string(received) != body {
			t.Errorf("%s: the body changed on the way", testCase.name)
		}
	}
}

// A cache between the browser and the server must not hand a gzip answer to a client that can't read it.
func TestCompressibleAnswersVaryOnAcceptEncoding(t *testing.T) {
	handler := compressResponses(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", request.URL.Query().Get("type"))
		_, _ = io.WriteString(writer, "{}")
	}))
	for contentType, varies := range map[string]bool{"application/json": true, "image/png": false} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?type="+contentType, nil))
		if got := recorder.Header().Get("Vary") == "Accept-Encoding"; got != varies {
			t.Errorf("%s: Vary = %q", contentType, recorder.Header().Get("Vary"))
		}
	}
}

func TestBodylessAndPartialAnswersAreNotCompressed(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified, http.StatusPartialContent} {
		handler := compressResponses(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(status)
			if status == http.StatusPartialContent {
				_, _ = io.WriteString(writer, `{"q"`)
			}
		}))
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Accept-Encoding", "gzip")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Header().Get("Content-Encoding") != "" {
			t.Errorf("a %d answer was compressed", status)
		}
	}
}

// The real event stream, asked for by a browser that accepts gzip: it stays plain text and each
// message reaches the client while the stream is still open.
func TestTheEventStreamIsNeverCompressed(t *testing.T) {
	testHarness := newHarness(t)
	server := httptest.NewServer(testHarness.handler)
	t.Cleanup(server.Close)
	streamContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(streamContext, http.MethodGet, server.URL+campaignPathOf(testHarness.campaignA, "/stream"), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = testHost
	request.Header.Set("Accept-Encoding", "gzip, deflate, br")
	request.AddCookie(&http.Cookie{Name: testHarness.auth.SessionCookieName(), Value: testHarness.signIn(testHarness.ana)})
	// The explicit Accept-Encoding stops the client from undoing a compressed body behind the test's back.
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("open the stream: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
		t.Fatalf("the stream was sent with Content-Encoding %q", encoding)
	}

	reader := bufio.NewReader(response.Body)
	opening, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(opening, "retry: ") {
		t.Fatalf("first line = %q, %v; want the plain-text retry line", opening, err)
	}
	// The harness sends a heartbeat every 50 ms; reading two proves each one was flushed on its own.
	heartbeats := 0
	for heartbeats < 2 {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("the stream stopped after %d heartbeats: %v", heartbeats, err)
		}
		if strings.HasPrefix(line, ":") {
			heartbeats++
		}
	}
}

// The Explore map's read at 5,000 hexes, the size M4 measured, goes out compressed to a browser that
// accepts gzip and plain to one that doesn't.
func TestTheHexesReadIsCompressed(t *testing.T) {
	testHarness := newHarness(t)
	viewer := testHarness.enter(testHarness.dmA, testHarness.campaignA.id)
	terrains := core.AllTerrainValues()
	const hexCount = 5000
	for start := 0; start < hexCount; start += core.HexCellsPerCall {
		stroke := make([]core.PaintedHex, 0, core.HexCellsPerCall)
		for index := start; index < start+core.HexCellsPerCall; index++ {
			q, r := int64(index%100-50), int64(index/100-25)
			stroke = append(stroke, core.PaintedHex{Q: q, R: r, Terrain: terrains[(index*7+index/100*3)%len(terrains)]})
		}
		if err := testHarness.core.PaintHexes(testHarness.context, viewer, core.PaintHexesInput{Cells: stroke}); err != nil {
			t.Fatalf("paint: %v", err)
		}
	}
	token := testHarness.signIn(testHarness.ana)
	hexesPath := campaignPathOf(testHarness.campaignA, "/hexes")

	plain := testHarness.do(http.MethodGet, hexesPath, requestOptions{token: token})
	testHarness.expectStatus(plain, http.StatusOK)
	if plain.Header().Get("Content-Encoding") != "" {
		t.Fatal("a client that didn't ask for gzip got it")
	}
	compressed := testHarness.do(http.MethodGet, hexesPath, requestOptions{token: token, headers: map[string]string{"Accept-Encoding": "gzip, deflate, br"}})
	testHarness.expectStatus(compressed, http.StatusOK)
	if compressed.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("the hexes read wasn't compressed for a client that accepts gzip")
	}
	if !bytes.Equal(gunzip(t, compressed.Body.Bytes()), plain.Body.Bytes()) {
		t.Fatal("the compressed hexes read differs from the plain one")
	}
	if !bytes.Contains(plain.Body.Bytes(), []byte(`"q":49,"r":24`)) {
		t.Fatal("the hexes read is missing the last painted hex")
	}
	plainSize, compressedSize := plain.Body.Len(), compressed.Body.Len()
	t.Logf("hexes read at %d hexes: %d bytes plain, %d bytes gzipped (%.1f%%)", hexCount, plainSize, compressedSize, 100*float64(compressedSize)/float64(plainSize))
	if compressedSize*5 > plainSize {
		t.Fatalf("gzip shrank %d bytes only to %d", plainSize, compressedSize)
	}
}
