#!/usr/bin/env bash
# Weline seam guard: fails when Weline changes upstream whatomate files outside
# the agreed places, so upstream syncs keep merging cleanly.
#
# Compares HEAD with the upstream commit it is based on (the merge base with
# upstream/main), so being behind upstream is never reported as a change.
#
# Allowed:
#   - anything under internal/modules/, frontend/src/modules/, wl-docs/, and CLAUDE.md
#   - the five seam files, if they only GAINED lines containing "Weline hook"
#   - paths listed in .github/weline/upstream-allowlist.txt, and legacy seam-file
#     lines listed in .github/weline/seam-exceptions.txt
# Everything else that differs from upstream fails.
#
# Usage: .github/scripts/weline-seam-guard.sh [upstream-ref]   (default upstream/main)
set -euo pipefail

UPSTREAM_REF="${1:-${UPSTREAM_REF:-upstream/main}}"
ALLOWLIST=".github/weline/upstream-allowlist.txt"
EXCEPTIONS=".github/weline/seam-exceptions.txt"
SEAM_FILES=(
  cmd/whatomate/main.go
  frontend/src/router/index.ts
  frontend/src/components/layout/AppLayout.vue
  frontend/src/i18n/index.ts
)
MARKER="Weline hook"

if ! git rev-parse --verify --quiet "$UPSTREAM_REF" >/dev/null; then
  echo "::error::$UPSTREAM_REF not found. Add and fetch the upstream remote first:"
  echo "  git remote add upstream https://github.com/shridarpatil/whatomate.git && git fetch upstream main"
  exit 2
fi
BASE="$(git merge-base HEAD "$UPSTREAM_REF")"
echo "Comparing HEAD $(git rev-parse --short HEAD) with upstream base $(git rev-parse --short "$BASE")"

weline_owned() {
  case "$1" in
    internal/modules/* | frontend/src/modules/* | wl-docs/* | CLAUDE.md) return 0 ;;
  esac
  return 1
}

is_seam() {
  local f
  for f in "${SEAM_FILES[@]}"; do [[ "$1" == "$f" ]] && return 0; done
  return 1
}

allowlisted() {
  local entry
  while IFS= read -r entry; do
    entry="${entry%%#*}"
    entry="${entry//[[:space:]]/}"
    [[ -z "$entry" ]] && continue
    if [[ "$entry" == */ ]]; then
      [[ "$1" == "$entry"* ]] && return 0
    elif [[ "$1" == "$entry" ]]; then
      return 0
    fi
  done < "$ALLOWLIST"
  return 1
}

failures=()
seams_checked=0

# Lines a seam file removed, or added without the marker. Whitespace-only added
# lines are fine, and so are the exact lines listed for that file in
# $EXCEPTIONS (legacy edits from before the module SDLC).
check_seam() {
  local file="$1" bad
  bad="$(git diff -U0 "$BASE" HEAD -- "$file" |
    awk -v m="$MARKER" -v f="$file" -v exfile="$EXCEPTIONS" '
      function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
      BEGIN {
        while ((getline l < exfile) > 0) {
          if (l ~ /^[ \t]*(#|$)/) continue
          i = index(l, "|"); if (i == 0) continue
          if (substr(l, 1, i - 1) == f) ok[trim(substr(l, i + 1))] = 1
        }
      }
      /^(\+\+\+|---) / { next }
      /^[-+]/ { line = substr($0, 2); t = trim(line); if (t in ok) next }
      /^-/ { print "  removed: " line; next }
      /^\+/ { if (t != "" && index(line, m) == 0) print "  added without \"" m "\": " line }
    ')"
  if [[ -n "$bad" ]]; then
    failures+=("$file (seam file: may only gain lines marked \"$MARKER\")"$'\n'"$bad")
  fi
  seams_checked=$((seams_checked + 1))
}

while IFS=$'\t' read -r status path; do
  [[ -z "$path" ]] && continue
  if weline_owned "$path"; then
    continue
  elif is_seam "$path"; then
    check_seam "$path"
  elif allowlisted "$path"; then
    continue
  else
    case "$status" in
      A) what="new file outside the module folders" ;;
      D) what="deletes an upstream file" ;;
      *) what="modifies an upstream file" ;;
    esac
    failures+=("$path ($what)")
  fi
done < <(git diff --name-status --no-renames "$BASE" HEAD)

if ((${#failures[@]})); then
  echo
  echo "::error::Seam guard failed: ${#failures[@]} file(s) change upstream whatomate outside the agreed places."
  printf '%s\n' "${failures[@]}" | sed 's/^/  /'
  cat <<'EOF'

Move module code into internal/modules/<module>/ or frontend/src/modules/<module>/.
Upstream files may only gain one-line seams marked "// Weline hook" in the five seam files.
If a change really must live in an upstream file, add it to .github/weline/upstream-allowlist.txt
with the reason, and call it out in the PR.
EOF
  exit 1
fi

echo "Seam guard passed: upstream files untouched except $seams_checked seam file(s) with marked lines and allowlisted paths."
