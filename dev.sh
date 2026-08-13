#!/usr/bin/env bash
# Starts the Hairpin server and the Expo client together for local testing.
#
#   ./dev.sh            server + Expo (pick a target from the Expo menu)
#   ./dev.sh web        server + opens the game in your browser
#   ./dev.sh server     server only
#   ./dev.sh smoke      server only, then run the loop smoke test against it
#
# Job timers are compressed 60x by default so a 4-hour job finishes in 4
# minutes and a 30-second one is instant. Set HAIRPIN_TIME_SCALE=1 for real time.
set -euo pipefail

cd "$(dirname "$0")"

MODE="${1:-}"
PORT="${HAIRPIN_PORT:-8080}"
export HAIRPIN_ADDR=":${PORT}"
export HAIRPIN_TIME_SCALE="${HAIRPIN_TIME_SCALE:-60}"
export HAIRPIN_TOKEN_SECRET="${HAIRPIN_TOKEN_SECRET:-local-dev-secret-change-me}"
export HAIRPIN_SEED_BOTS="${HAIRPIN_SEED_BOTS:-120}"

SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    echo
    echo "stopping server (pid $SERVER_PID)"
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

start_server() {
  echo "==> starting server on :${PORT} (time scale ${HAIRPIN_TIME_SCALE}x)"
  (cd server && go run ./cmd/hairpin) &
  SERVER_PID=$!

  for _ in $(seq 1 60); do
    if curl -fsS "http://localhost:${PORT}/health" >/dev/null 2>&1; then
      echo "==> server up: http://localhost:${PORT}"
      return 0
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      echo "server exited during startup" >&2
      exit 1
    fi
    sleep 1
  done
  echo "server did not become healthy in 60s" >&2
  exit 1
}

# The phone cannot reach "localhost" — that would be the phone itself. Guess the
# machine's LAN address so Expo Go on a real device works without extra setup.
lan_ip() {
  if command -v ipconfig >/dev/null 2>&1; then
    ipconfig getifaddr en0 2>/dev/null && return 0   # macOS
  fi
  if command -v hostname >/dev/null 2>&1; then
    hostname -I 2>/dev/null | awk '{print $1}' && return 0
  fi
  echo ""
}

case "$MODE" in
  server)
    start_server
    wait "$SERVER_PID"
    ;;

  smoke)
    start_server
    echo
    HAIRPIN_URL="http://localhost:${PORT}" python3 scripts/smoke.py
    ;;

  ""|web|expo)
    start_server

    if [[ ! -d client/node_modules ]]; then
      echo "==> installing client dependencies (first run only)"
      (cd client && npm install)
    fi

    IP="$(lan_ip)"
    if [[ -n "$IP" ]]; then
      export EXPO_PUBLIC_API_URL="http://${IP}:${PORT}"
    else
      export EXPO_PUBLIC_API_URL="http://localhost:${PORT}"
      echo "!! could not detect a LAN IP; a physical phone will not reach the server."
      echo "!! set EXPO_PUBLIC_API_URL=http://<your-ip>:${PORT} and rerun."
    fi
    echo "==> client will call ${EXPO_PUBLIC_API_URL}"
    echo

    if [[ "$MODE" == "web" ]]; then
      (cd client && npx expo start --web)
    else
      (cd client && npx expo start)
    fi
    ;;

  *)
    echo "usage: $0 [web|server|smoke]" >&2
    exit 2
    ;;
esac
