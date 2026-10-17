// Package web embeds the built single-page app, so one binary serves the whole site.
package web

import "embed"

// Dist holds Vite's output in dist/spa after `mise run build`. dist/.gitkeep is tracked so this
// embed compiles before the first web build; the server then serves a short note instead of the app.
//
//go:embed all:dist
var Dist embed.FS
