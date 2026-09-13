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
MIRROR="${LUMONAS_DEBIAN_MIRROR:-https://snapshot.debian.org/archive/debian/20260201T000000Z/}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || date +%s)}"
export SOURCE_DATE_EPOCH

[ -f "$DEB" ] || { echo "arm64 package not found: $DEB — run 'make package-arm64' first" >&2; exit 1; }
[ "$(dpkg-deb -f "$DEB" Architecture)" = "arm64" ] || { echo "arm64 image requires an arm64 LumoNAS package" >&2; exit 1; }
command -v mmdebstrap >/dev/null 2>&1 || { echo "mmdebstrap is required to build the arm64 image (Debian CI provides it)" >&2; exit 1; }
command -v truncate >/dev/null 2>&1 || { echo "truncate is required" >&2; exit 1; }
command -v losetup >/dev/null 2>&1 || { echo "losetup is required" >&2; exit 1; }
command -v sfdisk >/dev/null 2>&1 || { echo "sfdisk is required" >&2; exit 1; }
command -v partx >/dev/null 2>&1 || { echo "partx is required" >&2; exit 1; }
command -v mkfs.vfat >/dev/null 2>&1 || { echo "mkfs.vfat is required" >&2; exit 1; }
command -v blkid >/dev/null 2>&1 || { echo "blkid is required" >&2; exit 1; }
command -v mount >/dev/null 2>&1 || { echo "mount is required" >&2; exit 1; }
[ "$(id -u)" = "0" ] || { echo "image build requires root (loop mounts)" >&2; exit 1; }
dpkg --print-architecture 2>/dev/null | grep -q arm64 || [ -e /proc/sys/fs/binfmt_misc/qemu-aarch64 ] || {
	echo "arm64 bootstrap needs a native arm64 host or qemu-user-static binfmt registration" >&2
	exit 1
}

mkdir -p "$(dirname "$IMAGE")"
[ ! -e "$IMAGE" ] || { echo "refusing to overwrite existing image: $IMAGE" >&2; exit 1; }
truncate -s "$SIZE" "$IMAGE"
printf 'label: gpt\n,256M,U\n,,L\n' | sfdisk "$IMAGE" >/dev/null

WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-arm64.XXXXXX")"
MNT="$WORK/mnt"
mkdir -p "$MNT"
cleanup() {
	umount "$MNT/boot/efi" 2>/dev/null || true
	umount -R "$MNT/dev" 2>/dev/null || true
	umount -R "$MNT/proc" 2>/dev/null || true
	umount -R "$MNT/sys" 2>/dev/null || true
	umount "$MNT" 2>/dev/null || true
	if [ -n "${LOOP:-}" ]; then
		partx --delete "$LOOP" 2>/dev/null || true
		losetup -d "$LOOP" 2>/dev/null || true
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

LOOP="$(losetup --find --show --partscan "$IMAGE")"
partx --add "$LOOP"
EFI="${LOOP}p1"
ROOT_PART="${LOOP}p2"
[ -b "$EFI" ] && [ -b "$ROOT_PART" ] || { echo "partition devices were not created for $IMAGE" >&2; exit 1; }
mkfs.vfat -F 32 -n LUMONAS_EFI "$EFI" >/dev/null
mkfs.ext4 -q -F -L lumonasroot "$ROOT_PART"
mount "$ROOT_PART" "$MNT"
mkdir -p "$MNT/boot/efi"
mount "$EFI" "$MNT/boot/efi"

mmdebstrap --variant=apt --architecture=arm64 \
	--aptopt='Acquire::Check-Valid-Until "false"' \
	--include="linux-image-arm64,grub-efi-arm64,systemd-sysv,locales,ca-certificates,e2fsprogs" \
	trixie "$MNT" "$MIRROR" >/dev/null

mkdir -p "$MNT/dev" "$MNT/proc" "$MNT/sys"

mount --rbind /dev "$MNT/dev"
mount --make-rslave "$MNT/dev"
mount -t proc proc "$MNT/proc"
mount --rbind /sys "$MNT/sys"
mount --make-rslave "$MNT/sys"
rm -f "$MNT/etc/resolv.conf"
cp /etc/resolv.conf "$MNT/etc/resolv.conf"

cp "$DEB" "$MNT/tmp/lumonas.deb"
ROOT_UUID="$(blkid -s UUID -o value "$ROOT_PART")"
EFI_UUID="$(blkid -s UUID -o value "$EFI")"
cat > "$MNT/etc/fstab" <<FSTAB
UUID=$ROOT_UUID / ext4 defaults 0 1
UUID=$EFI_UUID /boot/efi vfat umask=0077 0 1
FSTAB
chroot "$MNT" /bin/sh -s <<-'CHROOT'
	set -eu
	export DEBIAN_FRONTEND=noninteractive
	apt-get update >/dev/null
	apt-get install --yes --no-install-recommends /tmp/lumonas.deb >/dev/null
	rm -f /tmp/lumonas.deb
	grub-install --target=arm64-efi --efi-directory=/boot/efi --boot-directory=/boot --removable --no-nvram >/dev/null
	update-grub >/dev/null
	systemctl enable lumonas-runtime.service lumonas-privd.service lumonas-privd-storage.service lumonas-privd-network.service lumonas-privd-power.service lumonas-privd-general.service lumonas-jobs.target lumonas-services.target lumonas-storage.target lumonasd.service lumonas-web.service
CHROOT

umount "$MNT"
sync
echo "Created arm64 image: $IMAGE"
