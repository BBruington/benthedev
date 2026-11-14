// Package foundry embeds the built Colony Bridge module, so Colony Planner serves its manifest and zip
// and the module's version always matches the server's (docs/design/foundry-bridge.md §12.4).
package foundry

import "embed"

// Dist holds the module build's module.json and colony-bridge.zip. dist/.gitkeep is tracked so this
// embed compiles before the first build; the server then answers those routes with a short 404.
//
//go:embed all:dist
var Dist embed.FS
