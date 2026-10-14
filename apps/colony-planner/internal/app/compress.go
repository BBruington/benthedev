package app

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// A gzip writer holds about a megabyte of compression state, so requests share them.
var gzipWriters = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// compressResponses gzips JSON answers and the SPA's text assets for clients that accept gzip. It
// decides from the Content-Type when the headers go out, so the event stream (which must reach the
// browser one event at a time) and images (already compressed) always pass through untouched.
func compressResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		compressing := &compressingWriter{ResponseWriter: writer, acceptsGzip: acceptsGzip(request.Header.Get("Accept-Encoding"))}
		defer compressing.finish()
		next.ServeHTTP(compressing, request)
	})
}

type compressingWriter struct {
	http.ResponseWriter
	acceptsGzip bool
	wroteHeader bool
	// gzip is nil unless this response is being compressed.
	gzip *gzip.Writer
}

func (writer *compressingWriter) WriteHeader(status int) {
	if writer.wroteHeader || status < http.StatusOK {
		writer.ResponseWriter.WriteHeader(status)
		return
	}
	writer.wroteHeader = true
	header := writer.Header()
	// No body (204, 304), or a byte range of the uncompressed file (206): nothing to compress.
	hasWholeBody := status != http.StatusNoContent && status != http.StatusNotModified && status != http.StatusPartialContent
	if hasWholeBody && header.Get("Content-Encoding") == "" && compressibleType(header.Get("Content-Type")) {
		header.Add("Vary", "Accept-Encoding")
		if writer.acceptsGzip {
			header.Set("Content-Encoding", "gzip")
			header.Del("Content-Length")
			writer.gzip = gzipWriters.Get().(*gzip.Writer)
			writer.gzip.Reset(writer.ResponseWriter)
		}
	}
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *compressingWriter) Write(data []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.gzip != nil {
		return writer.gzip.Write(data)
	}
	return writer.ResponseWriter.Write(data)
}

// FlushError is what http.ResponseController calls: compressed bytes still in the gzip writer go out first.
func (writer *compressingWriter) FlushError() error {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.gzip != nil {
		if err := writer.gzip.Flush(); err != nil {
			return err
		}
	}
	return http.NewResponseController(writer.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the connection's deadlines (the stream and uploads set them).
func (writer *compressingWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func (writer *compressingWriter) finish() {
	if writer.gzip == nil {
		return
	}
	// A failed close means the client went away; there is no one left to tell.
	_ = writer.gzip.Close()
	gzipWriters.Put(writer.gzip)
	writer.gzip = nil
}

func compressibleType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mediaType {
	case "application/json", "text/html", "text/css", "text/javascript", "image/svg+xml":
		return true
	}
	// application/problem+json, application/openapi+json and the PWA's application/manifest+json.
	return strings.HasSuffix(mediaType, "+json")
}

// acceptsGzip reads an Accept-Encoding header: gzip is acceptable when listed without q=0.
func acceptsGzip(acceptEncoding string) bool {
	for coding := range strings.SplitSeq(acceptEncoding, ",") {
		name, parameters, _ := strings.Cut(coding, ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		quality, hasQuality := strings.CutPrefix(strings.TrimSpace(parameters), "q=")
		if !hasQuality {
			return true
		}
		value, err := strconv.ParseFloat(quality, 64)
		return err == nil && value > 0
	}
	return false
}
