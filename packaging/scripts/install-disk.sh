#!/bin/sh
# LumoNAS offline system-disk provisioner.
#
# Arguments:
#   install-disk.sh <device> <filesystem> <uefi|bios> <hostname> <adminUsername>
#
# The live ISO already contains the pinned Debian runtime and the LumoNAS
# package. The target is built by copying that runtime, so the installation
# path never needs a network mirror or a package download.
set -eu

DEVICE_INPUT="${1:-}"
FILESYSTEM="${2:-}"
BOOT_MODE="${3:-}"
HOSTNAME="${4:-}"
ADMIN_NAME="${5:-}"
ADMIN_PASSWORD="$(sed -n '1p')"
MOUNT="${LUMONAS_INSTALL_MOUNT:-/mnt/lumonas-install}"

log() { printf '[install-disk] %s\n' "$*"; }
die() { log "ERROR: $*"; exit 1; }

[ "$(id -u)" = "0" ] || die "must run as root"
[ -n "$ADMIN_PASSWORD" ] || die "administrator password is required"
[ "$FILESYSTEM" = "ext4" ] || [ "$FILESYSTEM" = "xfs" ] || die "unsupported filesystem $FILESYSTEM"
[ "$BOOT_MODE" = "uefi" ] || [ "$BOOT_MODE" = "bios" ] || die "unsupported boot mode $BOOT_MODE"
printf '%s' "$HOSTNAME" | grep -Eq '^[a-z0-9][a-z0-9-]{0,62}$' || die "hostname is invalid"
printf '%s' "$ADMIN_NAME" | grep -Eq '^[a-z_][a-z0-9_-]{0,31}$' || die "administrator name is invalid"

