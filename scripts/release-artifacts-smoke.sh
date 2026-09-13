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
test -s "$WORK/RELEASE-MANIFEST.json"
(cd "$WORK" && sha256sum -c SHA256SUMS)
sh "$ROOT/scripts/verify-release.sh" "$WORK"

if LUMONAS_REQUIRE_RELEASE_SET=true sh "$ROOT/scripts/verify-release.sh" "$WORK"; then
	echo "incomplete release set was accepted" >&2
	exit 1
fi

printf '%s\n' 'installer image' >"$WORK/lumonas_test.iso"
LUMONAS_REQUIRE_SBOM=false sh "$ROOT/scripts/release-artifacts.sh" "$WORK"
LUMONAS_REQUIRE_RELEASE_SET=true sh "$ROOT/scripts/verify-release.sh" "$WORK"

printf '%s\n' '{"spdxVersion":"SPDX-2.3","creationInfo":{},"packages":[]}' >"$WORK/lumonas_test.deb.sbom.json"
LUMONAS_REQUIRE_SBOM=false sh "$ROOT/scripts/release-artifacts.sh" "$WORK"
sh "$ROOT/scripts/verify-release.sh" "$WORK"
printf '%s\n' tampered >>"$WORK/lumonas_test.deb.sbom.json"
if sh "$ROOT/scripts/verify-release.sh" "$WORK"; then
	echo "tampered SBOM sidecar was accepted" >&2
	exit 1
fi

printf '%s\n' '{"spdxVersion":"SPDX-2.3"}' >"$WORK/lumonas_test.deb.sbom.json"
LUMONAS_REQUIRE_SBOM=false sh "$ROOT/scripts/release-artifacts.sh" "$WORK"
if sh "$ROOT/scripts/verify-release.sh" "$WORK"; then
	echo "malformed SBOM sidecar was accepted" >&2
	exit 1
fi

grep -F 'lumonas_test.deb' "$WORK/SHA256SUMS" >/dev/null
grep -F 'lumonas_test.iso' "$WORK/SHA256SUMS" >/dev/null
grep -F 'lumonas_test.raw' "$WORK/SHA256SUMS" >/dev/null
grep -F 'RELEASE-MANIFEST.json' "$WORK/SHA256SUMS" >/dev/null
grep -F '"sourceDateEpoch"' "$WORK/RELEASE-MANIFEST.json" >/dev/null
grep -F '"lumonas_test.iso"' "$WORK/RELEASE-MANIFEST.json" >/dev/null
if LUMONAS_REQUIRE_RELEASE_SET=true LUMONAS_EXPECTED_SOURCE_COMMIT=wrong sh "$ROOT/scripts/verify-release.sh" "$WORK"; then
	echo "release manifest accepted an unexpected source commit" >&2
	exit 1
fi
if LUMONAS_REQUIRE_RELEASE_SET=true LUMONAS_EXPECTED_SOURCE_DATE_EPOCH=0 sh "$ROOT/scripts/verify-release.sh" "$WORK"; then
	echo "release manifest accepted an unexpected source timestamp" >&2
	exit 1
fi
printf '%s\n' 'unlisted artifact' >"$WORK/unlisted.raw"
if LUMONAS_REQUIRE_RELEASE_SET=true sh "$ROOT/scripts/verify-release.sh" "$WORK"; then
	echo "release manifest accepted an unlisted artifact" >&2
	exit 1
fi
rm -f "$WORK/unlisted.raw"
cp "$WORK/RELEASE-MANIFEST.json" "$WORK/RELEASE-MANIFEST.json.backup"
printf '%s\n' 'tampered' >>"$WORK/RELEASE-MANIFEST.json"
if sh "$ROOT/scripts/verify-release.sh" "$WORK"; then
	echo "tampered release manifest was accepted" >&2
	exit 1
fi
mv "$WORK/RELEASE-MANIFEST.json.backup" "$WORK/RELEASE-MANIFEST.json"
echo "LumoNAS release artifact checksum smoke test passed"
