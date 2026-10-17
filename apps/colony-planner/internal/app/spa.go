package app

import (
	"io/fs"
	"mime"
	"net/http"
	"strings"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/web"
)

func init() {
	// Go's MIME table lacks the PWA manifest; browsers expect this type for it.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// Unknown paths under these prefixes are API mistakes, not client-side routes, so they get a 404
// instead of the SPA shell.
var serverPrefixes = []string{"api/", "auth/", "img/", "calendar/"}

const missingBuildPage = `<!doctype html>
<html lang="en">
  <head><meta charset="utf-8"><title>Colony Planner</title></head>
  <body>
    <p>This server has no built web app embedded.</p>
    <p>During development, open <a href="http://localhost:5173">http://localhost:5173</a> (<code>mise run dev</code>).
    To embed the app, run <code>mise run build</code> and start <code>bin/colony serve</code>.</p>
  </body>
</html>`

// spaHandler serves the embedded build: real files as they are (hashed assets cached forever), and
// index.html for every other path so client-side routes survive a reload. Without a build (plain
// `go build` during development) it serves a short note pointing at the Vite dev server.
func spaHandler() http.Handler {
	distribution, indexPage, built := embeddedBuild()
	var fileServer http.Handler
	if built {
		fileServer = http.FileServerFS(distribution)
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedPath := strings.TrimPrefix(request.URL.Path, "/")
		for _, prefix := range serverPrefixes {
			if strings.HasPrefix(requestedPath, prefix) {
				writeMissingRoute(writer)
				return
			}
		}
		if !built {
			writeHTML(writer, "no-store", []byte(missingBuildPage))
			return
		}
		if requestedPath != "" && requestedPath != "index.html" {
			if information, statError := fs.Stat(distribution, requestedPath); statError == nil && !information.IsDir() {
				if strings.HasPrefix(requestedPath, "assets/") {
					writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					writer.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(writer, request)
				return
			}
		}
		writeHTML(writer, "no-cache", indexPage)
	})
}

// writeMissingRoute is the answer to an unknown path under a server prefix.
func writeMissingRoute(writer http.ResponseWriter) {
	auth.WriteProblem(writer, http.StatusNotFound, "No such route.")
}

func embeddedBuild() (fs.FS, []byte, bool) {
	distribution, err := fs.Sub(web.Dist, "dist/spa")
	if err != nil {
		return nil, nil, false
	}
	indexPage, err := fs.ReadFile(distribution, "index.html")
	if err != nil {
		return nil, nil, false
	}
	return distribution, indexPage, true
}

func writeHTML(writer http.ResponseWriter, cacheControl string, page []byte) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", cacheControl)
	_, _ = writer.Write(page)
}