case "$MOUNT" in
	''|/|/dev|/proc|/sys|/run|/tmp|/var|/var/*) die "unsafe installation mount path" ;;
	/*) ;;
	*) die "installation mount path must be absolute" ;;
esac

command -v readlink >/dev/null 2>&1 || die "readlink is required"
for command in sfdisk rsync findmnt lsblk partx udevadm base64 wipefs blkid mount umount chroot grub-install update-initramfs update-grub; do
	command -v "$command" >/dev/null 2>&1 || die "$command is required"
done
command -v "mkfs.$FILESYSTEM" >/dev/null 2>&1 || die "mkfs.$FILESYSTEM is required"
[ "$BOOT_MODE" != "uefi" ] || command -v mkfs.vfat >/dev/null 2>&1 || die "mkfs.vfat is required"

[ -b "$DEVICE_INPUT" ] || die "$DEVICE_INPUT is not a block device"
DEVICE="$(readlink -f "$DEVICE_INPUT")"
[ -b "$DEVICE" ] || die "could not resolve installation device"
case "$DEVICE" in
	/dev/[a-z]d[a-z]|/dev/nvme[0-9]n[0-9]|/dev/vd[a-z]|/dev/xvd[a-z]) ;;
	*) die "device path $DEVICE is not an accepted whole-disk path" ;;
esac

if mounts="$(findmnt -rn -S "$DEVICE" 2>/dev/null)"; then
	[ -z "$mounts" ] || die "installation device is mounted"
else
	status=$?
	[ "$status" -eq 1 ] || die "could not verify installation device mounts"
fi
mounts="$(lsblk -nrpo MOUNTPOINT -- "$DEVICE")" || die "could not inspect installation device mounts"
while IFS= read -r mountpoint; do
	case "$mountpoint" in
		''|-) ;;
		*) die "installation device has mounted partitions" ;;
	esac
done <<EOF
$mounts
EOF

log "provisioning $DEVICE_INPUT ($FILESYSTEM, $BOOT_MODE) as $HOSTNAME"
wipefs -a -f "$DEVICE" >/dev/null

if [ "$BOOT_MODE" = "uefi" ]; then
	sfdisk --wipe always --wipe-partitions always --quiet "$DEVICE" <<-'PARTS'
		label: gpt
		size=512MiB, type=U, name="LumoNAS ESP"
		type=L, name="LumoNAS root"
	PARTS
else
	sfdisk --wipe always --wipe-partitions always --quiet "$DEVICE" <<-'PARTS'
		label: dos
		type=83, bootable
	PARTS
fi

partx --update "$DEVICE" >/dev/null 2>&1 || die "could not refresh the partition table"
udevadm settle --timeout=10

partition_path() {
	index="$1"
	for candidate in "${DEVICE}${index}" "${DEVICE}p${index}" "${DEVICE}-part${index}"; do
		if [ -b "$candidate" ]; then
			printf '%s' "$candidate"
			return 0
		fi
	done
	return 1
}

if [ "$BOOT_MODE" = "uefi" ]; then
	PART_ESP="$(partition_path 1)" || die "ESP partition was not created"
	PART_ROOT="$(partition_path 2)" || die "root partition was not created"
	mkfs.vfat -F 32 -n LUMOESP "$PART_ESP" >/dev/null
else
	PART_ESP=""
	PART_ROOT="$(partition_path 1)" || die "root partition was not created"
fi

if [ "$FILESYSTEM" = "ext4" ]; then
	mkfs.ext4 -F -L lumonasroot "$PART_ROOT" >/dev/null
else
	mkfs.xfs -f -L lumonasroot "$PART_ROOT" >/dev/null
fi

mkdir -p "$MOUNT"
mount "$PART_ROOT" "$MOUNT"
cleanup() {
	umount "$MOUNT/boot/efi" 2>/dev/null || true
	umount "$MOUNT/dev" 2>/dev/null || true
	umount "$MOUNT/proc" 2>/dev/null || true
	umount "$MOUNT/sys" 2>/dev/null || true
	umount "$MOUNT" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Copy the live Debian runtime, excluding virtual filesystems and the target
# mount itself. This is the offline installation boundary.
rsync -aHAX --numeric-ids --one-file-system \
	--exclude=/dev/*** \
	--exclude=/proc/*** \
	--exclude=/sys/*** \
	--exclude=/run/*** \
	--exclude=/tmp/*** \
	--exclude=/var/tmp/*** \
	--exclude=/mnt/*** \
	--exclude="$MOUNT/***" \
	/ "$MOUNT/"

mkdir -p "$MOUNT/etc/lumonas" "$MOUNT/var/lib/lumonas/secrets" "$MOUNT/boot"
if [ -f "$MOUNT/etc/lumonas/lumonasd.env" ]; then
	sed -i '/^LUMONAS_INSTALLER_MODE=/d' "$MOUNT/etc/lumonas/lumonasd.env"
fi
printf '%s\n' "$HOSTNAME" >"$MOUNT/etc/hostname"
if ! grep -Eq '^[[:space:]]*127\.0\.1\.1[[:space:]]' "$MOUNT/etc/hosts"; then
	printf '127.0.1.1\t%s\n' "$HOSTNAME" >>"$MOUNT/etc/hosts"
fi

root_uuid="$(blkid -s UUID -o value "$PART_ROOT")"
[ -n "$root_uuid" ] || die "root filesystem UUID is missing"
printf 'UUID=%s / %s errors=remount-ro 0 1\n' "$root_uuid" "$FILESYSTEM" >"$MOUNT/etc/fstab"
if [ "$BOOT_MODE" = "uefi" ]; then
	mkdir -p "$MOUNT/boot/efi"
	mount "$PART_ESP" "$MOUNT/boot/efi"
	esp_uuid="$(blkid -s UUID -o value "$PART_ESP")"
	[ -n "$esp_uuid" ] || die "EFI filesystem UUID is missing"
	printf 'UUID=%s /boot/efi vfat umask=0077 0 1\n' "$esp_uuid" >>"$MOUNT/etc/fstab"
fi

# The live image already contains the service account from package postinst.
# Store the password only in the account-owned first-boot secret; lumonasd
# removes it after creating the application administrator.
first_boot="$MOUNT/var/lib/lumonas/secrets/first-boot.env"
umask 077
encoded_password="$(printf '%s' "$ADMIN_PASSWORD" | base64 | tr -d '\n')"
printf 'LUMONAS_ADMIN_USERNAME=%s\nLUMONAS_ADMIN_PASSWORD_B64=%s\n' "$ADMIN_NAME" "$encoded_password" >"$first_boot"
chown lumonas:lumonas "$first_boot"
chmod 0600 "$first_boot"

mount --bind /dev "$MOUNT/dev"
mount --bind /proc "$MOUNT/proc"
mount --bind /sys "$MOUNT/sys"

chroot "$MOUNT" /bin/sh -s -- "$ADMIN_NAME" <<-'CHROOT'
	set -eu
	admin_name="$1"
	systemctl enable lumonas-runtime.service lumonas-privd.service lumonas-privd-storage.service lumonas-privd-network.service lumonas-privd-power.service lumonas-privd-general.service lumonas-privd-acme.service lumonas-jobs.target lumonas-services.target lumonas-storage.target lumonasd.service lumonas-web.service
	update-initramfs -u -k all >/dev/null 2>&1
	if [ "$admin_name" = "root" ]; then
		echo "administrator name may not be root" >&2
		exit 1
	fi
CHROOT

if [ "$BOOT_MODE" = "uefi" ]; then
	chroot "$MOUNT" grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=LumoNAS --removable --no-nvram >/dev/null
else
	chroot "$MOUNT" grub-install --target=i386-pc "$DEVICE" >/dev/null
fi
chroot "$MOUNT" update-grub >/dev/null 2>&1

sync
log "installation complete on $DEVICE_INPUT"
