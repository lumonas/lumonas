#!/bin/sh
# PXE bundle smoke: validates the builder always, and — when an ISO is
# provided with LUMONAS_NETBOOT_ASSERT=true — builds the bundle and verifies
# its structure.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
VERSION="${1:-0.1.0-dev}"
ISO="${LUMONAS_ISO:-}"

sh -n "$ROOT/installer/build-netboot.sh" || {
	echo "build-netboot.sh has syntax errors" >&2
	exit 1
}
grep -q 'netboot' "$ROOT/Makefile" || {
	echo "Makefile is missing the netboot target" >&2
	exit 1
}

if [ "${LUMONAS_NETBOOT_ASSERT:-false}" != "true" ]; then
	echo "LumoNAS netboot packaging smoke passed (build gate disabled; set LUMONAS_NETBOOT_ASSERT=true with an ISO)"
	exit 0
fi

[ -n "$ISO" ] && [ -f "$ISO" ] || { echo "set LUMONAS_ISO to a built offline ISO" >&2; exit 1; }
command -v xorriso >/dev/null 2>&1 || { echo "xorriso missing for netboot bundle build" >&2; exit 1; }

LUMONAS_ISO="$ISO" LUMONAS_NETBOOT_DIR="${TMPDIR:-/tmp}/lumonas-netboot-check" \
	bash "$ROOT/installer/build-netboot.sh" "$VERSION"

BUNDLE="${TMPDIR:-/tmp}/lumonas-netboot-check/lumonas-netboot-$VERSION.tar.gz"
[ -s "$BUNDLE" ] || { echo "netboot bundle was not produced" >&2; exit 1; }
for member in vmlinuz initrd.img lumonas.squashfs grub/grub.cfg ipxe.script README.txt; do
	tar -tzf "$BUNDLE" | grep -Fx "$member" >/dev/null || {
		echo "netboot bundle is missing $member" >&2
		exit 1
	}
done
rm -rf "${TMPDIR:-/tmp}/lumonas-netboot-check"
echo "LumoNAS netboot smoke passed"
