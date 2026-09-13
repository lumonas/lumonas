#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
ERROR_FILE="$(mktemp "${TMPDIR:-/tmp}/lumonas-installer-signature.XXXXXX")"
cleanup() { rm -f "$ERROR_FILE"; }
trap cleanup EXIT INT TERM

if LUMONAS_REQUIRE_REPO_SIGNATURE=true LUMONAS_REPO_SIGN_KEY= \
	sh "$ROOT/installer/build-iso.sh" policy-test 2>"$ERROR_FILE"; then
	echo "installer accepted a required repository signature without a key" >&2
	exit 1
fi
grep -F 'LUMONAS_REPO_SIGN_KEY is required' "$ERROR_FILE" >/dev/null
grep -F 'REPO_SIGNATURE_REQUIRED' "$ROOT/installer/build-iso.sh" >/dev/null
grep -F 'signed LumoNAS repository metadata could not be verified' "$ROOT/installer/build-iso.sh" >/dev/null
grep -F 'signed LumoNAS repository package installation failed' "$ROOT/installer/build-iso.sh" >/dev/null
echo "LumoNAS installer signature policy passed"
