#!/usr/bin/env bash
# Weline module journeys: builds the production binary (frontend embedded),
# starts it with -migrate, and runs every module's Playwright journeys
# (frontend/src/modules/*/e2e/*.e2e.ts) against it.
#
# Needs Postgres, Redis, Node with `npm ci` done in frontend/, and a Chromium
# for Playwright (CI runs `npx playwright install --with-deps chromium`; set
# PW_CHROMIUM_PATH to use another one). Database settings come from WHATOMATE_*
# env vars, defaulting to a local postgres/postgres setup. The database must
# exist; it may be empty.
set -euo pipefail

export WHATOMATE_DATABASE__HOST="${WHATOMATE_DATABASE__HOST:-localhost}"
export WHATOMATE_DATABASE__PORT="${WHATOMATE_DATABASE__PORT:-5432}"
export WHATOMATE_DATABASE__USER="${WHATOMATE_DATABASE__USER:-postgres}"
export WHATOMATE_DATABASE__PASSWORD="${WHATOMATE_DATABASE__PASSWORD:-postgres}"
export WHATOMATE_DATABASE__NAME="${WHATOMATE_DATABASE__NAME:-weline_e2e}"
export WHATOMATE_DATABASE__SSL_MODE=disable
export WHATOMATE_REDIS__HOST="${WHATOMATE_REDIS__HOST:-localhost}"
export WHATOMATE_SERVER__PORT="${WHATOMATE_SERVER__PORT:-18082}"

LOG="${RUNNER_TEMP:-/tmp}/whatomate-e2e.log"
export BASE_URL="http://localhost:${WHATOMATE_SERVER__PORT}"

echo "::group::Build (frontend embedded)"
make build-prod
echo "::endgroup::"

./whatomate server -config config.example.toml -migrate > "$LOG" 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true' EXIT
for _ in $(seq 1 120); do
  if curl -sf --noproxy '*' -o /dev/null "$BASE_URL/health"; then break; fi
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "::error::Server exited during startup:"; cat "$LOG"; exit 1
  fi
  sleep 1
done
curl -sf --noproxy '*' -o /dev/null "$BASE_URL/health" ||
  { echo "::error::Server did not become healthy in 120s:"; cat "$LOG"; exit 1; }

cd frontend
if ! npx playwright test -c src/modules/playwright.config.ts; then
  echo "::group::Server log"; cat "$LOG"; echo "::endgroup::"
  exit 1
fi
