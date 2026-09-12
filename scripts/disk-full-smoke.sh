#!/bin/sh
set -eu

ASSERT_MODE="${LUMONAS_DISK_FULL_ASSERT:-false}"
if [ "$(id -u)" -ne 0 ]; then
	if [ "$ASSERT_MODE" = "true" ]; then
		echo "root is required for disk-full assertions" >&2
		exit 1
	fi
	echo "disk-full smoke test skipped: root is required" >&2
	exit 0
fi

for command in go losetup mount umount mkfs.ext4 truncate df dd mktemp; do
	command -v "$command" >/dev/null 2>&1 || {
		echo "$command is required for disk-full assertions" >&2
		exit 1
	}
done

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-disk-full.XXXXXX")"
LOOP=""
MOUNT_PATH="$WORK/mount"
cleanup() {
	set +e
	umount "$MOUNT_PATH" 2>/dev/null || true
	if [ -n "$LOOP" ]; then
		losetup -d "$LOOP" 2>/dev/null || true
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

mkdir -p "$MOUNT_PATH"
truncate -s 64M "$WORK/disk.img"
LOOP="$(losetup --find --show "$WORK/disk.img")"
mkfs.ext4 -F "$LOOP" >/dev/null
mount "$LOOP" "$MOUNT_PATH"

# Fill disposable media until ext4 reports ENOSPC. The failed final write is
# expected; the test validates the resulting production statfs classification.
dd if=/dev/zero of="$MOUNT_PATH/fill.bin" bs=1M status=none || true
used_percent="$(df -P "$MOUNT_PATH" | awk 'NR == 2 { gsub(/%/, "", $5); print $5 }')"
[ -n "$used_percent" ] && [ "$used_percent" -ge 95 ] || {
	echo "disposable filesystem did not reach the critical threshold: ${used_percent:-unknown}%" >&2
	exit 1
}

LUMONAS_TEST_FILESYSTEM_PATH="$MOUNT_PATH" \
	GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" \
	GOPATH="${GOPATH:-/tmp/lumonas-gopath}" \
	go test "$ROOT/internal/collector" -run '^TestFilesystemUsageReportsCriticalConfiguredPath$' -count=1

echo "disk-full smoke passed (real ext4 filesystem reached ${used_percent}% and collector reported critical)"
