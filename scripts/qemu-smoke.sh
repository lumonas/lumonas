#!/bin/sh
set -eu

ASSERT_MODE="${MYNAS_QEMU_ASSERT:-false}"

if ! command -v qemu-system-x86_64 >/dev/null 2>&1; then
  if [ "$ASSERT_MODE" = "true" ]; then
    echo "qemu-system-x86_64 is required in assertion mode" >&2
    exit 1
  fi
  echo "qemu-system-x86_64 is not installed; QEMU smoke test skipped" >&2
  exit 0
fi

if [ -z "${MYNAS_QEMU_IMAGE:-}" ]; then
  if [ "$ASSERT_MODE" = "true" ]; then
    echo "MYNAS_QEMU_IMAGE is required in assertion mode" >&2
    exit 1
  fi
  echo "Set MYNAS_QEMU_IMAGE to a Debian 13 image built by scripts/qemu-build-image.sh" >&2
  exit 0
fi

if ! command -v qemu-img >/dev/null 2>&1; then
  echo "qemu-img is required when MYNAS_QEMU_IMAGE is set" >&2
  exit 1
fi

DATA_DIR="${MYNAS_QEMU_DATA_DIR:-/tmp/lumonas-qemu-disks}"
mkdir -p "$DATA_DIR"
for disk in data1 data2 data3 parity; do
  image="$DATA_DIR/$disk.qcow2"
  if [ ! -f "$image" ]; then qemu-img create -f qcow2 "$image" 1G >/dev/null; fi
done

IMAGE_FORMAT="${MYNAS_QEMU_IMAGE_FORMAT:-raw}"
run_qemu() {
qemu-system-x86_64 \
  -machine q35,accel=tcg \
  -m 2048 \
  -smp 2 \
  -drive "file=$MYNAS_QEMU_IMAGE,if=virtio,format=$IMAGE_FORMAT" \
  -drive "file=$DATA_DIR/data1.qcow2,if=virtio,format=qcow2" \
  -drive "file=$DATA_DIR/data2.qcow2,if=virtio,format=qcow2" \
  -drive "file=$DATA_DIR/data3.qcow2,if=virtio,format=qcow2" \
  -drive "file=$DATA_DIR/parity.qcow2,if=virtio,format=qcow2" \
  -netdev user,id=n1,hostfwd=tcp::18080-:8081 \
  -device virtio-net-pci,netdev=n1 \
  -nographic \
  -serial mon:stdio \
  -no-reboot
}

if [ "$ASSERT_MODE" != "true" ]; then
  run_qemu
  exit $?
fi

LOG="${MYNAS_QEMU_LOG:-/tmp/lumonas-qemu-smoke.log}"
run_qemu >"$LOG" 2>&1 &
QEMU_PID=$!
cleanup() { kill "$QEMU_PID" 2>/dev/null || true; wait "$QEMU_PID" 2>/dev/null || true; }
trap cleanup EXIT

for attempt in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:18080/healthz >/dev/null 2>&1 && \
     curl -fsS http://127.0.0.1:18080/readyz >/dev/null 2>&1 && \
     curl -fsS http://127.0.0.1:18080/api/v1/server >/dev/null 2>&1 && \
     curl -fsS http://127.0.0.1:18080/api/v1/disks >"$LOG.disks" 2>/dev/null && \
     curl -fsS http://127.0.0.1:18080/api/v1/system/metrics >/dev/null 2>&1 && \
     curl -fsS http://127.0.0.1:18080/api/v1/jobs >/dev/null 2>&1; then
    disk_count=$(grep -o '"id"' "$LOG.disks" | wc -l | tr -d ' ')
    if [ "$disk_count" -ge 5 ]; then
      echo "QEMU appliance smoke test passed (disks=$disk_count)"
      exit 0
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
