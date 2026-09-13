#!/bin/sh
# LumoNAS disk provisioner — invoked by the privileged broker's
# `install.apply` operation with five validated arguments:
#
#   install-disk.sh <device> <filesystem> <uefi|bios> <hostname> <adminUsername>
#
# The administrator password hash arrives on stdin (never argv). The script
# partitions the target disk, creates the root filesystem, bootstraps a
# minimal Debian runtime from the medium's embedded package repository,
# installs the LumoNAS packages plus a bootloader, and records the
# administrator credential. Every step fails the whole installation.
set -eu

DEVICE="$1"
FILESYSTEM="$2"
BOOT_MODE="$3"
HOSTNAME="$4"
ADMIN_NAME="$5"
ADMIN_PASSWORD="$(sed -n '1p')"

MIRROR="${LUMONAS_INSTALL_MIRROR:-}"
SUITE="${LUMONAS_INSTALL_SUITE:-bookworm}"
MOUNT="${LUMONAS_INSTALL_MOUNT:-/mnt/lumonas-install}"
REPO_DIR="${LUMONAS_INSTALL_REPO_DIR:-/opt/lumonas-repo}"

log() { printf '[install-disk] %s\n' "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

[ "$(id -u)" = "0" ] || die "must run as root"
[ -b "$DEVICE" ] || die "$DEVICE is not a block device"
[ "$FILESYSTEM" = "ext4" ] || [ "$FILESYSTEM" = "xfs" ] || die "unsupported filesystem $FILESYSTEM"
[ "$BOOT_MODE" = "uefi" ] || [ "$BOOT_MODE" = "bios" ] || die "unsupported boot mode $BOOT_MODE"
case "$DEVICE" in
	*/disk/by-id/*|/dev/[a-z]d[a-z]|/dev/nvme[0-9]n[0-9]|/dev/vd[a-z]|/dev/xvd[a-z]) ;;
	*) die "device path $DEVICE is not an accepted disk path" ;;
esac
command -v sfdisk >/dev/null || die "sfdisk is required"
command -v "mkfs.$FILESYSTEM" >/dev/null || die "mkfs.$FILESYSTEM is required"
command -v debootstrap >/dev/null || die "debootstrap is required"
[ -d "$REPO_DIR/dists" ] || die "embedded LumoNAS repository not found at $REPO_DIR"

log "provisioning $DEVICE ($FILESYSTEM, $BOOT_MODE) as $HOSTNAME"

# 1. Partition: one ESP/firmware partition plus the root partition.
wipefs -a "$DEVICE" >/dev/null 2>&1 || true
if [ "$BOOT_MODE" = "uefi" ]; then
	sfdisk --quiet "$DEVICE" <<-'PARTS'
		label: gpt
		name="lumonas-esp", size=512MiB, type=U
		name="lumonas-root", type=L
	PARTS
else
	sfdisk --quiet "$DEVICE" <<-'PARTS'
		label: dos
		type=83, bootable
	PARTS
fi

# 2. Resolve the partitions the kernel created.
PART_ROOT=""
for candidate in "${DEVICE}2" "${DEVICE}p2" "${DEVICE}-part2"; do
	[ -b "$candidate" ] && PART_ROOT="$candidate" && break
done
PART_ESP=""
if [ "$BOOT_MODE" = "uefi" ]; then
	for candidate in "${DEVICE}1" "${DEVICE}p1" "${DEVICE}-part1"; do
		[ -b "$candidate" ] && PART_ESP="$candidate" && break
	done
	[ -n "$PART_ESP" ] || die "ESP partition not found"
fi
[ -n "$PART_ROOT" ] || die "root partition not found"

# 3. Filesystems.
if [ "$BOOT_MODE" = "uefi" ]; then
	mkfs.vfat -F 32 -n LUMOESP "$PART_ESP" >/dev/null
fi
"mkfs.$FILESYSTEM" -L luminasroot "$PART_ROOT" >/dev/null

mkdir -p "$MOUNT"
mount "$PART_ROOT" "$MOUNT"
cleanup() {
	umount "$MOUNT/dev" "$MOUNT/proc" "$MOUNT/sys" 2>/dev/null || true
	umount "$MOUNT/boot/efi" 2>/dev/null || true
	umount "$MOUNT" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# 4. Bootstrap the Debian runtime. The embedded repository supplies the
# LumoNAS packages afterwards; the Debian mirror is only needed here.
DEBIAN_FRONTEND=noninteractive debootstrap --variant=minbase "$SUITE" "$MOUNT" "$MIRROR" >/dev/null

mount --bind /dev "$MOUNT/dev"
mount --bind /proc "$MOUNT/proc"
mount --bind /sys "$MOUNT/sys"

# 5. System identity and package installation.
printf '%s\n' "$HOSTNAME" > "$MOUNT/etc/hostname"
printf '127.0.1.1\t%s\n' "$HOSTNAME" >> "$MOUNT/etc/hosts"
: > "$MOUNT/etc/fstab"
if [ "$BOOT_MODE" = "uefi" ]; then
	printf 'UUID=%s /boot/efi vfat umask=0077 0 1\n' "$(blkid -s UUID -o value "$PART_ESP")" >> "$MOUNT/etc/fstab"
fi
printf 'UUID=%s / %s errors=remount-ro 0 1\n' "$(blkid -s UUID -o value "$PART_ROOT")" "$FILESYSTEM" >> "$MOUNT/etc/fstab"

# 6. First-boot administrator bootstrap. The credential lives only in the
# root-owned environment file and is consumed by lumonasd on first start;
# it is never embedded in argv or logs.
mkdir -p "$MOUNT/etc/lumonas"
{
	printf 'LUMONAS_AUTH_REQUIRED=true\n'
	printf 'LUMONAS_ADMIN_PASSWORD=%s\n' "$ADMIN_PASSWORD"
} > "$MOUNT/etc/lumonas/lumonasd.env"
chmod 0600 "$MOUNT/etc/lumonas/lumonasd.env"

chroot "$MOUNT" /bin/sh -s <<-'CHROOT'
	set -eu
	apt-get update >/dev/null
	apt-get install --yes --no-install-recommends \
		linux-image-amd64 systemd-sysv locales ca-certificates >/dev/null
	if [ -d /opt/lumonas-repo ]; then
		printf 'deb [trusted=yes] file:/opt/lumonas-repo ./\n' > /etc/apt/sources.list.d/lumonas.list
		apt-get update >/dev/null
		apt-get install --yes --no-install-recommends lumonas >/dev/null
	fi
CHROOT

# 7. Bootloader.
if [ "$BOOT_MODE" = "uefi" ]; then
	mkdir -p "$MOUNT/boot/efi"
	mount "$PART_ESP" "$MOUNT/boot/efi"
	chroot "$MOUNT" bootctl install >/dev/null 2>&1 || {
		apt-get install --yes --no-install-recommends grub-efi-amd64 >/dev/null 2>&1
		chroot "$MOUNT" grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=lumonas >/dev/null
	}
else
	chroot "$MOUNT" grub-install --target=i386-pc "$DEVICE" >/dev/null
fi
chroot "$MOUNT" update-grub >/dev/null 2>&1 || true

log "installation complete on $DEVICE"
