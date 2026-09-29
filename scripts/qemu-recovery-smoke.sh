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
for command in go qemu-img qemu-system-x86_64 mkfs.ext4 mount umount curl python3 losetup partx blkid; do
	command -v "$command" >/dev/null 2>&1 || { echo "$command is required" >&2; exit 1; }
done

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
# shellcheck source=scripts/loop-partition-lib.sh
. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/loop-partition-lib.sh"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-recovery-qemu.XXXXXX")"
RECOVERY_IMAGE="$WORK/recovery-media.raw"
TARGET_IMAGE="$WORK/replacement.qcow2"
TARGET_RAW="$WORK/replacement.raw"
RECOVERY_MOUNT="$WORK/recovery-mount"
TARGET_MOUNT="$WORK/target-mount"
LOG="${LUMONAS_RECOVERY_LOG:-$WORK/qemu-recovery.log}"
DEBUG_DIR="${LUMONAS_RECOVERY_DEBUG_DIR:-}"
FIXTURE="${LUMONAS_RECOVERY_FIXTURE:-}"
SOURCE_IMAGE="${LUMONAS_RECOVERY_SOURCE_IMAGE:-}"
SOURCE_MODE=false
QEMU_PID=""
cleanup() {
	status=$?
	set +e
	if [ -n "$QEMU_PID" ]; then
		kill "$QEMU_PID" 2>/dev/null || true
		wait "$QEMU_PID" 2>/dev/null || true
	fi
	unmount_all_image_roots
	if [ "$status" -ne 0 ] && [ -n "$DEBUG_DIR" ]; then
		mkdir -p "$DEBUG_DIR"
		cp -a "$WORK"/. "$DEBUG_DIR"/ 2>/dev/null || true
	fi
	rm -rf "$WORK"
	return "$status"
}
trap cleanup EXIT INT TERM

mkdir -p "$RECOVERY_MOUNT" "$TARGET_MOUNT"
if [ -n "$SOURCE_IMAGE" ]; then
	SOURCE_MODE=true
	bash "$ROOT/scripts/qemu-live-recovery-source.sh" "$SOURCE_IMAGE" "$WORK"
	elif [ -n "$FIXTURE" ]; then
	"$FIXTURE" --bundle "$WORK/latest.mrb" --key "$WORK/recovery.key"
else
	(cd "$ROOT" && go run ./cmd/lumonas-recovery-fixture --bundle "$WORK/latest.mrb" --key "$WORK/recovery.key")
fi
PLAN_PATH="$WORK/recovery-plan.json"
if [ -n "${LUMONAS_RECOVER_BIN:-}" ]; then
	"$LUMONAS_RECOVER_BIN" --bundle "$WORK/latest.mrb" --key-file "$WORK/recovery.key" >"$PLAN_PATH"
else
	(cd "$ROOT" && go run ./cmd/lumonas-recover --bundle "$WORK/latest.mrb" --key-file "$WORK/recovery.key" >"$PLAN_PATH")
fi
grep -F '"verified":true' "$PLAN_PATH" >/dev/null
grep -F '"databaseValid":true' "$PLAN_PATH" >/dev/null
grep -F '"desiredStateValid":true' "$PLAN_PATH" >/dev/null
grep -F '"composeValid":true' "$PLAN_PATH" >/dev/null
python3 "$ROOT/scripts/validate-api-response.py" recovery-plan "$PLAN_PATH"
truncate -s 128M "$RECOVERY_IMAGE"
mkfs.ext4 -F -L LUMONAS-RECOVERY "$RECOVERY_IMAGE" >/dev/null
mount_image_root "$RECOVERY_IMAGE" "$RECOVERY_MOUNT" >/dev/null
cp "$WORK/latest.mrb" "$RECOVERY_MOUNT/latest.mrb"
cp "$WORK/recovery.key" "$RECOVERY_MOUNT/recovery.key"
touch "$RECOVERY_MOUNT/.lumonas-recovery-test"
[ "$SOURCE_MODE" = true ] && touch "$RECOVERY_MOUNT/.lumonas-recovery-source"
umount "$RECOVERY_MOUNT"
qemu-img create -f qcow2 "$TARGET_IMAGE" 2G >/dev/null

qemu-system-x86_64 \
	-machine q35,accel=tcg \
	-m 2048 \
	-smp 2 \
	-cdrom "$ISO" \
	-drive "file=$TARGET_IMAGE,if=none,id=replacement,format=qcow2" \
	-device "virtio-blk-pci,drive=replacement,serial=LUMONAS-REPLACEMENT" \
	-drive "file=$RECOVERY_IMAGE,if=none,id=recovery,format=raw" \
	-device "virtio-blk-pci,drive=recovery,serial=LUMONAS-RECOVERY" \
	-netdev user,id=n1,restrict=on,hostfwd=tcp::18082-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-boot d \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$LOG" 2>&1 &
