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
# no filesystem of its own and fails with "wrong fs type". The shared helper
# handles both image shapes.
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

# shellcheck source=scripts/loop-partition-lib.sh
. "$ROOT/scripts/loop-partition-lib.sh"

cleanup() {
	unmount_image_root "$CLEAN_MOUNT"
	rmdir "$CLEAN_MOUNT" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

mount_image_root "$IMAGE" "$CLEAN_MOUNT" >/dev/null

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

unmount_image_root "$CLEAN_MOUNT"

echo "QEMU appliance image cleaned of ephemeral credentials: $IMAGE"
