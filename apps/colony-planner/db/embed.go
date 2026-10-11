// Package db embeds the goose migrations, the one schema source, into the binary and applies them.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
