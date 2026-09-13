#!/bin/sh
# Arm64 image variant smoke: validates the packaging parameterization always,
# and — when run on an arm64-capable Debian host with LUMONAS_ARM64_ASSERT=true
# — performs a real image build and structural verification.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
VERSION="${1:-0.1.0-dev}"

grep -q 'LUMONAS_DEB_ARCH' "$ROOT/packaging/build-deb.sh" || {
	echo "build-deb.sh is not architecture-parameterized" >&2
	exit 1
}
grep -q 'package-arm64' "$ROOT/Makefile" || {
	echo "Makefile is missing the package-arm64 target" >&2
	exit 1
}
sh -n "$ROOT/installer/build-arm64.sh" || {
	echo "build-arm64.sh has syntax errors" >&2
	exit 1
}

if [ "${LUMONAS_ARM64_ASSERT:-false}" != "true" ]; then
	echo "LumoNAS arm64 packaging smoke passed (build gate disabled; set LUMONAS_ARM64_ASSERT=true on an arm64 Debian host)"
	exit 0
fi

# Full build path: only on a real Debian host with the tooling present.
command -v mmdebstrap >/dev/null 2>&1 || { echo "mmdebstrap missing for full arm64 build" >&2; exit 1; }
[ "$(id -u)" = "0" ] || { echo "full arm64 build requires root" >&2; exit 1; }

bash "$ROOT/scripts/reproducible-package-smoke.sh" "$VERSION"
LUMONAS_DEB="$ROOT/lumonas_${VERSION}_arm64.deb" \
LUMONAS_ARM64_IMAGE="${TMPDIR:-/tmp}/lumonas-arm64-check.img" \
LUMONAS_ARM64_SIZE=2G \
bash "$ROOT/installer/build-arm64.sh" "$VERSION"

IMAGE="${TMPDIR:-/tmp}/lumonas-arm64-check.img"
[ -s "$IMAGE" ] || { echo "arm64 image was not produced" >&2; exit 1; }
debugfs -R "ls /usr/lib/lumonas" "$IMAGE" 2>/dev/null | grep -q lumonasd || {
	echo "arm64 image is missing the appliance runtime" >&2
	exit 1
}
rm -f "$IMAGE"
echo "LumoNAS arm64 image smoke passed"
