#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
ERROR_FILE="$(mktemp "${TMPDIR:-/tmp}/lumonas-installer-workdir.XXXXXX")"
mkdir -p "$ROOT/build"
CACHE_ROOT="$(mktemp -d "$ROOT/build/lumonas-iso-policy.XXXXXX")"
cleanup() {
	rm -f "$ERROR_FILE"
	rm -rf "$CACHE_ROOT"
}
trap cleanup EXIT INT TERM

assert_rejected() {
	expected=$1
	shift
	if "$@" 2>"$ERROR_FILE"; then
		echo "installer accepted an unsafe path configuration: $expected" >&2
		exit 1
	fi
	grep -F "$expected" "$ERROR_FILE" >/dev/null || {
		echo "installer rejected an unsafe path without the expected diagnostic: $expected" >&2
		cat "$ERROR_FILE" >&2
		exit 1
	}
}

assert_rejected 'LUMONAS_ISO_WORKDIR is too broad' \
	env LUMONAS_ISO_WORKDIR=/ sh "$ROOT/installer/build-iso.sh" policy-test
assert_rejected 'LUMONAS_ISO_WORKDIR must be inside build/ or a temporary ISO workdir' \
	env LUMONAS_ISO_WORKDIR=/var/tmp/lumonas-iso-policy sh "$ROOT/installer/build-iso.sh" policy-test
mkdir "$CACHE_ROOT/cache"
assert_rejected 'LUMONAS_ISO_CACHE_SOURCE must not be inside the ISO workdir' \
	env LUMONAS_ISO_WORKDIR="$CACHE_ROOT" LUMONAS_ISO_CACHE_SOURCE="$CACHE_ROOT/cache" sh "$ROOT/installer/build-iso.sh" policy-test

grep -F 'rm -rf "$WORK"' "$ROOT/installer/build-iso.sh" >/dev/null
grep -F 'LUMONAS_ISO_WORKDIR must be an absolute path' "$ROOT/installer/build-iso.sh" >/dev/null
echo "LumoNAS installer workdir safety policy passed"
