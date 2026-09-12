#!/bin/sh
set -eu

ASSERT_MODE="${LUMONAS_STORAGE_ASSERT:-false}"

if [ "$(id -u)" -ne 0 ]; then
	if [ "$ASSERT_MODE" = "true" ]; then
		echo "root is required for loopback storage assertions" >&2
		exit 1
	fi
	echo "loopback storage smoke test skipped: root is required" >&2
	exit 0
fi

for command in blkid losetup mount umount mkfs.ext4 mkfs.xfs wipefs truncate findmnt mergerfs snapraid; do
	command -v "$command" >/dev/null 2>&1 || {
		echo "$command is required for loopback storage assertions" >&2
		exit 1
	}
done

WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-storage.XXXXXX")"
LOOPS=""
cleanup() {
	set +e
	for mountpoint in "$WORK/pool" "$WORK/branch-a" "$WORK/branch-b" "$WORK/ext4-mount" "$WORK/xfs-mount"; do
		umount "$mountpoint" 2>/dev/null || true
	done
	for loop in $LOOPS; do
		losetup -d "$loop" 2>/dev/null || true
	done
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

mkdir -p "$WORK/ext4-mount" "$WORK/xfs-mount" "$WORK/branch-a" "$WORK/branch-b" "$WORK/pool"

make_disk() {
	image=$1
	filesystem=$2
	truncate -s 64M "$image"
	loop=$(losetup --find --show "$image")
	LOOPS="$LOOPS $loop"
	LAST_LOOP=$loop
	case "$filesystem" in
		ext4)
		mkfs.ext4 -F -L lumonas-test "$loop" >/dev/null
		;;
		xfs)
		mkfs.xfs -f -L lumonas-test "$loop" >/dev/null
		;;
		*)
			echo "unsupported test filesystem: $filesystem" >&2
			exit 1
		;;
	esac
	LAST_UUID=$(blkid -s UUID -o value "$loop")
}

EXT4_IMAGE="$WORK/ext4.img"
make_disk "$EXT4_IMAGE" ext4
EXT4_UUID=$LAST_UUID
EXT4_LOOP=$LAST_LOOP
[ -n "$EXT4_UUID" ] && [ -n "$EXT4_LOOP" ] || { echo "ext4 identity discovery failed" >&2; exit 1; }

# Reattach the same image and prove identity comes from the filesystem, not
# the transient /dev/loop letter.
losetup -d "$EXT4_LOOP"
LOOPS=""
EXT4_LOOP="$(losetup --find --show "$EXT4_IMAGE")"
LOOPS="$EXT4_LOOP"
REATTACHED_UUID="$(blkid -s UUID -o value "$EXT4_LOOP")"
[ "$REATTACHED_UUID" = "$EXT4_UUID" ] || {
	echo "ext4 UUID changed after loop-device reattachment" >&2
	exit 1
}

mount "$EXT4_LOOP" "$WORK/ext4-mount"
printf '%s\n' 'stable identity smoke test' >"$WORK/ext4-mount/sentinel"
umount "$WORK/ext4-mount"
mount -o ro "$EXT4_LOOP" "$WORK/ext4-mount"
grep -Fx 'stable identity smoke test' "$WORK/ext4-mount/sentinel" >/dev/null
if touch "$WORK/ext4-mount/readonly-must-fail" 2>/dev/null; then
	echo "read-only ext4 import unexpectedly allowed a write" >&2
	exit 1
fi
umount "$WORK/ext4-mount"

MISMATCH_IMAGE="$WORK/mismatch.img"
make_disk "$MISMATCH_IMAGE" ext4
MISMATCH_UUID=$LAST_UUID
MISMATCH_LOOP=$LAST_LOOP
[ "$MISMATCH_UUID" != "$EXT4_UUID" ] || {
	echo "independent ext4 test disks unexpectedly share an identity" >&2
	exit 1
}
[ -n "$MISMATCH_LOOP" ] || { echo "mismatch disk attachment failed" >&2; exit 1; }

XFS_IMAGE="$WORK/xfs.img"
make_disk "$XFS_IMAGE" xfs
XFS_UUID=$LAST_UUID
XFS_LOOP=$LAST_LOOP
[ -n "$XFS_UUID" ] && [ -n "$XFS_LOOP" ] || { echo "xfs identity discovery failed" >&2; exit 1; }
mount -o ro "$XFS_LOOP" "$WORK/xfs-mount"
findmnt -rn -o FSTYPE "$WORK/xfs-mount" | grep -Fx xfs >/dev/null
umount "$WORK/xfs-mount"

# Exercise the destructive filesystem lifecycle only on a disposable loopback
# image. The release gate still relies on the broker/unit tests for identity,
# plan expiry, and mounted-disk rejection; this proves the real tools mutate
# and then clear test media as expected.
MUTATION_IMAGE="$WORK/mutation.img"
make_disk "$MUTATION_IMAGE" ext4
MUTATION_LOOP=$LAST_LOOP
MUTATION_UUID=$LAST_UUID
[ -n "$MUTATION_UUID" ] || { echo "mutation disk identity discovery failed" >&2; exit 1; }
if findmnt -rn -T "$MUTATION_LOOP" >/dev/null 2>&1; then
	echo "mutation disk unexpectedly mounted before format" >&2
	exit 1
fi
mkfs.ext4 -F "$MUTATION_LOOP" >/dev/null
FORMATTED_UUID=$(blkid -s UUID -o value "$MUTATION_LOOP")
[ -n "$FORMATTED_UUID" ] || { echo "format did not create a filesystem UUID" >&2; exit 1; }
wipefs --all --force "$MUTATION_LOOP" >/dev/null
if blkid "$MUTATION_LOOP" >/dev/null 2>&1; then
	echo "erase did not remove filesystem signatures" >&2
	exit 1
fi

mount "$EXT4_LOOP" "$WORK/branch-a"
mount "$MISMATCH_LOOP" "$WORK/branch-b"
mergerfs -o category.create=mfs,use_ino "$WORK/branch-a:$WORK/branch-b" "$WORK/pool"
printf '%s\n' 'mergerfs integration smoke test' >"$WORK/pool/created.txt"
find "$WORK/branch-a" "$WORK/branch-b" -name created.txt -type f -print -quit | grep -F created.txt >/dev/null
umount "$WORK/pool"

SNAPRAID_CONFIG="$WORK/snapraid.conf"
cat >"$SNAPRAID_CONFIG" <<EOF
parity $WORK/branch-a/snapraid.parity
content $WORK/branch-a/snapraid.content
data d1 $WORK/branch-a
data d2 $WORK/branch-b
EOF
snapraid -c "$SNAPRAID_CONFIG" status >/dev/null
snapraid -c "$SNAPRAID_CONFIG" sync >/dev/null
[ -s "$WORK/branch-a/snapraid.parity" ] || { echo "SnapRAID sync did not create parity data" >&2; exit 1; }
snapraid -c "$SNAPRAID_CONFIG" scrub -p 100 >/dev/null
if snapraid -c "$WORK/missing-snapraid.conf" scrub -p 10 >/dev/null 2>&1; then
	echo "SnapRAID scrub unexpectedly succeeded with a missing configuration" >&2
	exit 1
fi

echo "loopback storage smoke test passed (ext4/xfs identity, read-only import, format/erase, mergerfs pool, SnapRAID sync/scrub/failure, mismatch rejected)"