QEMU_PID=$!

API_READY=false
for attempt in $(seq 1 120); do
	if curl -kfsS https://127.0.0.1:18082/healthz >/dev/null 2>&1 && \
		curl -kfsS https://127.0.0.1:18082/readyz >"$WORK/ready.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18082/api/v1/server >"$WORK/server.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18082/api/v1/disks >"$WORK/disks.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18082/api/v1/system/metrics >"$WORK/metrics.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18082/api/v1/jobs >"$WORK/jobs.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18082/api/v1/services >"$WORK/services.json" 2>/dev/null; then
		API_READY=true
	fi
	if ! kill -0 "$QEMU_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
[ "$API_READY" = true ] || { echo "recovery ISO API never became ready" >&2; cat "$LOG" >&2 || true; exit 1; }
grep -F '"privilegedBroker":true' "$WORK/ready.json" >/dev/null
python3 "$ROOT/scripts/validate-api-response.py" readiness "$WORK/ready.json"
python3 "$ROOT/scripts/validate-api-response.py" server "$WORK/server.json"
python3 "$ROOT/scripts/validate-api-response.py" disks "$WORK/disks.json"
python3 "$ROOT/scripts/validate-api-response.py" metrics "$WORK/metrics.json"
python3 "$ROOT/scripts/validate-api-response.py" jobs "$WORK/jobs.json"
python3 "$ROOT/scripts/validate-api-response.py" services "$WORK/services.json"

guest_exited=false
for attempt in $(seq 1 90); do
	if ! kill -0 "$QEMU_PID" 2>/dev/null; then
		guest_exited=true
		break
	fi
	sleep 2
done
[ "$guest_exited" = true ] || {
	echo "recovery guest did not power off before timeout" >&2
	cat "$LOG" >&2 || true
	exit 1
}
wait "$QEMU_PID"
QEMU_PID=""
qemu-img convert -O raw "$TARGET_IMAGE" "$TARGET_RAW" >/dev/null
mount_image_root "$TARGET_RAW" "$TARGET_MOUNT" -o ro >/dev/null
grep -Fx 'recovery-applied' "$TARGET_MOUNT/recovery-success" >/dev/null
if [ "$SOURCE_MODE" = true ]; then
	grep -F 'configGeneration' "$TARGET_MOUNT/var/lib/lumonas/recovery/restored/desired-state.json" >/dev/null
	grep -F 'example/media:latest' "$TARGET_MOUNT/srv/lumonas/docker/stacks/media/compose.yaml" >/dev/null
	grep -F 'mode: live-source' "$TARGET_MOUNT/srv/lumonas/docker/appdata/media/config.yaml" >/dev/null
	grep -F 'share-media' "$TARGET_MOUNT/var/lib/lumonas/shares.json" >/dev/null
	grep -a -F 'operator' "$TARGET_MOUNT/var/lib/lumonas/lumonas.db" >/dev/null
	grep -F 'live-source-recovery-secret' "$TARGET_MOUNT/var/lib/lumonas/secrets/recovered-secrets.bin" >/dev/null
	grep -F 'data d1 /srv/disks/serial_LUMONAS-DATA1' "$TARGET_MOUNT/etc/lumonas/snapraid.conf" >/dev/null
	grep -F 'parity /srv/disks/serial_LUMONAS-PARITY' "$TARGET_MOUNT/etc/lumonas/snapraid.conf" >/dev/null
	grep -F 'fuse.mergerfs' "$TARGET_MOUNT/restored-mounts.json" >/dev/null
	grep -F 'serial_LUMONAS-DATA1' "$TARGET_MOUNT/restored-mounts.json" >/dev/null
	grep -F '"id":"lan"' "$TARGET_MOUNT/restored-network.json" >/dev/null
	grep -F '"interface":"eth0"' "$TARGET_MOUNT/restored-network.json" >/dev/null
	test -s "$TARGET_MOUNT/etc/lumonas/recovery/network-connections.json"
	grep -F '"databaseRestored":true' "$TARGET_MOUNT/recovery-result.json" >/dev/null
	grep -F '"secretsRestored":true' "$TARGET_MOUNT/recovery-result.json" >/dev/null
	echo "QEMU recovery smoke test passed (live source appliance, real filesystems, mergerfs pool, network state, appdata, verified bundle, offline ISO, blank replacement disk restored)"
