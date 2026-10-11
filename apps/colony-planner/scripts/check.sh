#!/usr/bin/env bash
# Every check, in order (spec §8.12). Prints "All checks passed" or names the first failure.
set -uo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

run_step() {
  local description="$1"
  shift
  echo ""
  echo "==> $description"
  if ! "$@"; then
    echo "" >&2
    echo "FAILED: $description" >&2
    exit 1
  fi
}

in_web() {
  (cd web && "$@")
}

run_step "Install web dependencies (frozen lockfile)" in_web bun install --frozen-lockfile
run_step "Generate code (sqlc, OpenAPI, TypeScript types)" scripts/gen.sh
run_step "Only the devauth build tag" scripts/check-build-tags.sh
run_step "go vet" go vet ./...
run_step "go vet (devauth build)" go vet -tags devauth ./...
run_step "golangci-lint (devauth build)" golangci-lint run --build-tags=devauth ./...
run_step "golangci-lint (production build)" golangci-lint run ./...
run_step "Start the test database" scripts/db-up.sh
run_step "go test (against colony_test)" go test -tags devauth ./...
run_step "go test (production build, no devauth)" go test ./...
run_step "Web typecheck" in_web bun run typecheck
run_step "Biome" in_web bun x biome ci .
run_step "Web unit tests" in_web bun run test
run_step "Web build" in_web bun run build --logLevel warn
run_step "CLAUDE.md paths exist" scripts/check-docs.sh

echo ""
echo "All checks passed"
