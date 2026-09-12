#!/bin/sh
set -eu

ASSERT_MODE="${LUMONAS_QEMU_ASSERT:-false}"

if ! command -v qemu-system-x86_64 >/dev/null 2>&1; then
  if [ "$ASSERT_MODE" = "true" ]; then
    echo "qemu-system-x86_64 is required in assertion mode" >&2
    exit 1
  fi
  echo "qemu-system-x86_64 is not installed; QEMU smoke test skipped" >&2
  exit 0
fi

if [ -z "${LUMONAS_QEMU_IMAGE:-}" ]; then
  if [ "$ASSERT_MODE" = "true" ]; then
    echo "LUMONAS_QEMU_IMAGE is required in assertion mode" >&2
    exit 1
  fi
  echo "Set LUMONAS_QEMU_IMAGE to a Debian 13 image built by scripts/qemu-build-image.sh" >&2
  exit 0
fi

if ! command -v qemu-img >/dev/null 2>&1; then
  echo "qemu-img is required when LUMONAS_QEMU_IMAGE is set" >&2
  exit 1
fi

if [ "$ASSERT_MODE" = "true" ] && ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required for QEMU disk identity assertions" >&2
  exit 1
fi

DATA_DIR="${LUMONAS_QEMU_DATA_DIR:-/tmp/lumonas-qemu-disks}"
mkdir -p "$DATA_DIR"
for disk in data1 data2 data3 parity; do
  image="$DATA_DIR/$disk.qcow2"
  if [ ! -f "$image" ]; then qemu-img create -f qcow2 "$image" 1G >/dev/null; fi
done

IMAGE_FORMAT="${LUMONAS_QEMU_IMAGE_FORMAT:-raw}"
run_qemu() {
  data_a=data1
  data_b=data2
  data_c=data3
  parity_disk=parity
  if [ "${LUMONAS_QEMU_REORDER:-false}" = "true" ]; then
    data_a=data3
    data_b=data1
    data_c=data2
    parity_disk=parity
  fi
qemu-system-x86_64 \
  -machine q35,accel=tcg \
  -m 2048 \
  -smp 2 \
  -drive "file=$LUMONAS_QEMU_IMAGE,if=virtio,format=$IMAGE_FORMAT,serial=LUMONAS-SYSTEM" \
  -drive "file=$DATA_DIR/$data_a.qcow2,if=virtio,format=qcow2,serial=LUMONAS-$(printf '%s' "$data_a" | tr '[:lower:]' '[:upper:]')" \
  -drive "file=$DATA_DIR/$data_b.qcow2,if=virtio,format=qcow2,serial=LUMONAS-$(printf '%s' "$data_b" | tr '[:lower:]' '[:upper:]')" \
  -drive "file=$DATA_DIR/$data_c.qcow2,if=virtio,format=qcow2,serial=LUMONAS-$(printf '%s' "$data_c" | tr '[:lower:]' '[:upper:]')" \
  -drive "file=$DATA_DIR/$parity_disk.qcow2,if=virtio,format=qcow2,serial=LUMONAS-PARITY" \
  -netdev user,id=n1,hostfwd=tcp::18080-:8081 \
  -device virtio-net-pci,netdev=n1 \
  -nographic \
  -serial mon:stdio \
  -no-reboot
}

snapshot_disk_identities() {
  input=$1
  output=$2
  python3 - "$input" "$output" <<'PY'
import json
import sys

input_path, output_path = sys.argv[1:]
with open(input_path, encoding="utf-8") as handle:
    disks = json.load(handle)

fields = ("id", "serial", "wwn", "gptDiskGuid", "partitionUuid", "filesystemUuid", "sizeBytes")
rows = []
for disk in disks:
    identity = {field: disk.get(field, "") for field in fields}
    rows.append((json.dumps(identity, sort_keys=True, separators=(",", ":")), disk.get("currentPath", "")))

with open(output_path, "w", encoding="utf-8") as handle:
    for identity, current_path in sorted(rows):
        handle.write(f"{identity}\t{current_path}\n")
PY
}

if [ "$ASSERT_MODE" != "true" ]; then
  run_qemu
  exit $?
fi

LOG="${LUMONAS_QEMU_LOG:-/tmp/lumonas-qemu-smoke.log}"
INDEX_LOG="$LOG.index"
run_qemu >"$LOG" 2>&1 &
QEMU_PID=$!
cleanup() { kill "$QEMU_PID" 2>/dev/null || true; wait "$QEMU_PID" 2>/dev/null || true; }
trap cleanup EXIT

