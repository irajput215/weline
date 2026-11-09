#!/usr/bin/env bash
# Weline frontend checks: lint, type check and unit tests.
#
# Upstream whatomate's own frontend currently has lint and type errors we must
# not fix in upstream files. So lint runs on Weline's files only, and the type
# check runs over the whole app but fails only on errors in Weline's files;
# upstream errors are printed for information. Unit tests run in full.
#
# Run from the repo root, after `npm ci` in frontend/.
set -euo pipefail
cd frontend

# Weline-owned frontend paths (module folders and seam files).
WELINE_PATHS=(
  src/modules
  src/router/index.ts
  src/components/layout/AppLayout.vue
  src/i18n/index.ts
)
# Same list as a regex for vue-tsc output lines ("path(line,col): error ...").
WELINE_RE='^(src/modules/|src/router/index\.ts|src/components/layout/AppLayout\.vue|src/i18n/index\.ts)'

existing=()
for p in "${WELINE_PATHS[@]}"; do [[ -e "$p" ]] && existing+=("$p"); done

echo "::group::eslint (Weline files)"
npx eslint --ext .vue,.js,.ts "${existing[@]}"
echo "::endgroup::"

echo "::group::vue-tsc"
set +e
npx vue-tsc --noEmit > /tmp/vue-tsc.log 2>&1
tsc_status=$?
set -e
ours="$(grep -E "$WELINE_RE" /tmp/vue-tsc.log || true)"
theirs="$(grep -E '^[^ ].*error TS' /tmp/vue-tsc.log | grep -vE "$WELINE_RE" || true)"
if [[ -n "$theirs" ]]; then
  echo "Upstream type errors (not Weline's, not failing the build):"
  echo "$theirs" | sed 's/^/  /'
fi
echo "::endgroup::"
if [[ -n "$ours" ]]; then
  echo "::error::Type errors in Weline files:"
  echo "$ours"
  exit 1
fi
if [[ $tsc_status -ne 0 && -z "$theirs" ]]; then
  # vue-tsc failed without any recognisable error line: show everything.
  cat /tmp/vue-tsc.log
  exit 1
fi

echo "::group::vitest"
npm run test:unit
echo "::endgroup::"
