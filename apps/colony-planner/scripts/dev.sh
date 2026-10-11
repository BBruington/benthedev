#!/usr/bin/env bash
# Runs the Go server (devauth build, 127.0.0.1:8080) and the Vite dev server (:5173) together,
# with labelled output. Stops both when either exits or on Ctrl-C.
# Usage: scripts/dev.sh [local|phone]
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

mode="${1:-local}"
vite_arguments=()
if [[ "$mode" == "phone" ]]; then
  vite_arguments+=(--host 0.0.0.0)
fi

label_output() {
  local label="$1"
  local color="$2"
  local line
  while IFS= read -r line; do
    printf '\033[%sm%s |\033[0m %s\n' "$color" "$label" "$line"
  done
}

# A second copy would fail to bind, and the first copy's answers would look like this one's.
for port in "${PORT:-8080}" 5173; do
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
    echo "dev: something is already listening on port $port (another \`mise run dev\`?). Stop it, then try again." >&2
    exit 1
  fi
done

mkdir -p tmp
echo "dev: building the Go server with the devauth tag..."
go build -tags devauth -o tmp/colony-dev ./cmd/colony

server_process=""
vite_process=""
stop_processes() {
  trap - EXIT INT TERM
  for process_id in $server_process $vite_process; do
    kill "$process_id" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
# Ctrl-C or a TERM is a normal way to stop, so it exits cleanly; any other exit still stops both.
trap stop_processes EXIT
trap 'echo "dev: stopping."; stop_processes; exit 0' INT TERM

tmp/colony-dev serve > >(label_output "api" "36") 2>&1 &
server_process=$!

(cd web && exec bun --bun node_modules/vite/bin/vite.js "${vite_arguments[@]+"${vite_arguments[@]}"}") > >(label_output "web" "35") 2>&1 &
vite_process=$!

# Announce the address only once Go answers, so a port clash or a database problem is the last word.
health_url="http://127.0.0.1:${PORT:-8080}/healthz"
until curl --silent --fail --max-time 1 --output /dev/null "$health_url"; do
  if ! kill -0 "$server_process" 2>/dev/null; then
    echo "dev: the Go server stopped before it was ready; its last lines above say why." >&2
    exit 1
  fi
  sleep 0.2
done

echo "dev: open http://localhost:5173 (Go API on http://127.0.0.1:8080, Postgres on 127.0.0.1:54329)"
if [[ "$mode" == "phone" ]]; then
  wifi_address="$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)"
  if [[ -n "$wifi_address" ]]; then
    echo "dev:phone: on a phone on the same Wi-Fi, open http://$wifi_address:5173 (dev sign-in is open to this network; use it at home)"
  else
    echo "dev:phone: could not find this Mac's Wi-Fi address; check System Settings > Wi-Fi > Details."
  fi
fi

while kill -0 "$server_process" 2>/dev/null && kill -0 "$vite_process" 2>/dev/null; do
  sleep 1
done

if ! kill -0 "$server_process" 2>/dev/null; then
  echo "dev: the Go server stopped; stopping Vite too." >&2
else
  echo "dev: Vite stopped; stopping the Go server too." >&2
fi
exit 1
