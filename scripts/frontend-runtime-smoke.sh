#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
DIST="$ROOT/web/dist"
ASSETS="$DIST/assets"

[ -f "$DIST/index.html" ] || { echo "frontend dist/index.html is missing" >&2; exit 1; }
[ -d "$ASSETS" ] || { echo "frontend dist/assets is missing" >&2; exit 1; }
grep -F '<title>LumoNAS</title>' "$DIST/index.html" >/dev/null 2>&1 || {
	echo "production frontend title is missing" >&2
	exit 1
}
grep -F '<div id="root"></div>' "$DIST/index.html" >/dev/null 2>&1 || {
	echo "production frontend React root is missing" >&2
	exit 1
}

# Vite statically removes the mock bootstrap when VITE_USE_MOCKS is not true.
# The mock service-worker asset may remain available for local demos, but the
# production JavaScript must never start MSW or contain its request protocol.
if grep -R -n -E 'setupWorker|MOCK_ACTIVATE|msw/passthrough|VITE_USE_MOCKS' "$ASSETS" >/dev/null 2>&1; then
	echo "production frontend bundle contains the MSW bootstrap" >&2
	exit 1
fi

echo "LumoNAS production frontend runtime passed (real API mode, no MSW bootstrap)"
