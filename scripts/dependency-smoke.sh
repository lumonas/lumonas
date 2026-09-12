#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
REQUIRE_TOOLS="${LUMONAS_REQUIRE_DEPENDENCY_TOOLS:-false}"

if command -v govulncheck >/dev/null 2>&1; then
	cd "$ROOT"
	govulncheck ./...
elif [ "$REQUIRE_TOOLS" = "true" ]; then
	echo "govulncheck is required for the dependency gate" >&2
	exit 1
else
	echo "govulncheck is not installed; Go vulnerability scan skipped" >&2
fi

if [ "${LUMONAS_SKIP_NPM_AUDIT:-false}" = "true" ]; then
	echo "pnpm audit explicitly skipped"
elif command -v pnpm >/dev/null 2>&1; then
	cd "$ROOT/web"
	pnpm audit --prod --audit-level high
else
	if [ "$REQUIRE_TOOLS" = "true" ]; then
		echo "pnpm is required for the frontend dependency gate" >&2
		exit 1
	fi
	echo "pnpm is not installed; frontend dependency scan skipped" >&2
fi

echo "LumoNAS dependency checks passed"
