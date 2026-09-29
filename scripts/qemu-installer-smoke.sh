#!/bin/sh
set -eu

ASSERT_MODE="${LUMONAS_INSTALLER_ASSERT:-false}"
CONTRACT_ASSERT="${LUMONAS_INSTALLER_CONTRACT_ASSERT:-$ASSERT_MODE}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
ISO="${LUMONAS_ISO:-}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-installer.XXXXXX")"
DISK="$WORK/blank-system.qcow2"
LIVE_LOG="${LUMONAS_INSTALLER_LIVE_LOG:-$WORK/live.log}"
INSTALLED_LOG="${LUMONAS_INSTALLER_INSTALLED_LOG:-$WORK/installed.log}"
DEBUG_DIR="${LUMONAS_INSTALLER_DEBUG_DIR:-}"
QEMU_PID=""

cleanup() {
	status=$?
	set +e
	if [ -n "$QEMU_PID" ]; then
		kill "$QEMU_PID" 2>/dev/null || true
		wait "$QEMU_PID" 2>/dev/null || true
	fi
	if [ "$status" -ne 0 ] && [ -n "$DEBUG_DIR" ]; then
		mkdir -p "$DEBUG_DIR"
		cp -a "$WORK"/. "$DEBUG_DIR"/ 2>/dev/null || true
	fi
	rm -rf "$WORK"
	return "$status"
}
trap cleanup EXIT INT TERM

if ! command -v qemu-system-x86_64 >/dev/null 2>&1; then
	if [ "$ASSERT_MODE" = "true" ]; then
		echo "qemu-system-x86_64 is required in assertion mode" >&2
		exit 1
	fi
	echo "qemu-system-x86_64 is unavailable; QEMU installer smoke skipped" >&2
	exit 0
fi
[ -n "$ISO" ] && [ -f "$ISO" ] || {
	if [ "$ASSERT_MODE" = "true" ]; then
		echo "LUMONAS_ISO must point to a generated ISO in assertion mode" >&2
		exit 1
	fi
	echo "Set LUMONAS_ISO to a generated LumoNAS ISO" >&2
	exit 0
}
command -v qemu-img >/dev/null 2>&1 || { echo "qemu-img is required" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "python3 is required" >&2; exit 1; }

# The production installer rejects smaller disks. Keep this disposable disk
# above that floor so the smoke exercises the real partitioning boundary.
qemu-img create -f qcow2 "$DISK" 12G >/dev/null

qemu-system-x86_64 \
	-machine q35,accel=tcg \
	-m 2048 \
	-smp 2 \
	-cdrom "$ISO" \
	-drive "file=$DISK,if=none,id=replacement,format=qcow2" \
	-device "virtio-blk-pci,drive=replacement,serial=LUMONAS-REPLACEMENT" \
	-netdev user,id=n1,restrict=on,hostfwd=tcp::18082-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-boot d \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$LIVE_LOG" 2>&1 &
QEMU_PID=$!

live_ready=false
for attempt in $(seq 1 120); do
	if curl -kfsS https://127.0.0.1:18082/healthz >/dev/null 2>&1 && \
		curl -kfsS https://127.0.0.1:18082/readyz >"$WORK/live-ready.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18082/api/v1/install/targets >"$WORK/targets.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18082/install >"$WORK/install.html" 2>/dev/null; then
		live_ready=true
		break
	fi
	if ! kill -0 "$QEMU_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
[ "$live_ready" = true ] || {
	echo "live installer did not become ready" >&2
	cat "$LIVE_LOG" >&2 || true
	exit 1
}
grep -F '<div id="root"></div>' "$WORK/install.html" >/dev/null || {
	echo "live installer route did not serve the frontend shell" >&2
	exit 1
}
grep -F '"privilegedBroker":true' "$WORK/live-ready.json" >/dev/null

python3 - "$WORK/targets.json" "$WORK/target.id" <<'PY'
import json
import sys

