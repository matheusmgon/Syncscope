#!/usr/bin/env bash
# Start two fake Argo CD instances and `wails dev` against an isolated config.
#
#   scripts/dev.sh                 # prod: 15000 apps on :8099, staging: 4000 apps on :8098
#   PROD_APPS=2000 STAGING_APPS=500 scripts/dev.sh
#   KEEP_CONFIG=1 scripts/dev.sh   # reuse ~/.cache/syncscope-dev instead of a fresh temp dir
#
# Add the instances in the app as http://localhost:8099 and http://localhost:8098
# (local login admin / admin, or the fake SSO).
set -euo pipefail

cd "$(dirname "$0")/.."

PROD_PORT=${PROD_PORT:-8099}
STAGING_PORT=${STAGING_PORT:-8098}
PROD_APPS=${PROD_APPS:-15000}
STAGING_APPS=${STAGING_APPS:-4000}

WAILS=${WAILS:-$(command -v wails || echo "$(go env GOPATH)/bin/wails")}
if [[ ! -x "$WAILS" ]]; then
  echo "wails CLI not found; install it with: go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
  exit 1
fi

TMP=${TMPDIR:-/tmp}
TMP=${TMP%/}
if [[ -n "${KEEP_CONFIG:-}" ]]; then
  CONFIG_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/syncscope-dev"
  mkdir -p "$CONFIG_DIR"
else
  CONFIG_DIR=$(mktemp -d "$TMP/syncscope-dev.XXXXXX")
fi

BIN_DIR=$(mktemp -d "$TMP/mockargo.XXXXXX")
pids=()
cleanup() {
  for p in "${pids[@]:-}"; do [[ -n "$p" ]] && kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  rm -rf "$BIN_DIR"
  [[ -z "${KEEP_CONFIG:-}" ]] && rm -rf "$CONFIG_DIR"
  return 0
}
trap cleanup EXIT INT TERM

wait_port() {
  local port=$1
  for _ in $(seq 1 60); do
    if curl -fsS "http://localhost:$port/api/version" >/dev/null 2>&1; then return 0; fi
    sleep 0.5
  done
  echo "mockargo on :$port did not come up" >&2
  return 1
}

echo "building mockargo..."
go build -o "$BIN_DIR/mockargo" ./cmd/mockargo

"$BIN_DIR/mockargo" -port "$PROD_PORT" -apps "$PROD_APPS" -name prod >"$BIN_DIR/prod.log" 2>&1 &
pids+=($!)
"$BIN_DIR/mockargo" -port "$STAGING_PORT" -apps "$STAGING_APPS" -name staging >"$BIN_DIR/staging.log" 2>&1 &
pids+=($!)
wait_port "$PROD_PORT"
wait_port "$STAGING_PORT"

echo "mock Argo CD: prod     http://localhost:$PROD_PORT ($PROD_APPS apps, log $BIN_DIR/prod.log)"
echo "mock Argo CD: staging  http://localhost:$STAGING_PORT ($STAGING_APPS apps, log $BIN_DIR/staging.log)"
echo "config dir:   $CONFIG_DIR (keyring disabled)"

SYNCSCOPE_CONFIG_DIR="$CONFIG_DIR" SYNCSCOPE_NO_KEYRING=1 "$WAILS" dev "$@"