for attempt in $(seq 1 60); do
  if curl -kfsS https://127.0.0.1:18080/healthz >/dev/null 2>&1 && \
     curl -kfsS https://127.0.0.1:18080/readyz >/dev/null 2>&1 && \
     curl -kfsS https://127.0.0.1:18080/ >"$INDEX_LOG" 2>/dev/null && \
     curl -kfsS https://127.0.0.1:18080/api/v1/server >/dev/null 2>&1 && \
     curl -kfsS https://127.0.0.1:18080/api/v1/disks >"$LOG.disks" 2>/dev/null && \
     curl -kfsS https://127.0.0.1:18080/api/v1/system/metrics >/dev/null 2>&1 && \
     curl -kfsS https://127.0.0.1:18080/api/v1/jobs >/dev/null 2>&1 && \
     curl -kfsS https://127.0.0.1:18080/api/v1/settings >"$LOG.settings" 2>/dev/null && \
     curl -kfsS https://127.0.0.1:18080/api/v1/onboarding/state >/dev/null 2>&1 && \
     curl -kfsS https://127.0.0.1:18080/api/v1/services >"$LOG.services" 2>/dev/null; then
    disk_count=$(grep -o '"id"' "$LOG.disks" | wc -l | tr -d ' ')
    if [ "$disk_count" -ge 5 ] && \
       grep -F 'serial:LUMONAS-DATA1' "$LOG.disks" >/dev/null 2>&1 && \
       grep -F '"runtime"' "$LOG.settings" >/dev/null 2>&1 && \
       grep -F '"tmpfs"' "$LOG.settings" >/dev/null 2>&1 && \
       grep -F '<title>LumoNAS</title>' "$INDEX_LOG" >/dev/null 2>&1 && \
       grep -F '<div id="root"></div>' "$INDEX_LOG" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-privd.service","name":"lumonas-privd.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-runtime.service","name":"lumonas-runtime.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-privd-storage.service","name":"lumonas-privd-storage.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-privd-network.service","name":"lumonas-privd-network.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-privd-power.service","name":"lumonas-privd-power.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-privd-general.service","name":"lumonas-privd-general.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonasd.service","name":"lumonasd.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-web.service","name":"lumonas-web.service","active":true,"state":"running","user":"lumonas"' "$LOG.services" >/dev/null 2>&1; then
      EVENTS_LOG="$LOG.events"
      curl -kfsS --max-time 5 -N https://127.0.0.1:18080/api/v1/events/stream >"$EVENTS_LOG" 2>/dev/null || true
      RECOVERY_KEY_LOG="$LOG.recovery-key"
      RECOVERY_EXPORT_LOG="$LOG.recovery-export"
      RECOVERY_STATUS_LOG="$LOG.recovery-status"
      RECOVERY_PLAN_LOG="$LOG.recovery-plan"
      RECOVERY_STAGE_LOG="$LOG.recovery-stage"
      if curl -kfsS -X POST https://127.0.0.1:18080/api/v1/recovery/key >"$RECOVERY_KEY_LOG" 2>/dev/null && \
         curl -kfsS -X POST https://127.0.0.1:18080/api/v1/recovery/export >"$RECOVERY_EXPORT_LOG" 2>/dev/null && \
         curl -kfsS https://127.0.0.1:18080/api/v1/recovery/status >"$RECOVERY_STATUS_LOG" 2>/dev/null && \
         curl -kfsS https://127.0.0.1:18080/api/v1/recovery/plan >"$RECOVERY_PLAN_LOG" 2>/dev/null && \
         curl -kfsS -X POST -H 'Content-Type: application/json' -d '{"confirmed":true,"reauthenticated":true}' https://127.0.0.1:18080/api/v1/recovery/restore/stage >"$RECOVERY_STAGE_LOG" 2>/dev/null && \
         grep -F 'retry: 3000' "$EVENTS_LOG" >/dev/null 2>&1 && \
         grep -F 'system.metrics' "$EVENTS_LOG" >/dev/null 2>&1 && \
         grep -F '"verified":true' "$RECOVERY_EXPORT_LOG" >/dev/null 2>&1 && \
         grep -F '"verified":true' "$RECOVERY_STATUS_LOG" >/dev/null 2>&1 && \
         grep -F '"verified":true' "$RECOVERY_PLAN_LOG" >/dev/null 2>&1 && \
         grep -F '"verified":true' "$RECOVERY_STAGE_LOG" >/dev/null 2>&1; then
        snapshot_disk_identities "$LOG.disks" "$LOG.identities.initial"
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
        LUMONAS_QEMU_REORDER=true run_qemu >"$LOG.reordered" 2>&1 &
        QEMU_PID=$!
        for reorder_attempt in $(seq 1 60); do
          if curl -kfsS https://127.0.0.1:18080/healthz >/dev/null 2>&1 && \
             curl -kfsS https://127.0.0.1:18080/api/v1/disks >"$LOG.disks.reordered" 2>/dev/null; then
            snapshot_disk_identities "$LOG.disks.reordered" "$LOG.identities.reordered"
            cut -f1 "$LOG.identities.initial" >"$LOG.ids.initial"
            cut -f1 "$LOG.identities.reordered" >"$LOG.ids.reordered"
            cut -f2 "$LOG.identities.initial" >"$LOG.paths.initial"
            cut -f2 "$LOG.identities.reordered" >"$LOG.paths.reordered"
            if cmp -s "$LOG.ids.initial" "$LOG.ids.reordered"; then
              if cmp -s "$LOG.paths.initial" "$LOG.paths.reordered"; then
                echo "QEMU appliance device reorder did not change any transient device path" >&2
                exit 1
              fi
              echo "QEMU appliance smoke test passed (disks=$disk_count, recovery=verified, stable identities=verified, reorder=verified)"
              exit 0
            fi
          fi
          if ! kill -0 "$QEMU_PID" 2>/dev/null; then
            echo "QEMU exited during device reorder boot; log: $LOG.reordered" >&2
            cat "$LOG.reordered" >&2 || true
            exit 1
          fi
          sleep 2
        done
        echo "QEMU appliance device reorder verification failed" >&2
        diff -u "$LOG.ids.initial" "$LOG.ids.reordered" >&2 || true
        exit 1
      fi
    fi
  fi
  if ! kill -0 "$QEMU_PID" 2>/dev/null; then
    echo "QEMU exited before readiness; log: $LOG" >&2
    cat "$LOG" >&2 || true
    exit 1
  fi
  sleep 2
done
echo "QEMU appliance did not become ready; log: $LOG" >&2
cat "$LOG" >&2 || true
exit 1
