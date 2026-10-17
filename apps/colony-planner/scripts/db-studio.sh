#!/usr/bin/env bash
# Starts the DbGate database browser (compose profile "studio") next to the local Postgres and opens
# it in the browser. It is already connected to the `colony` database. Stop it with `mise run db:down`.
set -euo pipefail

studio_url="http://localhost:54330"

if ! docker info >/dev/null 2>&1; then
  echo "db:studio: Docker is not running. Open Docker Desktop, wait until it says it is running, then try again." >&2
  exit 1
fi

if [[ -z "${POSTGRES_PASSWORD:-}" ]]; then
  echo "db:studio: POSTGRES_PASSWORD is not set. Run \`mise run setup\` to create .env." >&2
  exit 1
fi

docker compose --profile studio up --detach --wait --quiet-pull studio >/dev/null
echo "db:studio: DbGate is ready at ${studio_url} (connection \"Colony Planner (local)\")."
echo "db:studio: edits made there skip the app's rules (colony version, undo, timeline); \`mise run fresh\` resets the sample data."

if command -v open >/dev/null 2>&1; then
  open "$studio_url"
fi
