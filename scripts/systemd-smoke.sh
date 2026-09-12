#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
REQUIRE="${LUMONAS_REQUIRE_SYSTEMD_VERIFY:-false}"

if ! command -v systemd-analyze >/dev/null 2>&1; then
	if [ "$REQUIRE" = "true" ]; then
		echo "systemd-analyze is required for the systemd unit gate" >&2
		exit 1
	fi
	echo "systemd-analyze is not installed; unit verification skipped" >&2
	exit 0
fi

systemd-analyze verify "$ROOT"/packaging/systemd/*.service
echo "LumoNAS systemd units verified"
