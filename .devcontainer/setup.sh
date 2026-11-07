#!/usr/bin/env bash
# Dev container setup. Runs twice:
#   - updateContentCommand: during a Codespaces prebuild (or first create), does the slow work
#   - postCreateCommand:    when a Codespace is created, catches up if go.mod or
#                           package-lock.json changed since the prebuild; otherwise a quick no-op
set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f config.toml ]; then
  cp config.example.toml config.toml
  echo "Created config.toml from config.example.toml (db/redis hosts already match)."
fi
mkdir -p uploads

# Go modules: fast when already cached.
echo "Downloading Go modules..."
go mod download

# Frontend packages: reinstall only when package-lock.json changed.
stamp=frontend/node_modules/.weline-lock-hash
want="$(sha256sum frontend/package-lock.json | cut -d' ' -f1)"
if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$want" ]; then
  echo "Frontend dependencies up to date."
else
  echo "Installing frontend dependencies..."
  (cd frontend && npm ci --no-audit --no-fund)
  echo "$want" > "$stamp"
fi

echo "Setup complete. Run 'make dev' to start Weline."
