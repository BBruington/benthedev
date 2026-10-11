#!/usr/bin/env bash
# govulncheck over the module: fails when code we call reaches a known vulnerability, so new
# advisories for Go or a dependency show up in `mise run check` and CI without a manual run.
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

# Pinned like every other tool: the newest release at least two weeks old.
govulncheck_version="v1.8.0"

# Advisories already triaged, each with the change that clears it. Remove the IDs once it lands.
# - Go 1.27.1 standard library (HTTP/2 server, HTTP/1 CONNECT, Range parsing, textproto, crypto/tls ECH),
#   all fixed in Go 1.27.2 (released 2026-10-08). Under the pinning rule it may be adopted on or after
#   2026-10-22: bump `go` in mise.toml and the go line in go.mod, then delete these IDs. The server
#   speaks plain HTTP/1 on 127.0.0.1 until M6, so none is reachable before then.
accepted_advisories=(
  GO-2026-6603 GO-2026-6605 GO-2026-6607 GO-2026-6608 GO-2026-6609
  GO-2026-6610 GO-2026-6611 GO-2026-6612 GO-2026-6613 GO-2026-6617
)

if ! command -v jq >/dev/null 2>&1; then
  echo "vulncheck: jq is required (preinstalled on macOS 15+ and GitHub's Ubuntu runners)" >&2
  exit 1
fi

report="$(mktemp)"
trap 'rm -f "$report"' EXIT
# JSON output exits 0 whatever it finds; the reachable findings are the ones whose trace names a function.
go run "golang.org/x/vuln/cmd/govulncheck@${govulncheck_version}" -format json ./... >"$report"
reachable="$(jq -r 'select(.finding != null and .finding.trace[0].function != null) | .finding.osv' "$report" | sort -u)"

unexpected=()
for advisory in $reachable; do
  accepted=0
  for known in "${accepted_advisories[@]}"; do
    if [[ "$advisory" == "$known" ]]; then
      accepted=1
      break
    fi
  done
  if [[ "$accepted" -eq 0 ]]; then
    unexpected+=("$advisory")
  fi
done

if [[ "${#unexpected[@]}" -gt 0 ]]; then
  echo "vulncheck: code reaches these vulnerabilities: ${unexpected[*]}" >&2
  echo "vulncheck: see the details with: go run golang.org/x/vuln/cmd/govulncheck@${govulncheck_version} ./..." >&2
  exit 1
fi
echo "vulncheck: no unexpected reachable vulnerabilities ($(echo "$reachable" | grep -c . || true) triaged in scripts/vulncheck.sh)"
