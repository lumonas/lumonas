#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-release-artifacts.XXXXXX")"
cleanup() {
	rm -rf "$WORK"
	if [ -n "${outside:-}" ]; then
		rm -f "$outside"
	fi
}
trap cleanup EXIT INT TERM

printf '%s\n' 'debian artifact' >"$WORK/lumonas_test.deb"
printf '%s\n' 'recovery image' >"$WORK/lumonas_test.raw"
printf '%s\n' 'linux x64 client' >"$WORK/lumonas-workstation_ci_linux_amd64"
printf '%s\n' 'linux arm client' >"$WORK/lumonas-workstation_ci_linux_arm64"
printf '%s\n' 'windows client' >"$WORK/lumonas-workstation_ci_windows_amd64.exe"
printf '%s\n' 'mac x64 client' >"$WORK/lumonas-workstation_ci_darwin_amd64"
printf '%s\n' 'mac arm client' >"$WORK/lumonas-workstation_ci_darwin_arm64"

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
grep -F 'lumonas-workstation_ci_linux_amd64' "$WORK/SHA256SUMS" >/dev/null
grep -F 'RELEASE-MANIFEST.json' "$WORK/SHA256SUMS" >/dev/null
grep -F '"sourceDateEpoch"' "$WORK/RELEASE-MANIFEST.json" >/dev/null
grep -F '"lumonas_test.iso"' "$WORK/RELEASE-MANIFEST.json" >/dev/null
grep -F '"lumonas-workstation_ci_linux_amd64"' "$WORK/RELEASE-MANIFEST.json" >/dev/null
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
cp "$WORK/RELEASE-MANIFEST.json" "$WORK/RELEASE-MANIFEST.json.before-path-test"
cp "$WORK/SHA256SUMS" "$WORK/SHA256SUMS.before-path-test"
outside="$WORK/../lumonas-release-artifact-outside.deb"
printf '%s\n' 'outside release directory' >"$outside"
python3 - "$WORK/RELEASE-MANIFEST.json" "$outside" <<'PY'
import hashlib
import json
import pathlib
import sys

manifest_path = pathlib.Path(sys.argv[1])
outside = pathlib.Path(sys.argv[2])
manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
artifact = manifest["artifacts"][0]
artifact["name"] = "../" + outside.name
artifact["sha256"] = hashlib.sha256(outside.read_bytes()).hexdigest()
artifact["sizeBytes"] = outside.stat().st_size
manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
manifest_sha=$(sha256sum "$WORK/RELEASE-MANIFEST.json" | awk '{print $1}')
sed -i.bak "s/^[0-9a-f][0-9a-f]*  RELEASE-MANIFEST.json$/$manifest_sha  RELEASE-MANIFEST.json/" "$WORK/SHA256SUMS"
rm -f "$WORK/SHA256SUMS.bak"
if path_output=$(sh "$ROOT/scripts/verify-release.sh" "$WORK" 2>&1); then
	echo "release manifest accepted an artifact outside the release directory" >&2
	exit 1
fi
printf '%s\n' "$path_output" | grep -F 'release manifest artifact name is not a direct file name' >/dev/null
mv "$WORK/RELEASE-MANIFEST.json.before-path-test" "$WORK/RELEASE-MANIFEST.json"
mv "$WORK/SHA256SUMS.before-path-test" "$WORK/SHA256SUMS"
rm -f "$outside"
cp "$WORK/RELEASE-MANIFEST.json" "$WORK/RELEASE-MANIFEST.json.backup"
printf '%s\n' 'tampered' >>"$WORK/RELEASE-MANIFEST.json"
if manifest_output=$(sh "$ROOT/scripts/verify-release.sh" "$WORK" 2>&1); then
	echo "tampered release manifest was accepted" >&2
	exit 1
fi
printf '%s\n' "$manifest_output" | grep -F 'release manifest is not valid JSON' >/dev/null
mv "$WORK/RELEASE-MANIFEST.json.backup" "$WORK/RELEASE-MANIFEST.json"
echo "LumoNAS release artifact checksum smoke test passed"
