#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
RELEASE_DIR="${1:-$ROOT/build/releases}"
CHECKSUMS="$RELEASE_DIR/SHA256SUMS"
MANIFEST="$RELEASE_DIR/RELEASE-MANIFEST.json"

[ -d "$RELEASE_DIR" ] || { echo "release directory not found: $RELEASE_DIR" >&2; exit 1; }
[ -s "$CHECKSUMS" ] || { echo "release checksums are missing: $CHECKSUMS" >&2; exit 1; }
if [ "${LUMONAS_REQUIRE_RELEASE_SET:-false}" = "true" ] && [ ! -s "$MANIFEST" ]; then
	echo "release manifest is missing: $MANIFEST" >&2
	exit 1
fi

if [ -s "$MANIFEST" ]; then
	python3 - "$RELEASE_DIR" "$MANIFEST" "${LUMONAS_EXPECTED_SOURCE_COMMIT:-}" <<'PY'
import hashlib
import json
import pathlib
import sys

release_dir = pathlib.Path(sys.argv[1])
manifest_path = pathlib.Path(sys.argv[2])
expected_source_commit = sys.argv[3]
manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
if manifest.get("schemaVersion") != 1:
    raise SystemExit("unsupported release manifest schema")
if not manifest.get("sourceCommit"):
    raise SystemExit("release manifest sourceCommit is empty")
if expected_source_commit and manifest["sourceCommit"] != expected_source_commit:
    raise SystemExit("release manifest sourceCommit does not match the expected release commit")
epoch = manifest.get("sourceDateEpoch")
if not isinstance(epoch, int) or epoch < 0:
    raise SystemExit("release manifest sourceDateEpoch must be a non-negative integer")
if manifest.get("checksums") != "SHA256SUMS":
    raise SystemExit("release manifest checksum filename mismatch")

expected = {}
for item in manifest.get("artifacts", []):
    name = item.get("name")
    if not isinstance(name, str) or name in expected:
        raise SystemExit("release manifest contains an invalid or duplicate artifact")
    path = release_dir / name
    if not path.is_file() or path.suffix not in {".deb", ".iso", ".qcow2", ".raw"}:
        raise SystemExit(f"release manifest artifact is missing or invalid: {name}")
    if item.get("sizeBytes") != path.stat().st_size:
        raise SystemExit(f"release manifest size mismatch: {name}")
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    if item.get("sha256") != digest:
        raise SystemExit(f"release manifest checksum mismatch: {name}")
    expected[name] = item

actual = sorted(path.name for pattern in ("*.deb", "*.iso", "*.qcow2", "*.raw") for path in release_dir.glob(pattern))
if sorted(expected) != actual:
    raise SystemExit("release manifest artifact set does not match release directory")
PY
fi

artifact_count=0
deb_count=0
iso_count=0
machine_image_count=0
if [ "${LUMONAS_REQUIRE_SIGNATURES:-false}" = "true" ]; then
	command -v cosign >/dev/null 2>&1 || { echo "cosign is required for signature verification" >&2; exit 1; }
fi
for artifact in "$RELEASE_DIR"/*.deb "$RELEASE_DIR"/*.iso "$RELEASE_DIR"/*.qcow2 "$RELEASE_DIR"/*.raw; do
	[ -f "$artifact" ] || continue
	artifact_count=$((artifact_count + 1))
	base=$(basename "$artifact")
	case "$base" in
		*.deb) deb_count=$((deb_count + 1)) ;;
		*.iso) iso_count=$((iso_count + 1)) ;;
		*.qcow2|*.raw) machine_image_count=$((machine_image_count + 1)) ;;
	esac
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
	if [ "${LUMONAS_REQUIRE_SIGNATURES:-false}" = "true" ] && [ ! -s "$artifact.bundle" ]; then
		echo "Cosign verification bundle is missing: $artifact.bundle" >&2
		exit 1
	fi
	if [ "${LUMONAS_REQUIRE_SIGNATURES:-false}" = "true" ]; then
		identity="${LUMONAS_COSIGN_CERTIFICATE_IDENTITY_REGEXP:-}"
		issuer="${LUMONAS_COSIGN_CERTIFICATE_OIDC_ISSUER:-}"
		if [ -n "$identity" ] && [ -n "$issuer" ]; then
			cosign verify-blob "$artifact" --bundle "$artifact.bundle" \
				--certificate-identity-regexp "$identity" \
				--certificate-oidc-issuer "$issuer" >/dev/null
		else
			cosign verify-blob "$artifact" --bundle "$artifact.bundle" >/dev/null
		fi
	fi
done

[ "$artifact_count" -gt 0 ] || { echo "no release artifacts found in $RELEASE_DIR" >&2; exit 1; }
if [ "${LUMONAS_REQUIRE_RELEASE_SET:-false}" = "true" ]; then
	[ "$deb_count" -eq 1 ] || { echo "release must contain exactly one Debian package" >&2; exit 1; }
	[ "$iso_count" -ge 1 ] || { echo "release must contain an installer ISO" >&2; exit 1; }
	[ "$machine_image_count" -ge 1 ] || { echo "release must contain a QEMU machine image" >&2; exit 1; }
fi
(cd "$RELEASE_DIR" && sha256sum -c SHA256SUMS >/dev/null)
echo "LumoNAS release artifacts verified: $artifact_count"