else
	grep -F 'fixture-nas' "$TARGET_MOUNT/var/lib/lumonas/recovery/restored/desired-state.json" >/dev/null
	grep -F 'example/media:latest' "$TARGET_MOUNT/srv/lumonas/docker/stacks/media/compose.yaml" >/dev/null
	grep -F 'mode: fixture' "$TARGET_MOUNT/srv/lumonas/docker/appdata/media/config.yaml" >/dev/null
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
	grep -F 'fuse.mergerfs' "$TARGET_MOUNT/restored-mounts.json" >/dev/null
	grep -F 'serial_DATA1' "$TARGET_MOUNT/restored-mounts.json" >/dev/null
	grep -F '"id":"lan"' "$TARGET_MOUNT/restored-network.json" >/dev/null
	grep -F '"interface":"eth0"' "$TARGET_MOUNT/restored-network.json" >/dev/null
	echo "QEMU recovery smoke test passed (bundle checksums verified, offline ISO, blank replacement disk, users/shares/network/mounts/Compose/appdata/SnapRAID restored, API ready)"
fi
unmount_image_root "$TARGET_MOUNT"
QEMU_PID=""
qemu-system-x86_64 \
	-machine q35,accel=tcg \
	-m 2048 \
	-smp 2 \
	-drive "file=$TARGET_IMAGE,if=none,id=recovered,format=qcow2" \
	-device "virtio-blk-pci,drive=recovered,serial=LUMONAS-RECOVERED" \
	-netdev user,id=n1,restrict=on,hostfwd=tcp::18084-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$WORK/recovered-boot.log" 2>&1 &
QEMU_PID=$!
recovered_ready=false
for attempt in $(seq 1 120); do
	if curl -kfsS https://127.0.0.1:18084/healthz >/dev/null 2>&1 && \
		curl -kfsS https://127.0.0.1:18084/readyz >"$WORK/recovered-ready.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/server >"$WORK/recovered-server.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/disks >"$WORK/recovered-disks.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/system/metrics >"$WORK/recovered-metrics.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/jobs >"$WORK/recovered-jobs.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/health/components >"$WORK/recovered-health.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/docker/summary >"$WORK/recovered-docker-summary.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/docker/containers >"$WORK/recovered-docker-containers.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/docker/images >"$WORK/recovered-docker-images.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/docker/volumes >"$WORK/recovered-docker-volumes.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18084/api/v1/services >"$WORK/recovered-services.json" 2>/dev/null; then
		recovered_ready=true
		break
	fi
	if ! kill -0 "$QEMU_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
[ "$recovered_ready" = true ] || {
	echo "recovered replacement disk did not boot a healthy API" >&2
	cat "$WORK/recovered-boot.log" >&2 || true
	exit 1
}
grep -F '"privilegedBroker":true' "$WORK/recovered-ready.json" >/dev/null
grep -F '"nasUuid"' "$WORK/recovered-server.json" >/dev/null
python3 "$ROOT/scripts/validate-api-response.py" readiness "$WORK/recovered-ready.json"
python3 "$ROOT/scripts/validate-api-response.py" server "$WORK/recovered-server.json"
python3 "$ROOT/scripts/validate-api-response.py" disks "$WORK/recovered-disks.json"
python3 "$ROOT/scripts/validate-api-response.py" metrics "$WORK/recovered-metrics.json"
python3 "$ROOT/scripts/validate-api-response.py" jobs "$WORK/recovered-jobs.json"
python3 "$ROOT/scripts/validate-api-response.py" health "$WORK/recovered-health.json"
python3 "$ROOT/scripts/validate-api-response.py" docker-summary "$WORK/recovered-docker-summary.json"
python3 "$ROOT/scripts/validate-api-response.py" docker-containers "$WORK/recovered-docker-containers.json"
python3 "$ROOT/scripts/validate-api-response.py" docker-images "$WORK/recovered-docker-images.json"
python3 "$ROOT/scripts/validate-api-response.py" docker-volumes "$WORK/recovered-docker-volumes.json"
python3 "$ROOT/scripts/validate-api-response.py" services "$WORK/recovered-services.json"
grep -F '"available":true' "$WORK/recovered-docker-summary.json" >/dev/null
grep -F '"id":"lumonasd.service","name":"lumonasd.service","active":true,"state":"running","user":"lumonas"' "$WORK/recovered-services.json" >/dev/null
grep -F '"id":"lumonas-web.service","name":"lumonas-web.service","active":true,"state":"running","user":"lumonas"' "$WORK/recovered-services.json" >/dev/null
grep -F '"id":"lumonas-privd.service","name":"lumonas-privd.service","active":true,"state":"running"' "$WORK/recovered-services.json" >/dev/null
grep -F '"id":"lumonas-privd-storage.service","name":"lumonas-privd-storage.service","active":true,"state":"running"' "$WORK/recovered-services.json" >/dev/null
RECOVERED_EVENTS_LOG="$WORK/recovered-events.sse"
curl -kfsS --max-time 5 -N https://127.0.0.1:18084/api/v1/events >"$RECOVERED_EVENTS_LOG" 2>/dev/null || true
python3 "$ROOT/scripts/validate-sse.py" "$RECOVERED_EVENTS_LOG" system.metrics
echo "LumoNAS recovered replacement disk boot passed"
