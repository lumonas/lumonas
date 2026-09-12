#!/bin/sh
set -eu

ASSERT_MODE="${LUMONAS_RECOVERY_ASSERT:-false}"
if [ "$(id -u)" -ne 0 ]; then
	if [ "$ASSERT_MODE" = "true" ]; then
		echo "root is required for QEMU recovery media assertions" >&2
		exit 1
	fi
	echo "QEMU recovery smoke test skipped: root is required" >&2
	exit 0
fi

ISO="${LUMONAS_ISO:-}"
[ -f "$ISO" ] || { echo "LUMONAS_ISO must point to the offline ISO" >&2; exit 1; }
for command in go qemu-img qemu-system-x86_64 mkfs.ext4 mount umount curl; do
	command -v "$command" >/dev/null 2>&1 || { echo "$command is required" >&2; exit 1; }
done

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-recovery-qemu.XXXXXX")"
RECOVERY_IMAGE="$WORK/recovery-media.raw"
TARGET_IMAGE="$WORK/replacement.qcow2"
TARGET_RAW="$WORK/replacement.raw"
RECOVERY_MOUNT="$WORK/recovery-mount"
TARGET_MOUNT="$WORK/target-mount"
LOG="${LUMONAS_RECOVERY_LOG:-$WORK/qemu-recovery.log}"
FIXTURE="${LUMONAS_RECOVERY_FIXTURE:-}"
QEMU_PID=""
cleanup() {
	set +e
	if [ -n "$QEMU_PID" ]; then
		kill "$QEMU_PID" 2>/dev/null || true
		wait "$QEMU_PID" 2>/dev/null || true
	fi
	umount "$TARGET_MOUNT" 2>/dev/null || true
	umount "$RECOVERY_MOUNT" 2>/dev/null || true
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

mkdir -p "$RECOVERY_MOUNT" "$TARGET_MOUNT"
if [ -n "$FIXTURE" ]; then
	"$FIXTURE" --bundle "$WORK/latest.mrb" --key "$WORK/recovery.key"
else
	go run ./cmd/lumonas-recovery-fixture --bundle "$WORK/latest.mrb" --key "$WORK/recovery.key"
fi
truncate -s 128M "$RECOVERY_IMAGE"
mkfs.ext4 -F -L LUMONAS-RECOVERY "$RECOVERY_IMAGE" >/dev/null
mount -o loop "$RECOVERY_IMAGE" "$RECOVERY_MOUNT"
cp "$WORK/latest.mrb" "$RECOVERY_MOUNT/latest.mrb"
cp "$WORK/recovery.key" "$RECOVERY_MOUNT/recovery.key"
touch "$RECOVERY_MOUNT/.lumonas-recovery-test"
umount "$RECOVERY_MOUNT"
qemu-img create -f qcow2 "$TARGET_IMAGE" 2G >/dev/null

qemu-system-x86_64 \
	-machine q35,accel=tcg \
	-m 2048 \
	-smp 2 \
	-cdrom "$ISO" \
	-drive "file=$TARGET_IMAGE,if=virtio,format=qcow2,serial=LUMONAS-REPLACEMENT" \
	-drive "file=$RECOVERY_IMAGE,if=virtio,format=raw,serial=LUMONAS-RECOVERY" \
	-netdev user,id=n1,hostfwd=tcp::18082-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-boot d \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$LOG" 2>&1 &
QEMU_PID=$!

API_READY=false
for attempt in $(seq 1 120); do
	if curl -fsS http://127.0.0.1:18082/healthz >/dev/null 2>&1 && \
		curl -fsS http://127.0.0.1:18082/readyz >/dev/null 2>&1 && \
		curl -fsS http://127.0.0.1:18082/api/v1/server >/dev/null 2>&1; then
		API_READY=true
	fi
	if ! kill -0 "$QEMU_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
[ "$API_READY" = true ] || { echo "recovery ISO API never became ready" >&2; cat "$LOG" >&2 || true; exit 1; }

wait "$QEMU_PID" || true
QEMU_PID=""
qemu-img convert -O raw "$TARGET_IMAGE" "$TARGET_RAW" >/dev/null
mount -o loop,ro "$TARGET_RAW" "$TARGET_MOUNT"
grep -Fx 'recovery-applied' "$TARGET_MOUNT/recovery-success" >/dev/null
grep -F 'fixture-nas' "$TARGET_MOUNT/var/lib/lumonas/recovery/restored/desired-state.json" >/dev/null
grep -F 'example/media:latest' "$TARGET_MOUNT/srv/lumonas/docker/stacks/media/compose.yaml" >/dev/null
grep -F 'fixture-encrypted-secret' "$TARGET_MOUNT/var/lib/lumonas/secrets/recovered-secrets.bin" >/dev/null
grep -F 'share-media' "$TARGET_MOUNT/var/lib/lumonas/shares.json" >/dev/null
grep -F 'operator' "$TARGET_MOUNT/etc/lumonas/recovery/users.json" >/dev/null
grep -F 'media' "$TARGET_MOUNT/etc/lumonas/recovery/users.json" >/dev/null
grep -F '"generation":2' "$TARGET_MOUNT/etc/lumonas/recovery/config-generation.json" >/dev/null
grep -F 'data d1 /srv/disks/serial_DATA1' "$TARGET_MOUNT/etc/lumonas/snapraid.conf" >/dev/null
grep -F 'fuse.mergerfs' "$TARGET_MOUNT/etc/lumonas/recovery/mergerfs.conf" >/dev/null
grep -F 'serial:PARITY' "$TARGET_MOUNT/etc/lumonas/recovery/disk-identities.json" >/dev/null
grep -F '"level":"write"' "$TARGET_MOUNT/etc/lumonas/acl/share-media.json" >/dev/null
grep -a -F 'operator' "$TARGET_MOUNT/var/lib/lumonas/lumonas.db" >/dev/null
grep -a -F 'share-media' "$TARGET_MOUNT/var/lib/lumonas/lumonas.db" >/dev/null
grep -F '"databaseRestored":true' "$TARGET_MOUNT/recovery-result.json" >/dev/null
grep -F '"secretsRestored":true' "$TARGET_MOUNT/recovery-result.json" >/dev/null
grep -F 'operator' "$TARGET_MOUNT/restored-principals.json" >/dev/null
grep -F 'share-media' "$TARGET_MOUNT/restored-shares.json" >/dev/null
echo "QEMU recovery smoke test passed (offline ISO, blank replacement disk, users/shares/Compose/SnapRAID restored, API ready)"
