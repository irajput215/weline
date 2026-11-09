#!/usr/bin/env bash
# Weline migration smoke test: builds the server, starts it with -migrate on an
# EMPTY database, checks the wl schema and every registered module's
# migrations and permissions are in place, stops it, then starts it again with
# -migrate to prove re-running is a no-op. This is what every deploy does.
#
# Needs Postgres and Redis. Database settings come from WHATOMATE_* env vars
# (see internal/config), defaulting to a local postgres/postgres setup.
# The database named by WHATOMATE_DATABASE__NAME must exist and be empty.
set -euo pipefail

export WHATOMATE_DATABASE__HOST="${WHATOMATE_DATABASE__HOST:-localhost}"
export WHATOMATE_DATABASE__PORT="${WHATOMATE_DATABASE__PORT:-5432}"
export WHATOMATE_DATABASE__USER="${WHATOMATE_DATABASE__USER:-postgres}"
export WHATOMATE_DATABASE__PASSWORD="${WHATOMATE_DATABASE__PASSWORD:-postgres}"
export WHATOMATE_DATABASE__NAME="${WHATOMATE_DATABASE__NAME:-weline_smoke}"
export WHATOMATE_DATABASE__SSL_MODE=disable
export WHATOMATE_REDIS__HOST="${WHATOMATE_REDIS__HOST:-localhost}"
export WHATOMATE_SERVER__PORT="${WHATOMATE_SERVER__PORT:-18081}"
export PGPASSWORD="$WHATOMATE_DATABASE__PASSWORD"

BIN="${RUNNER_TEMP:-/tmp}/whatomate-smoke"
LOG="${RUNNER_TEMP:-/tmp}/whatomate-smoke.log"
URL="http://localhost:${WHATOMATE_SERVER__PORT}/health"

psql_q() {
  psql -h "$WHATOMATE_DATABASE__HOST" -p "$WHATOMATE_DATABASE__PORT" -U "$WHATOMATE_DATABASE__USER" \
    -d "$WHATOMATE_DATABASE__NAME" -tAc "$1"
}

echo "Building server..."
go build -o "$BIN" ./cmd/whatomate

start_and_wait() {
  "$BIN" server -config config.example.toml -migrate > "$LOG" 2>&1 &
  PID=$!
  for _ in $(seq 1 120); do
    if curl -sf --noproxy '*' -o /dev/null "$URL"; then return 0; fi
    if ! kill -0 "$PID" 2>/dev/null; then
      echo "::error::Server exited during startup:"; cat "$LOG"; exit 1
    fi
    sleep 1
  done
  echo "::error::Server did not become healthy in 120s:"; cat "$LOG"; kill "$PID" || true; exit 1
}

stop() { kill "$PID"; wait "$PID" 2>/dev/null || true; }

echo "Run 1: -migrate on an empty database"
[[ "$(psql_q "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'")" == "0" ]] ||
  { echo "::error::database $WHATOMATE_DATABASE__NAME is not empty"; exit 1; }
start_and_wait
grep -q "Weline modules migrated" "$LOG" || { echo "::error::module migration did not run"; cat "$LOG"; exit 1; }
stop

migrations="$(psql_q "SELECT count(*) FROM wl.schema_migrations")"
perms="$(psql_q "SELECT count(*) FROM permissions WHERE resource LIKE 'wl\_%'")"
echo "wl.schema_migrations rows: $migrations, wl_ permissions: $perms"
psql_q "SELECT id FROM wl.schema_migrations ORDER BY id" | sed 's/^/  /'
[[ "$migrations" -gt 0 && "$perms" -gt 0 ]] || { echo "::error::expected module migrations and permissions"; exit 1; }

echo "Run 2: -migrate again must be a no-op"
start_and_wait
stop
[[ "$(psql_q "SELECT count(*) FROM wl.schema_migrations")" == "$migrations" ]] ||
  { echo "::error::second -migrate changed wl.schema_migrations"; exit 1; }
[[ "$(psql_q "SELECT count(*) FROM permissions WHERE resource LIKE 'wl\_%'")" == "$perms" ]] ||
  { echo "::error::second -migrate duplicated permissions"; exit 1; }

echo "Migration smoke test passed."
