#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
RELEASE_DIR="${1:-$ROOT/build/releases}"
SOURCE_COMMIT="${LUMONAS_GIT_COMMIT:-$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || printf '%s' unknown)}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || printf '%s' 0)}"
case "$SOURCE_DATE_EPOCH" in
	''|*[!0-9]*) echo "SOURCE_DATE_EPOCH must be a non-negative integer" >&2; exit 1 ;;
esac
mkdir -p "$RELEASE_DIR"
if command -v syft >/dev/null 2>&1; then
	for artifact in "$RELEASE_DIR"/*.deb "$RELEASE_DIR"/*.iso "$RELEASE_DIR"/*.qcow2 "$RELEASE_DIR"/*.raw "$RELEASE_DIR"/lumonas-workstation_*_linux_amd64 "$RELEASE_DIR"/lumonas-workstation_*_linux_arm64 "$RELEASE_DIR"/lumonas-workstation_*_windows_amd64.exe "$RELEASE_DIR"/lumonas-workstation_*_darwin_amd64 "$RELEASE_DIR"/lumonas-workstation_*_darwin_arm64; do
		[ -f "$artifact" ] || continue
		syft "file:$artifact" -o spdx-json > "$artifact.sbom.json"
	done
elif [ "${LUMONAS_REQUIRE_SBOM:-false}" = "true" ]; then
  echo "syft is required for this release" >&2
  exit 1
else
  echo "syft not installed; SBOM generation skipped" >&2
fi

if [ "${LUMONAS_SIGN_ARTIFACTS:-false}" = "true" ]; then
  command -v cosign >/dev/null 2>&1 || { echo "cosign is required when LUMONAS_SIGN_ARTIFACTS=true" >&2; exit 1; }
	for artifact in "$RELEASE_DIR"/*.deb "$RELEASE_DIR"/*.iso "$RELEASE_DIR"/*.qcow2 "$RELEASE_DIR"/*.raw "$RELEASE_DIR"/lumonas-workstation_*_linux_amd64 "$RELEASE_DIR"/lumonas-workstation_*_linux_arm64 "$RELEASE_DIR"/lumonas-workstation_*_windows_amd64.exe "$RELEASE_DIR"/lumonas-workstation_*_darwin_amd64 "$RELEASE_DIR"/lumonas-workstation_*_darwin_arm64; do
    [ -f "$artifact" ] || continue
    cosign sign-blob --yes \
      --output-signature "$artifact.sig" \
      --bundle "$artifact.bundle" \
      "$artifact"
  done
fi

python3 - "$RELEASE_DIR" "$SOURCE_COMMIT" "$SOURCE_DATE_EPOCH" <<'PY'
import hashlib
import json
import pathlib
import sys

release_dir = pathlib.Path(sys.argv[1])
source_commit = sys.argv[2]
source_date_epoch = int(sys.argv[3])
artifacts = []
def sidecar(path):
    if not path.is_file():
        return None
    return {
        "name": path.name,
        "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
        "sizeBytes": path.stat().st_size,
    }

for pattern in ("*.deb", "*.iso", "*.qcow2", "*.raw", "lumonas-workstation_*_linux_amd64", "lumonas-workstation_*_linux_arm64", "lumonas-workstation_*_windows_amd64.exe", "lumonas-workstation_*_darwin_amd64", "lumonas-workstation_*_darwin_arm64"):
    for path in release_dir.glob(pattern):
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        artifacts.append({
            "name": path.name,
            "sha256": digest,
            "sizeBytes": path.stat().st_size,
            "sbom": sidecar(path.with_name(path.name + ".sbom.json")),
            "signature": sidecar(path.with_name(path.name + ".sig")),
            "bundle": sidecar(path.with_name(path.name + ".bundle")),
        })
artifacts.sort(key=lambda item: item["name"])
if not artifacts:
    raise SystemExit("no release artifacts found")
manifest = {
    "schemaVersion": 1,
    "sourceCommit": source_commit,
    "sourceDateEpoch": source_date_epoch,
    "checksums": "SHA256SUMS",
    "artifacts": artifacts,
}
(release_dir / "RELEASE-MANIFEST.json").write_text(
    json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
)
PY

# Generate checksums after the manifest so the release checksum file covers
# every published payload, verification sidecar, and the manifest itself.
(
	cd "$RELEASE_DIR"
	for artifact in *.deb *.iso *.qcow2 *.raw lumonas-workstation_*_linux_amd64 lumonas-workstation_*_linux_arm64 lumonas-workstation_*_windows_amd64.exe lumonas-workstation_*_darwin_amd64 lumonas-workstation_*_darwin_arm64 *.sbom.json *.sig *.bundle RELEASE-MANIFEST.json; do
		[ -f "$artifact" ] || continue
		sha256sum "$artifact"
	done
) > "$RELEASE_DIR/SHA256SUMS"