targets = json.load(open(sys.argv[1], encoding="utf-8"))
eligible = [target for target in targets if target.get("eligible")]
if len(eligible) != 1:
    raise SystemExit(f"expected one eligible installer target, got {len(eligible)}")
if not eligible[0].get("identity", {}).get("serial"):
    raise SystemExit("installer target is missing stable serial identity")
open(sys.argv[2], "w", encoding="utf-8").write(eligible[0]["diskId"])
PY
TARGET_ID="$(sed -n '1p' "$WORK/target.id")"

python3 - "$TARGET_ID" "$WORK/plan-request.json" <<'PY'
import json
import sys

json.dump({
    "targetDiskId": sys.argv[1],
    "hostname": "lumonas-installed",
    "adminUsername": "admin",
    "filesystem": "ext4",
    "uefi": False,
}, open(sys.argv[2], "w", encoding="utf-8"), separators=(",", ":"))
PY
curl -kfsS -X POST -H 'Content-Type: application/json' \
	--data-binary @"$WORK/plan-request.json" \
	https://127.0.0.1:18082/api/v1/install/plan >"$WORK/plan.json"
python3 - "$WORK/plan.json" "$TARGET_ID" "$WORK/plan.hash" <<'PY'
import json
import sys

payload = json.load(open(sys.argv[1], encoding="utf-8"))
if payload.get("plan", {}).get("targetDiskId") != sys.argv[2]:
    raise SystemExit("installation plan target changed")
plan_hash = payload.get("hash", "")
if not plan_hash:
    raise SystemExit("installation plan has no hash")
open(sys.argv[3], "w", encoding="utf-8").write(plan_hash)
PY
PLAN_HASH="$(sed -n '1p' "$WORK/plan.hash")"

python3 - "$PLAN_HASH" "$WORK/apply-request.json" <<'PY'
import json
import sys

json.dump({
    "hash": sys.argv[1],
    "confirm": True,
    "adminPassword": "a-very-strong-test-password",
}, open(sys.argv[2], "w", encoding="utf-8"), separators=(",", ":"))
PY
curl -kfsS -X POST -H 'Content-Type: application/json' \
	--data-binary @"$WORK/apply-request.json" \
	https://127.0.0.1:18082/api/v1/install/apply >"$WORK/apply.json"
grep -F '"status":"succeeded"' "$WORK/apply.json" >/dev/null || {
	echo "live installer did not report success" >&2
	cat "$WORK/apply.json" >&2
	exit 1
}

install_succeeded=false
for attempt in $(seq 1 30); do
	if curl -kfsS https://127.0.0.1:18082/api/v1/install/status >"$WORK/install-status.json" 2>/dev/null && \
		grep -F '"stage":"succeeded"' "$WORK/install-status.json" >/dev/null; then
		install_succeeded=true
		break
	fi
	sleep 2
done
[ "$install_succeeded" = true ] || {
	echo "live installer status did not reach succeeded" >&2
	cat "$WORK/install-status.json" >&2 || true
	exit 1
}

kill "$QEMU_PID" 2>/dev/null || true
wait "$QEMU_PID" 2>/dev/null || true
QEMU_PID=""

# Boot the newly provisioned disk without the ISO. This proves the copied
# runtime, fstab, bootloader, first-boot credential, and systemd graph work as
# one appliance path.
qemu-system-x86_64 \
	-machine q35,accel=tcg \
	-m 2048 \
	-smp 2 \
	-drive "file=$DISK,if=none,id=installed,format=qcow2" \
	-device "virtio-blk-pci,drive=installed,serial=LUMONAS-INSTALLED" \
	-netdev user,id=n1,restrict=on,hostfwd=tcp::18083-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$INSTALLED_LOG" 2>&1 &
QEMU_PID=$!

