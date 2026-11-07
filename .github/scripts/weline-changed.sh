#!/usr/bin/env bash
# Weline: decide whether a CI job needs to run, based on which files changed.
#
# Usage: weline-changed.sh <regex>            run if ANY changed file matches
#        weline-changed.sh <regex> --except   run if ANY changed file does NOT match
#
# Writes run=true|false to $GITHUB_OUTPUT. When the base commit can't be
# determined (new branch, force push, manual run) it always says run=true, so
# the safe default is "test everything".
set -euo pipefail

pattern="$1"
mode="${2:-}"

if [ "${GITHUB_EVENT_NAME:-}" = "pull_request" ]; then
  base="${BASE_SHA:-}"
else
  base="${BEFORE_SHA:-}"
fi

if [ -z "$base" ] || [ "$base" = "0000000000000000000000000000000000000000" ] \
  || ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
  echo "No usable base commit; running everything."
  echo "run=true" >> "$GITHUB_OUTPUT"
  exit 0
fi

files="$(git diff --name-only "$base" HEAD)"
echo "Changed files since ${base}:"
echo "${files:-  (none)}"

if [ "$mode" = "--except" ]; then
  hits="$(printf '%s\n' "$files" | grep -Ev "$pattern" | grep -v '^$' || true)"
else
  hits="$(printf '%s\n' "$files" | grep -E "$pattern" || true)"
fi

if [ -n "$hits" ]; then
  echo "Relevant changes found; running."
  echo "run=true" >> "$GITHUB_OUTPUT"
else
  echo "No relevant changes; skipping."
  echo "run=false" >> "$GITHUB_OUTPUT"
fi
