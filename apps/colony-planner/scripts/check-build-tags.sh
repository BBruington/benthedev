#!/usr/bin/env bash
# Fails on any Go build constraint other than devauth / !devauth. check.sh lints and tests exactly
# those two builds, so a file behind any other tag would skip every boundary rule and test unseen.
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

unexpected_constraints="$(grep -rn --include='*.go' -E '^//go:build ' cmd internal db web \
  | grep -v -E ':(//go:build devauth|//go:build !devauth)$' || true)"
if [[ -n "$unexpected_constraints" ]]; then
  echo "check-build-tags: only the devauth tag is allowed (check.sh builds with and without it):" >&2
  echo "$unexpected_constraints" >&2
  exit 1
fi
echo "check-build-tags: only devauth build constraints are used"
