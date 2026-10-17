#!/usr/bin/env bash
# govulncheck over the module: fails when code we call reaches a known vulnerability, so new
# advisories for Go or a dependency show up in `mise run check` and CI without a manual run. Nothing is
# accepted: Go 1.27.2 fixed the ten standard-library advisories that were triaged under Go 1.27.1.
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

# Pinned like every other tool: the newest release at least two weeks old.
govulncheck_version="v1.8.0"

# Text output exits non-zero only for a vulnerability our code reaches; one in a package or module
# we merely import is listed in the summary and passes.
go run "golang.org/x/vuln/cmd/govulncheck@${govulncheck_version}" ./...
