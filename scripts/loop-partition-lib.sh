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
#   Mounts the image's root filesystem and records the loop device it attached,
#   keyed by mountpoint. Prints the loop device path on success, nothing on
#   failure. On failure nothing is left attached.
#
# unmount_image_root <mountpoint>
#   Unmounts and detaches what mount_image_root attached for that mountpoint.
#   Safe to call for a mountpoint that was never mounted, which matters because
#   cleanup traps call it unconditionally.
#
# Callers can source this file:
#   . "$(dirname "$0")/loop-partition-lib.sh"

# A script can hold several images mounted at once, so record the pairing
# rather than a single loop device. Kept as "mountpoint<TAB>loop" lines.
LUMONAS_LOOP_REGISTRY=""

# _loop_registry_put / _loop_registry_take
#
# These iterate the registry with a here-document rather than a pipe on purpose:
# a pipeline runs the loop body in a subshell, so the rebuilt registry would be
# discarded and unmount_all_image_roots would never make progress.
#
# _loop_registry_take sets LUMONAS_LOOP_TAKEN rather than printing it, for the
# same reason: a caller that captured stdout with a command substitution would
# throw the registry update away with the subshell.
LUMONAS_LOOP_TAKEN=""

_loop_registry_put() {
	LUMONAS_LOOP_REGISTRY="$LUMONAS_LOOP_REGISTRY$1	$2
"
}

_loop_registry_take() {
	kept=""
	LUMONAS_LOOP_TAKEN=""
	while IFS='	' read -r point value; do
		[ -n "$point" ] || continue
		if [ "$point" = "$1" ]; then
			LUMONAS_LOOP_TAKEN=$value
		else
			kept="$kept$point	$value
"
		fi
	done <<REGISTRY
$LUMONAS_LOOP_REGISTRY
REGISTRY
	LUMONAS_LOOP_REGISTRY=$kept
}

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

	_loop_registry_put "$mountpoint" "$LOOP"
	printf '%s' "$LOOP"
	return 0
}

unmount_image_root() {
	mountpoint=${1:-}
	[ -n "$mountpoint" ] || return 0
	_loop_registry_take "$mountpoint"
	umount "$mountpoint" 2>/dev/null || true
	if [ -n "$LUMONAS_LOOP_TAKEN" ]; then
		losetup -d "$LUMONAS_LOOP_TAKEN" 2>/dev/null || true
	fi
}

# unmount_all_image_roots releases every mount this helper still holds. Useful in
# a cleanup trap that may run with several images attached.
unmount_all_image_roots() {
	guard=0
	while [ -n "$LUMONAS_LOOP_REGISTRY" ] && [ "$guard" -lt 64 ]; do
		guard=$((guard + 1))
		point=${LUMONAS_LOOP_REGISTRY%%	*}
		[ -n "$point" ] || break
		unmount_image_root "$point"
	done
}
