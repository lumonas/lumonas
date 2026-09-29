#!/bin/sh
# Attach and mount the root filesystem of a raw disk image.
#
# Raw disk images come in two shapes, and telling them apart matters:
#
#   * A plain filesystem image, made with truncate and mkfs.ext4. The file
#     itself is the filesystem, so "mount -o loop" is correct.
#
#   * A partitioned GPT disk, like the QEMU appliance. The file is a disk, not a
#     filesystem, so "mount -o loop" attaches the whole-disk device, which has
#     no filesystem of its own and fails with "wrong fs type". The loop device
#     has to be scanned for partitions and the root partition mounted. This is
#     easy to get wrong, and the resulting error names the image rather than the
#     cause, so it reads as a corrupt image.
#
# mount_image_root <image> <mountpoint> [mount options...]
#   Mounts the image's root filesystem and records the loop device it attached.
#   Prints the loop device path on success, nothing on failure. On failure
#   nothing is left attached.
#
# unmount_image_root <mountpoint>
#   Unmounts and detaches what mount_image_root attached. Safe to call when
#   nothing is attached.
#
# Callers can source this file:
#   . "$(dirname "$0")/loop-partition-lib.sh"

LUMONAS_LOOP_DEVICE=""
LUMONAS_LOOP_MOUNTPOINT=""

mount_image_root() {
	image=$1
	mountpoint=$2
	shift 2
	extra_options="$*"

	[ -f "$image" ] || { echo "no such disk image: $image" >&2; return 1; }

	LOOP="$(losetup --find --show --partscan "$image")" || return 1

	# Only refresh the partition table when there is one, so this stays quiet on
	# an image that is a bare filesystem.
	if [ -n "$(blkid -s PTTYPE -o value "$LOOP" 2>/dev/null || true)" ]; then
		partx -u "$LOOP" 2>/dev/null || true
	fi

	# A partitioned appliance puts its root filesystem third, but prefer the
	# filesystem that is actually labelled as one so the helper does not depend
	# on the partition order staying fixed.
	candidate=""
	case "$LOOP" in
		*[0-9]) default_root="${LOOP}p3" ;;
		*) default_root="${LOOP}3" ;;
	esac
	for device in "$default_root" "$LOOP"; do
		[ -b "$device" ] || continue
		if [ "$(blkid -s TYPE -o value "$device" 2>/dev/null || true)" = "ext4" ]; then
			candidate=$device
			break
		fi
	done
	if [ -z "$candidate" ]; then
		# A partitioned image whose partitions are not visible, for example in a
		# container without loop partition support. Say so, because the bare
		# "wrong fs type" is what made this so hard to diagnose.
		echo "could not find an ext4 filesystem in $image (tried ${default_root} and $LOOP)" >&2
		losetup -d "$LOOP" 2>/dev/null || true
		return 1
	fi

	mkdir -p "$mountpoint"
	if ! mount $extra_options "$candidate" "$mountpoint"; then
		losetup -d "$LOOP" 2>/dev/null || true
		return 1
	fi

	LUMONAS_LOOP_DEVICE=$LOOP
	LUMONAS_LOOP_MOUNTPOINT=$mountpoint
	printf '%s' "$LOOP"
	return 0
}

unmount_image_root() {
	mountpoint=${1:-$LUMONAS_LOOP_MOUNTPOINT}
	if [ -n "$mountpoint" ]; then
		umount "$mountpoint" 2>/dev/null || true
	fi
	if [ -n "$LUMONAS_LOOP_DEVICE" ]; then
		losetup -d "$LUMONAS_LOOP_DEVICE" 2>/dev/null || true
	fi
	LUMONAS_LOOP_DEVICE=""
	LUMONAS_LOOP_MOUNTPOINT=""
}
