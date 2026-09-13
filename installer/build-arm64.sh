#!/bin/sh
# Build a generic arm64 UEFI disk image preloaded with the LumoNAS appliance
# runtime. The heavy lifting needs Debian tooling (mmdebstrap plus arm64
# package support), so on unsupported hosts the script fails with a clear
# message and CI runs it on an arm64-capable Debian runner instead.
#
# Inputs (environment):
#   LUMONAS_DEB          path to the arm64 LumoNAS .deb (required)
#   LUMONAS_ARM64_IMAGE  output image path (default build/arm64/lumonas-debian13-arm64.img)
#   LUMONAS_ARM64_SIZE   image size      (default 4G)
#   LUMONAS_DEBIAN_MIRROR Debian mirror used for the bootstrap
#
# The image boots into the normal first-boot onboarding: no credentials are
# embedded at build time.
set -eu

VERSION="${1:-0.1.0-dev}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
DEB="${LUMONAS_DEB:-$ROOT/lumonas_${VERSION}_arm64.deb}"
IMAGE="${LUMONAS_ARM64_IMAGE:-$ROOT/build/arm64/lumonas-debian13-arm64.img}"
SIZE="${LUMONAS_ARM64_SIZE:-4G}"
MIRROR="${LUMONAS_DEBIAN_MIRROR:-https://deb.debian.org/debian}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || date +%s)}"
export SOURCE_DATE_EPOCH

[ -f "$DEB" ] || { echo "arm64 package not found: $DEB — run 'make package-arm64' first" >&2; exit 1; }
command -v mmdebstrap >/dev/null 2>&1 || { echo "mmdebstrap is required to build the arm64 image (Debian CI provides it)" >&2; exit 1; }
command -v truncate >/dev/null 2>&1 || { echo "truncate is required" >&2; exit 1; }
command -v losetup >/dev/null 2>&1 || { echo "losetup is required" >&2; exit 1; }
[ "$(id -u)" = "0" ] || { echo "image build requires root (loop mounts)" >&2; exit 1; }
dpkg --print-architecture 2>/dev/null | grep -q arm64 || [ -e /proc/sys/fs/binfmt_misc/qemu-aarch64 ] || {
	echo "arm64 bootstrap needs a native arm64 host or qemu-user-static binfmt registration" >&2
	exit 1
}

mkdir -p "$(dirname "$IMAGE")"
truncate -s "$SIZE" "$IMAGE"
mkfs.ext4 -q -F -L luminasroot "$IMAGE"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-arm64.XXXXXX")"
MNT="$WORK/mnt"
mkdir -p "$MNT"
cleanup() {
	umount "$MNT" 2>/dev/null || true
	losetup -D 2>/dev/null || true
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

LOOP="$(losetup --find --show "$IMAGE")"
mount "$LOOP" "$MNT"

mmdebstrap --variant=apt --architecture=arm64 \
	--include="linux-image-arm64,systemd-sysv,systemd-boot,locales,ca-certificates,e2fsprogs" \
	sid "$MNT" "$MIRROR" >/dev/null

cp "$DEB" "$MNT/tmp/lumonas.deb"
chroot "$MNT" /bin/sh -s <<-'CHROOT'
	set -eu
	apt-get update >/dev/null
	apt-get install --yes --no-install-recommends /tmp/lumonas.deb >/dev/null
	rm -f /tmp/lumonas.deb
	bootctl install >/dev/null
CHROOT

umount "$MNT"
sync
echo "Created arm64 image: $IMAGE"
