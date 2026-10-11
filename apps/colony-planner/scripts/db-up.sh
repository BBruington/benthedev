#!/usr/bin/env bash
# Starts the compose Postgres and waits until it accepts connections.
# CI provides its own Postgres service, so this is a no-op there.
set -euo pipefail

if [[ -n "${CI:-}" ]]; then
  exit 0
fi

if ! docker info >/dev/null 2>&1; then
  echo "db:up: Docker is not running. Open Docker Desktop, wait until it says it is running, then try again." >&2
  exit 1
fi

if [[ -z "${POSTGRES_PASSWORD:-}" ]]; then
  echo "db:up: POSTGRES_PASSWORD is not set. Run \`mise run setup\` to create .env." >&2
  exit 1
fi

docker compose up --detach --wait --quiet-pull postgres >/dev/null
echo "db:up: Postgres is ready on 127.0.0.1:54329 (container colony-planner-db)."