installed_ready=false
for attempt in $(seq 1 120); do
	if curl -kfsS https://127.0.0.1:18083/healthz >/dev/null 2>&1 && \
		curl -kfsS https://127.0.0.1:18083/readyz >"$WORK/installed-ready.json" 2>/dev/null && \
		curl -kfsS https://127.0.0.1:18083/api/v1/auth/status >"$WORK/auth-status.json" 2>/dev/null; then
		installed_ready=true
		break
	fi
	if ! kill -0 "$QEMU_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
[ "$installed_ready" = true ] || {
	echo "installed disk did not become ready" >&2
	cat "$INSTALLED_LOG" >&2 || true
	exit 1
}
grep -F '"privilegedBroker":true' "$WORK/installed-ready.json" >/dev/null
python3 "$ROOT/scripts/validate-api-response.py" readiness "$WORK/installed-ready.json"
grep -F '"configured":true' "$WORK/auth-status.json" >/dev/null

printf '%s\n' '{"username":"admin","password":"a-very-strong-test-password"}' >"$WORK/login.json"
curl -kfsS -c "$WORK/cookies.txt" -X POST -H 'Content-Type: application/json' \
	--data-binary @"$WORK/login.json" \
	https://127.0.0.1:18083/api/v1/auth/login >"$WORK/login-response.json"
curl -kfsS -b "$WORK/cookies.txt" https://127.0.0.1:18083/api/v1/server >"$WORK/installed-server.json"
python3 "$ROOT/scripts/validate-api-response.py" server "$WORK/installed-server.json"

if [ "$CONTRACT_ASSERT" = "true" ]; then
	for endpoint in server disks metrics jobs health services; do
		case "$endpoint" in
			server) path=/api/v1/server; output="$WORK/installed-server.json"; validator=server ;;
			disks) path=/api/v1/disks; output="$WORK/installed-disks.json"; validator=disks ;;
			metrics) path=/api/v1/system/metrics; output="$WORK/installed-metrics.json"; validator=metrics ;;
			jobs) path=/api/v1/jobs; output="$WORK/installed-jobs.json"; validator=jobs ;;
			health) path=/api/v1/health/components; output="$WORK/installed-health.json"; validator=health ;;
			services) path=/api/v1/services; output="$WORK/installed-services.json"; validator=services ;;
		esac
		if [ "$endpoint" != "server" ]; then
			curl -kfsS -b "$WORK/cookies.txt" "https://127.0.0.1:18083$path" >"$output"
		fi
		python3 "$ROOT/scripts/validate-api-response.py" "$validator" "$output"
	done
	curl -kfsS -b "$WORK/cookies.txt" https://127.0.0.1:18083/ >"$WORK/installed-index.html"
	grep -F '<title>LumoNAS</title>' "$WORK/installed-index.html" >/dev/null
	grep -F '<div id="root"></div>' "$WORK/installed-index.html" >/dev/null
	grep -F '"id":"lumonas-web.service","name":"lumonas-web.service","active":true,"state":"running","user":"lumonas"' "$WORK/installed-services.json" >/dev/null
	grep -F '"id":"lumonasd.service","name":"lumonasd.service","active":true,"state":"running","user":"lumonas"' "$WORK/installed-services.json" >/dev/null
	installed_events_log="$WORK/installed-events.sse"
	curl -kfsS --max-time 5 -N -b "$WORK/cookies.txt" https://127.0.0.1:18083/api/v1/events/stream >"$installed_events_log" 2>/dev/null || true
	python3 "$ROOT/scripts/validate-sse.py" "$installed_events_log" system.metrics
fi

installer_status_code="$(curl -ksS -o /dev/null -w '%{http_code}' -b "$WORK/cookies.txt" https://127.0.0.1:18083/api/v1/install/status)"
[ "$installer_status_code" = "404" ] || {
	echo "installer endpoint remained available after installation: HTTP $installer_status_code" >&2
	exit 1
}

echo "LumoNAS QEMU installer smoke passed (live install, installed-disk boot, and post-install runtime contract verified)"
