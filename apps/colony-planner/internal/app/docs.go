package app

import (
	"net/http"
	"strings"
)

func serveOpenAPI(document []byte) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/openapi+json")
		writer.Header().Set("Cache-Control", "no-store")
		_, _ = writer.Write(document)
	})
}

// The docs page is huma's Stoplight Elements page, pinned with subresource integrity, pointed at the
// dm-only /api/openapi.json.
var docsContentSecurityPolicy = strings.Join([]string{
	"default-src 'none'",
	"base-uri 'none'",
	"connect-src 'self'",
	"form-action 'none'",
	"frame-ancestors 'none'",
	"sandbox allow-same-origin allow-scripts allow-popups allow-popups-to-escape-sandbox allow-downloads",
	"script-src https://unpkg.com/@stoplight/elements@9.0.15/web-components.min.js",
	"style-src 'unsafe-inline' https://unpkg.com/@stoplight/elements@9.0.15/styles.min.css",
}, "; ")

const docsPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="referrer" content="no-referrer">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Colony Planner API</title>
    <link rel="stylesheet" href="https://unpkg.com/@stoplight/elements@9.0.15/styles.min.css" crossorigin integrity="sha384-iVQBHadsD+eV0M5+ubRCEVXrXEBj+BqcuwjUwPoVJc0Pb1fmrhYSAhL+BFProHdV">
    <script src="https://unpkg.com/@stoplight/elements@9.0.15/web-components.min.js" crossorigin integrity="sha384-xjOcq9PZ/k+pGtPS/xcsCRXGjKKfTlIa4H1IYEnC+97jNa6sAMWTNrV6hY08W3GL"></script>
  </head>
  <body style="height: 100vh;">
    <elements-api apiDescriptionUrl="/api/openapi.json" router="hash" layout="sidebar" tryItCredentialsPolicy="same-origin"></elements-api>
  </body>
</html>`

func serveDocs(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Security-Policy", docsContentSecurityPolicy)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = writer.Write([]byte(docsPage))
}
