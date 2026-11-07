#!/usr/bin/env bash
# One-time setup when the Codespace / dev container is created.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f config.toml ]; then
  cp config.example.toml config.toml
  echo "Created config.toml from config.example.toml (db/redis hosts already match)."
fi
mkdir -p uploads

echo "Downloading Go modules..."
go mod download

echo "Installing frontend dependencies..."
(cd frontend && npm ci --no-audit --no-fund)

echo "Setup complete. Run 'make dev' to start Weline."
