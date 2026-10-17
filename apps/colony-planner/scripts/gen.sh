#!/usr/bin/env bash
# Regenerates everything derived from the migrations and the Go API structs (spec §8.4):
# sqlc → internal/store, the OpenAPI document, and the TypeScript API types.
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

echo "gen: sqlc generate"
sqlc generate

echo "gen: web/src/api/openapi.json"
openapi_document="$(mktemp)"
trap 'rm -f "$openapi_document"' EXIT
go run ./cmd/colony openapi > "$openapi_document"
cat "$openapi_document" > web/src/api/openapi.json

echo "gen: web/src/api/schema.d.ts"
(cd web && bun run --silent gen:api)
