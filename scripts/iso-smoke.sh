#!/bin/sh
set -eu

ASSERT_MODE="${LUMONAS_ISO_ASSERT:-false}"
if ! command -v qemu-system-x86_64 >/dev/null 2>&1; then
	if [ "$ASSERT_MODE" = "true" ]; then
		echo "qemu-system-x86_64 is required in assertion mode" >&2
		exit 1
	fi
	echo "ISO smoke test skipped: qemu-system-x86_64 is unavailable" >&2
	exit 0
fi
if [ -z "${LUMONAS_ISO:-}" ] || [ ! -f "$LUMONAS_ISO" ]; then
	if [ "$ASSERT_MODE" = "true" ]; then
		echo "LUMONAS_ISO must point to a bootable ISO in assertion mode" >&2
		exit 1
	fi
	echo "Set LUMONAS_ISO to the generated LumoNAS ISO" >&2
	exit 0
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-iso.XXXXXX")"
LOG="${LUMONAS_ISO_LOG:-$WORK/iso-smoke.log}"
DISK="$WORK/blank-system.qcow2"
QEMU_PID=""
cleanup() {
	set +e
	if [ -n "$QEMU_PID" ]; then
		kill "$QEMU_PID" 2>/dev/null || true
		wait "$QEMU_PID" 2>/dev/null || true
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

if ! command -v qemu-img >/dev/null 2>&1; then
	echo "qemu-img is required for ISO smoke testing" >&2
	exit 1
fi
qemu-img create -f qcow2 "$DISK" 2G >/dev/null

qemu-system-x86_64 \
	-machine q35,accel=tcg \
	-m 2048 \
	-smp 2 \
	-cdrom "$LUMONAS_ISO" \
	-drive "file=$DISK,if=virtio,format=qcow2,serial=LUMONAS-REPLACEMENT" \
	-netdev user,id=n1,hostfwd=tcp::18081-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-boot d \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$LOG" 2>&1 &
QEMU_PID=$!

for attempt in $(seq 1 90); do
	if curl -kfsS https://127.0.0.1:18081/healthz >/dev/null 2>&1 && \
		curl -kfsS https://127.0.0.1:18081/readyz >"$LOG.ready" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18081/api/v1/server >/dev/null 2>&1; then
		grep -F '"privilegedBroker":true' "$LOG.ready" >/dev/null
		echo "LumoNAS ISO smoke test passed (blank replacement disk booted)"
		exit 0
	fi
	if ! kill -0 "$QEMU_PID" 2>/dev/null; then
		echo "ISO guest exited before readiness; log: $LOG" >&2
		cat "$LOG" >&2 || true
		exit 1
	fi
	sleep 2
done

echo "ISO guest did not become ready; log: $LOG" >&2
cat "$LOG" >&2 || true
exit 1
