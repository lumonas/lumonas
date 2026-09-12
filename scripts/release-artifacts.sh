#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
RELEASE_DIR="${1:-$ROOT/build/releases}"
mkdir -p "$RELEASE_DIR"
find "$RELEASE_DIR" -maxdepth 1 -type f \( -name '*.deb' -o -name '*.iso' -o -name '*.qcow2' -o -name '*.raw' \) -print0 | xargs -0 -r sha256sum > "$RELEASE_DIR/SHA256SUMS"
if command -v syft >/dev/null 2>&1; then
	for artifact in "$RELEASE_DIR"/*.deb "$RELEASE_DIR"/*.iso "$RELEASE_DIR"/*.qcow2 "$RELEASE_DIR"/*.raw; do
		[ -f "$artifact" ] || continue
		syft "file:$artifact" -o spdx-json > "$artifact.sbom.json"
	done
elif [ "${MYNAS_REQUIRE_SBOM:-false}" = "true" ]; then
  echo "syft is required for this release" >&2
  exit 1
else
  echo "syft not installed; SBOM generation skipped" >&2
fi

if [ "${MYNAS_SIGN_ARTIFACTS:-false}" = "true" ]; then
  command -v cosign >/dev/null 2>&1 || { echo "cosign is required when MYNAS_SIGN_ARTIFACTS=true" >&2; exit 1; }
	for artifact in "$RELEASE_DIR"/*.deb "$RELEASE_DIR"/*.iso "$RELEASE_DIR"/*.qcow2 "$RELEASE_DIR"/*.raw; do
    [ -f "$artifact" ] || continue
    cosign sign-blob --yes --output-signature "$artifact.sig" "$artifact"
  done
fi
