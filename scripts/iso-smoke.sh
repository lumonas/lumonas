#!/bin/sh
set -eu

# Every request to the appliance must be bounded. Without an explicit timeout
# curl blocks indefinitely if the appliance accepts the connection but does not
# answer, which turns a failure into a job that hangs until the runner's own
# limit. A refused connection fails immediately, so this only bounds the case
# where something accepts the socket and then goes quiet.
CURL_BOUNDS="${LUMONAS_CURL_BOUNDS:---connect-timeout 3 --max-time 10}"


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
command -v python3 >/dev/null 2>&1 || { echo "python3 is required for ISO API contract validation" >&2; exit 1; }
qemu-img create -f qcow2 "$DISK" 2G >/dev/null

qemu-system-x86_64 \
	-machine q35,accel=tcg \
	-m 2048 \
	-smp 2 \
	-cdrom "$LUMONAS_ISO" \
	-drive "file=$DISK,if=none,id=replacement,format=qcow2" \
		-device "virtio-blk-pci,drive=replacement,serial=LUMONAS-REPLACEMENT" \
	-netdev user,id=n1,restrict=on,hostfwd=tcp::18081-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-boot d \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$LOG" 2>&1 &
QEMU_PID=$!

for attempt in $(seq 1 90); do
	if curl $CURL_BOUNDS -kfsS https://127.0.0.1:18081/healthz >/dev/null 2>&1 && \
		curl $CURL_BOUNDS -kfsS https://127.0.0.1:18081/readyz >"$LOG.ready" 2>/dev/null && \
		curl $CURL_BOUNDS -kfsS https://127.0.0.1:18081/api/v1/server >"$LOG.server" 2>/dev/null && \
		curl $CURL_BOUNDS -kfsS https://127.0.0.1:18081/api/v1/disks >"$LOG.disks" 2>/dev/null && \
		curl $CURL_BOUNDS -kfsS https://127.0.0.1:18081/api/v1/system/metrics >"$LOG.metrics" 2>/dev/null && \
		curl $CURL_BOUNDS -kfsS https://127.0.0.1:18081/api/v1/jobs >"$LOG.jobs" 2>/dev/null && \
		curl $CURL_BOUNDS -kfsS https://127.0.0.1:18081/api/v1/services >"$LOG.services" 2>/dev/null; then
		grep -F '"privilegedBroker":true' "$LOG.ready" >/dev/null
		ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
		python3 "$ROOT/scripts/validate-api-response.py" readiness "$LOG.ready"
		python3 "$ROOT/scripts/validate-api-response.py" server "$LOG.server"
		python3 "$ROOT/scripts/validate-api-response.py" disks "$LOG.disks"
		python3 "$ROOT/scripts/validate-api-response.py" metrics "$LOG.metrics"
		python3 "$ROOT/scripts/validate-api-response.py" jobs "$LOG.jobs"
		python3 "$ROOT/scripts/validate-api-response.py" services "$LOG.services"
		EVENTS_LOG="$LOG.events"
		curl -kfsS --max-time 5 -N https://127.0.0.1:18081/api/v1/events >"$EVENTS_LOG" 2>/dev/null || true
		python3 "$ROOT/scripts/validate-sse.py" "$EVENTS_LOG" system.metrics
		grep -F '"id":"lumonasd.service","name":"lumonasd.service","active":true,"state":"running","user":"lumonas"' "$LOG.services" >/dev/null
		grep -F '"id":"lumonas-web.service","name":"lumonas-web.service","active":true,"state":"running","user":"lumonas"' "$LOG.services" >/dev/null
		grep -F '"id":"lumonas-privd.service","name":"lumonas-privd.service","active":true,"state":"running"' "$LOG.services" >/dev/null
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
