#!/bin/sh
# Remove the ephemeral test credentials from a built QEMU appliance image.
#
# The QEMU smoke tests install a throwaway SSH public key and an update fixture
# so the running appliance can be driven and upgraded over the network. That
# image is then uploaded as a build artifact, so the key has to come back out
# first: an artifact carrying a known-public key is an appliance anyone can log
# into.
#
# The image is a partitioned GPT disk, not a bare filesystem, so it cannot be
# mounted with "mount -o loop": that attaches the whole-disk device, which has
# no filesystem of its own and fails with "wrong fs type". Attach the loop
# device with partition scanning and mount the root partition instead.
#
# Exits non-zero if the image exists but could not be cleaned. A silent
# success would be worse than a failure, because the artifact would then carry
# the key.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
IMAGE="${LUMONAS_QEMU_IMAGE:-$ROOT/build/qemu/lumonas-debian13.raw}"
CLEAN_MOUNT="${LUMONAS_QEMU_CLEAN_MOUNT:-/tmp/lumonas-qemu-clean}"

if [ ! -e "$IMAGE" ]; then
	echo "no image to clean at $IMAGE"
	exit 0
fi
if [ "$(id -u)" -ne 0 ]; then
	echo "Run this as root, for example: sudo $0" >&2
	exit 1
fi
for command in losetup partx mount umount blkid; do
	command -v "$command" >/dev/null 2>&1 || {
		echo "$command is required to clean the image" >&2
		exit 1
	}
done

LOOP=""
MOUNTED=""
cleanup() {
	[ -n "$MOUNTED" ] && umount "$MOUNTED" 2>/dev/null || true
	[ -n "$LOOP" ] && losetup -d "$LOOP" 2>/dev/null || true
	rmdir "$CLEAN_MOUNT" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Prefer the filesystem label, so this keeps working if the partition layout
# changes. Fall back to the third partition, which is the root filesystem the
# builder creates.
LOOP="$(losetup --find --show --partscan "$IMAGE")"
# Only refresh the partition table when there is one, so this stays quiet on an
# image that failed to build and never got partitioned.
if [ -n "$(blkid -s PTTYPE -o value "$LOOP" 2>/dev/null || true)" ]; then
	partx -u "$LOOP" || true
fi
case "$LOOP" in
	*[0-9]) root_part="${LOOP}p3" ;;
	*) root_part="${LOOP}3" ;;
esac
for candidate in "$root_part" "$LOOP"; do
	if [ "$(blkid -s TYPE -o value "$candidate" 2>/dev/null || true)" = "ext4" ]; then
		root_part=$candidate
		break
	fi
done

mkdir -p "$CLEAN_MOUNT"
mount "$root_part" "$CLEAN_MOUNT"
MOUNTED="$CLEAN_MOUNT"

# The SSH key, its enabling symlink, and the update fixture's staged package.
rm -f "$CLEAN_MOUNT/root/.ssh/authorized_keys"
rmdir "$CLEAN_MOUNT/root/.ssh" 2>/dev/null || true
rm -f "$CLEAN_MOUNT/etc/systemd/system/multi-user.target.wants/ssh.service"
rm -f "$CLEAN_MOUNT/var/lib/lumonas/update-fixture/package"
rmdir "$CLEAN_MOUNT/var/lib/lumonas/update-fixture" 2>/dev/null || true
sed -i '/^LUMONAS_UPDATE_PUBLIC_KEY=/d' "$CLEAN_MOUNT/etc/lumonas/lumonasd.env"

# Prove the key is really gone rather than assuming the removal worked.
if [ -e "$CLEAN_MOUNT/root/.ssh/authorized_keys" ]; then
	echo "the ephemeral SSH key is still present after cleaning" >&2
	exit 1
fi
if grep -q '^LUMONAS_UPDATE_PUBLIC_KEY=' "$CLEAN_MOUNT/etc/lumonas/lumonasd.env" 2>/dev/null; then
	echo "LUMONAS_UPDATE_PUBLIC_KEY is still present after cleaning" >&2
	exit 1
fi

umount "$CLEAN_MOUNT"
MOUNTED=""
losetup -d "$LOOP"
LOOP=""

echo "QEMU appliance image cleaned of ephemeral credentials: $IMAGE"
