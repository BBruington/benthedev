#!/usr/bin/env bash
# Fails when CLAUDE.md names a repository path that doesn't exist, so the agent guide can't drift.
# A path is any `backticked` token with a slash or a file extension, without spaces, placeholders
# (<feature>), URL paths (/api/...) or Go package patterns (./...).
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

missing_paths=0
while IFS= read -r candidate; do
  if [[ ! -e "$candidate" ]]; then
    echo "check-docs: CLAUDE.md names \`$candidate\`, which does not exist" >&2
    missing_paths=1
  fi
done < <(
  grep -o '`[^`]*`' CLAUDE.md \
    | tr -d '`' \
    | grep -E '^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*/?$' \
    | grep -E '/|\.(md|toml|yaml|yml|json|sh|go|sql|ts|tsx|css|html|webmanifest|webp|grit)$' \
    | grep -v '\.\.\.' \
    | sort -u
)

if [[ "$missing_paths" -ne 0 ]]; then
  exit 1
fi
echo "check-docs: every path named in CLAUDE.md exists"
