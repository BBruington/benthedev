package app

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/BBruington/colony-planner/internal/core"
)

// uploadOverTheWire sends an image upload to a real listener, since read deadlines and chunked bodies
// need a connection; body is wrapped so the client can't learn its length and must send it chunked.
func (testHarness *harness) uploadOverTheWire(token string, contentType string, body io.Reader) (int, string) {
	testHarness.t.Helper()
	server := httptest.NewServer(testHarness.handler)
	testHarness.t.Cleanup(server.Close)
	request, err := http.NewRequest(http.MethodPost, server.URL+campaignPathOf(testHarness.campaignA, "/images"), io.NopCloser(body))
	if err != nil {
		testHarness.t.Fatalf("build the upload: %v", err)
	}
	request.Header.Set("Content-Type", contentType)
	request.AddCookie(&http.Cookie{Name: testHarness.auth.SessionCookieName(), Value: token})
	response, err := server.Client().Do(request)
	if err != nil {
		testHarness.t.Fatalf("upload: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	var problem struct {
		Detail string `json:"detail"`
	}
	_ = json.NewDecoder(response.Body).Decode(&problem)
	return response.StatusCode, problem.Detail
}

func TestAChunkedUploadOverTheCapIsTooLarge(t *testing.T) {
	testHarness := newHarness(t)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", "huge.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(testImage(t, 90))
	_, _ = part.Write(make([]byte, core.ImageMaxBytes+128*1024))
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	status, detail := testHarness.uploadOverTheWire(testHarness.signIn(testHarness.dmA), form.FormDataContentType(), &body)
	if status != http.StatusRequestEntityTooLarge || detail != imageTooLargeMessage {
		t.Fatalf("a chunked upload over the cap = %d %q, want 413 %q", status, detail, imageTooLargeMessage)
	}
}

func TestAStalledUploadTimesOut(t *testing.T) {
	testHarness := newHarness(t)
	var head bytes.Buffer
	form := multipart.NewWriter(&head)
	part, err := form.CreateFormFile("image", "slow.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(testImage(t, 91)[:16])
	// The rest of the body never comes; the pipe stays open until the test ends.
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	go func() { _, _ = writer.Write(head.Bytes()) }()
	started := time.Now()
	status, _ := testHarness.uploadOverTheWire(testHarness.signIn(testHarness.dmA), form.FormDataContentType(), reader)
	if status != http.StatusRequestTimeout {
		t.Fatalf("a stalled upload = %d, want 408", status)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("a stalled upload took %v to end", elapsed)
	}
}

// Files over 8 KiB spill to temporary files while the form is read; none may outlive its request,
// whether the upload is stored, refused or the same bytes again.
func TestUploadsLeaveNoTemporaryFiles(t *testing.T) {
	testHarness := newHarness(t)
	temporaryDirectory := t.TempDir()
	t.Setenv("TMPDIR", temporaryDirectory)
	token := testHarness.signIn(testHarness.dmA)
	// Bytes after the header are never decoded, so padding makes a valid image over 8 KiB.
	largeImage := append(testImage(t, 92), make([]byte, 32*1024)...)
	uploads := []struct {
		data   []byte
		status int
	}{
		{data: largeImage, status: http.StatusCreated},
		{data: largeImage, status: http.StatusCreated},
		{data: bytes.Repeat([]byte("not an image "), 4096), status: http.StatusUnprocessableEntity},
	}
	for _, upload := range uploads {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile("image", "large.png")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(upload.data)
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		status, detail := testHarness.uploadOverTheWire(token, form.FormDataContentType(), &body)
		if status != upload.status {
			t.Fatalf("upload of %d bytes = %d %q, want %d", len(upload.data), status, detail, upload.status)
		}
		// The form is removed just after the response is written, so give it a moment.
		var remaining []os.DirEntry
		for attempt := 0; attempt < 50; attempt++ {
			entries, err := os.ReadDir(temporaryDirectory)
			if err != nil {
				t.Fatal(err)
			}
			if remaining = entries; len(remaining) == 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if len(remaining) != 0 {
			t.Fatalf("after an upload answered %d, %d temporary files remain (first %s)", status, len(remaining), remaining[0].Name())
		}
	}
}
