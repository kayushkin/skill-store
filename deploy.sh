#!/usr/bin/env bash
set -euo pipefail

# One shared gate decides whether this tree may be deployed (main clone, default
# branch, clean, pushed, not behind, and the same for every tree the build reads).
# It lives in healthcheck/scripts/deploy-gate.sh. Do not inline or copy it.
( cd "$(dirname "$0")" && "$HOME/bin/deploy-gate" check )

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$HOME/bin"
SERVICE="skill-store.service"
BINARY="skill-store"

cd "$REPO_DIR"

export PATH="$HOME/.local/share/mise/shims:$PATH"
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=${XDG_RUNTIME_DIR}/bus}"

echo "==> Building $BINARY..."
go build -o "$BINARY" ./cmd/skill-store
echo "    built: $(ls -lh "$BINARY" | awk '{print $5}')"

# A set SKILL_STORE_ADDR…, SKILL_STORE_DATA_DIR… or SKILL_STORE_INSTALL_DIR…
# variable that settings.go does not declare stops the new binary at boot. Ask
# before the old one is stopped: build the registry from the running service's
# own environment. The test prints a verdict, never a value.
echo "==> Checking the running service's environment against the declared settings..."
live_pid="$(systemctl --user show -p MainPID --value "$SERVICE")"
if [ -n "$live_pid" ] && [ "$live_pid" != "0" ]; then
  go test -count=1 -run '^TestTheLiveProcessEnvironmentBuildsARegistry$' . -args -live-environment-file="/proc/$live_pid/environ"
else
  echo "    $SERVICE is not running, so there is no environment to check"
fi

echo "==> Stopping $SERVICE..."
systemctl --user stop "$SERVICE" 2>/dev/null || true
sleep 1

echo "==> Installing binary to $BIN_DIR..."
mkdir -p "$BIN_DIR"
cp "$BINARY" "$BIN_DIR/$BINARY"

echo "==> Starting $SERVICE..."
systemctl --user daemon-reload
systemctl --user start "$SERVICE"

echo "==> Verifying..."
# `is-active` only proves the process exists. Poll the HTTP surface: a binary
# that boots and then fails to answer is the failure this check has to catch.
PORT="$(systemctl --user show -p Environment --value "$SERVICE" \
  | tr ' ' '\n' | sed -n 's/^SKILL_STORE_ADDR=.*://p')"
PORT="${PORT:-8301}"
READY=""
for _ in $(seq 1 30); do
  if ! systemctl --user is-active --quiet "$SERVICE"; then
    echo "ERROR: $SERVICE failed to start"
    journalctl --user -u "$SERVICE" -n 20 --no-pager 2>&1
    exit 1
  fi
  if [ "$(curl -fsS --max-time 2 "http://127.0.0.1:$PORT/health" 2>/dev/null)" = '{"status":"ok"}' ]; then
    READY=1
    break
  fi
  sleep 0.5
done
if [ -z "$READY" ]; then
  echo "ERROR: $SERVICE is running but did not answer /health on :$PORT within 15s"
  journalctl --user -u "$SERVICE" -n 20 --no-pager 2>&1
  exit 1
fi
# The registry must actually serve its rows, not just a static /health.
SKILL_COUNT="$(curl -fsS --max-time 5 "http://127.0.0.1:$PORT/skills" | jq -r 'length')"
echo "    $SERVICE answers on :$PORT ($SKILL_COUNT skills registered)"
journalctl --user -u "$SERVICE" -n 5 --no-pager 2>&1 | grep -v '^--' || true

echo "==> Done."

# Last act: write this deploy to repo-store's ledger, so the next agent sees what is live.
( cd "$(dirname "$0")" && "$HOME/bin/deploy-gate" record )
