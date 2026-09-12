#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-release-artifacts.XXXXXX")"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

printf '%s\n' 'debian artifact' >"$WORK/lumonas_test.deb"
printf '%s\n' 'recovery image' >"$WORK/lumonas_test.raw"

LUMONAS_REQUIRE_SBOM=false sh "$ROOT/scripts/release-artifacts.sh" "$WORK"
test -s "$WORK/SHA256SUMS"
(cd "$WORK" && sha256sum -c SHA256SUMS)
sh "$ROOT/scripts/verify-release.sh" "$WORK"

if LUMONAS_REQUIRE_RELEASE_SET=true sh "$ROOT/scripts/verify-release.sh" "$WORK"; then
	echo "incomplete release set was accepted" >&2
	exit 1
fi

printf '%s\n' 'installer image' >"$WORK/lumonas_test.iso"
LUMONAS_REQUIRE_SBOM=false sh "$ROOT/scripts/release-artifacts.sh" "$WORK"
LUMONAS_REQUIRE_RELEASE_SET=true sh "$ROOT/scripts/verify-release.sh" "$WORK"

grep -F 'lumonas_test.deb' "$WORK/SHA256SUMS" >/dev/null
grep -F 'lumonas_test.iso' "$WORK/SHA256SUMS" >/dev/null
grep -F 'lumonas_test.raw' "$WORK/SHA256SUMS" >/dev/null
echo "LumoNAS release artifact checksum smoke test passed"
