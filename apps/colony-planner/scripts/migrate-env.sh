#!/usr/bin/env bash
# M1.5: renames DM_DISCORD_ID to SITE_OWNER_DISCORD_ID in .env, in place and by key name, keeping the
# value. It never prints a value. Run it as `mise run env:upgrade`; pass another file to try it on a
# copy. DISCORD_GUILD_ID stays: delete it yourself after Beny Bot's one-time clean-up
# (docs/discord-dev-setup.md).
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
environment_file="${1:-$repository_root/.env}"

if [[ ! -f "$environment_file" ]]; then
  echo "env:upgrade: $environment_file does not exist; run \`mise run setup\` first." >&2
  exit 1
fi

legacy_lines="$(grep -c -E '^[[:space:]]*(export[[:space:]]+)?DM_DISCORD_ID=' "$environment_file" || true)"
owner_lines="$(grep -c -E '^[[:space:]]*(export[[:space:]]+)?SITE_OWNER_DISCORD_ID=' "$environment_file" || true)"

if [[ "$legacy_lines" == "0" ]]; then
  echo "env:upgrade: no DM_DISCORD_ID in $(basename "$environment_file"); nothing to do."
  exit 0
fi
if [[ "$legacy_lines" != "1" ]]; then
  echo "env:upgrade: DM_DISCORD_ID appears $legacy_lines times; keep one line and run this again." >&2
  exit 1
fi

value_of() {
  # Prints nothing: the value goes only into a variable of the caller.
  grep -E "^[[:space:]]*(export[[:space:]]+)?$1=" "$environment_file" | head -n 1 | sed -E "s/^[[:space:]]*(export[[:space:]]+)?$1=//"
}

temporary_file="$(mktemp "$environment_file.upgrade.XXXXXX")"
trap 'rm -f "$temporary_file"' EXIT
# Keep .env private while it is rewritten.
chmod 600 "$temporary_file"

if [[ "$owner_lines" == "0" ]]; then
  sed -E 's/^([[:space:]]*(export[[:space:]]+)?)DM_DISCORD_ID=/\1SITE_OWNER_DISCORD_ID=/' "$environment_file" > "$temporary_file"
  message="renamed DM_DISCORD_ID to SITE_OWNER_DISCORD_ID (value kept)."
else
  legacy_value="$(value_of DM_DISCORD_ID)"
  owner_value="$(value_of SITE_OWNER_DISCORD_ID)"
  if [[ -n "$owner_value" && "$legacy_value" != "$owner_value" ]]; then
    echo "env:upgrade: DM_DISCORD_ID and SITE_OWNER_DISCORD_ID differ. Keep the Discord ID you used as DM in SITE_OWNER_DISCORD_ID and delete the DM_DISCORD_ID line." >&2
    exit 1
  fi
  if [[ -z "$owner_value" ]]; then
    # An empty SITE_OWNER_DISCORD_ID line takes the old value; the old line goes.
    grep -v -E '^[[:space:]]*(export[[:space:]]+)?SITE_OWNER_DISCORD_ID=' "$environment_file" \
      | sed -E 's/^([[:space:]]*(export[[:space:]]+)?)DM_DISCORD_ID=/\1SITE_OWNER_DISCORD_ID=/' > "$temporary_file"
  else
    grep -v -E '^[[:space:]]*(export[[:space:]]+)?DM_DISCORD_ID=' "$environment_file" > "$temporary_file"
  fi
  message="removed DM_DISCORD_ID; SITE_OWNER_DISCORD_ID keeps the same value."
fi

chmod "$(stat -f '%Lp' "$environment_file" 2>/dev/null || stat -c '%a' "$environment_file")" "$temporary_file"
mv "$temporary_file" "$environment_file"
trap - EXIT
echo "env:upgrade: $message"
if grep -q -E '^[[:space:]]*(export[[:space:]]+)?DISCORD_GUILD_ID=' "$environment_file"; then
  echo "env:upgrade: DISCORD_GUILD_ID is still set. Keep it for one Beny Bot start (it clears the old guild commands and logs \"old guild commands cleared\"), then delete that line."
fi
