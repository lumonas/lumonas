#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
RELEASE_DIR="${1:-$ROOT/build/releases}"
CHECKSUMS="$RELEASE_DIR/SHA256SUMS"

[ -d "$RELEASE_DIR" ] || { echo "release directory not found: $RELEASE_DIR" >&2; exit 1; }
[ -s "$CHECKSUMS" ] || { echo "release checksums are missing: $CHECKSUMS" >&2; exit 1; }

artifact_count=0
for artifact in "$RELEASE_DIR"/*.deb "$RELEASE_DIR"/*.iso "$RELEASE_DIR"/*.qcow2 "$RELEASE_DIR"/*.raw; do
	[ -f "$artifact" ] || continue
	artifact_count=$((artifact_count + 1))
	base=$(basename "$artifact")
	grep -F "  $artifact" "$CHECKSUMS" >/dev/null 2>&1 || grep -F "  $base" "$CHECKSUMS" >/dev/null 2>&1 || {
		echo "artifact is not covered by SHA256SUMS: $base" >&2
		exit 1
	}
	if [ "${LUMONAS_REQUIRE_SBOM:-false}" = "true" ] && [ ! -s "$artifact.sbom.json" ]; then
		echo "SBOM is missing: $artifact.sbom.json" >&2
		exit 1
	fi
	if [ "${LUMONAS_REQUIRE_SIGNATURES:-false}" = "true" ] && [ ! -s "$artifact.sig" ]; then
		echo "signature is missing: $artifact.sig" >&2
		exit 1
	fi
done

[ "$artifact_count" -gt 0 ] || { echo "no release artifacts found in $RELEASE_DIR" >&2; exit 1; }
(cd "$RELEASE_DIR" && sha256sum -c SHA256SUMS >/dev/null)
echo "LumoNAS release artifacts verified: $artifact_count"
