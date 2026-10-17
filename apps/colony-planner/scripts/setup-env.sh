#!/usr/bin/env bash
# Creates .env from .env.example with a random local database password.
# An existing .env is left alone, because the database volume keeps the password it was created with.
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
environment_file="$repository_root/.env"
example_file="$repository_root/.env.example"

if [[ -f "$environment_file" ]]; then
  echo "setup: .env already exists; leaving it unchanged."
  exit 0
fi

database_password="$(openssl rand -hex 24)"
sed "s/__POSTGRES_PASSWORD__/$database_password/g" "$example_file" > "$environment_file"
chmod 600 "$environment_file"
echo "setup: created .env with a random local database password."
