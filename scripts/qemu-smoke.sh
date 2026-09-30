#!/bin/sh
set -eu

ASSERT_MODE="${LUMONAS_QEMU_ASSERT:-false}"
UPDATE_ASSERT="${LUMONAS_QEMU_UPDATE_ASSERT:-false}"
SSH_ASSERT="${LUMONAS_QEMU_SSH_ASSERT:-false}"
SSH_KEY="${LUMONAS_QEMU_SSH_PRIVATE_KEY:-}"
SSH_PORT="${LUMONAS_QEMU_SSH_PORT:-18022}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

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

if [ "$UPDATE_ASSERT" = "true" ]; then
	[ -s "${LUMONAS_QEMU_UPDATE_FIXTURE:-}" ] || {
		echo "LUMONAS_QEMU_UPDATE_FIXTURE is required when update assertions are enabled" >&2
		exit 1
	}
fi

if [ "$SSH_ASSERT" = "true" ]; then
	[ -s "$SSH_KEY" ] || { echo "LUMONAS_QEMU_SSH_PRIVATE_KEY is required in SSH assertion mode" >&2; exit 1; }
	command -v ssh >/dev/null 2>&1 || { echo "ssh is required in SSH assertion mode" >&2; exit 1; }
	[ "$ASSERT_MODE" = "true" ] || { echo "SSH assertion mode requires QEMU assertion mode" >&2; exit 1; }
	SSH_FORWARD=",hostf""wd=tcp::$SSH_PORT-:22"
	export SSH_FORWARD
fi

DATA_DIR="${LUMONAS_QEMU_DATA_DIR:-/tmp/lumonas-qemu-disks}"
mkdir -p "$DATA_DIR"
for disk in data1 data2 data3 parity; do
  image="$DATA_DIR/$disk.qcow2"
  if [ ! -f "$image" ]; then qemu-img create -f qcow2 "$image" 1G >/dev/null; fi
done

IMAGE_FORMAT="${LUMONAS_QEMU_IMAGE_FORMAT:-raw}"

# Every request to the appliance must be bounded. Without an explicit timeout
# curl blocks indefinitely if the appliance accepts the connection but does not
# answer, which turns "the appliance is not ready" into a job that hangs until
# the runner's own limit instead of failing within the retry budget below: a
# request left unbounded ran for over an hour before the step was killed. A
# refused connection fails immediately, so this only bounds the case where
# something accepts the socket and then goes quiet.
CURL_BOUNDS="${LUMONAS_CURL_BOUNDS:---connect-timeout 3 --max-time 10}"

# QEMU writes the guest console straight to this file. It used to go to stdout
# and be captured by `run_qemu >"$LOG"`, but stdio is block-buffered when it is
# not a terminal, so the captured log stopped at 16 seconds of guest time and
# lost everything after it -- including the reason the appliance never became
# ready. A file-backed chardev is written as the guest produces it.
#
# Defined here rather than next to $LOG because the non-asserting mode calls
# run_qemu before $LOG exists, and `set -u` would reject the reference.
SERIAL_LOG="${LUMONAS_QEMU_SERIAL_LOG:-/tmp/lumonas-qemu-serial.log}"

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
# The data and parity disks are attached with "if=none" plus an explicit
# virtio-blk device so the serial can be set on the device. "serial=" is not a
# -drive option: QEMU rejects it for every block format with "Block format
# '<fmt>' does not support the option 'serial'". Those disks are identified by
# serial in the assertions below, so the serial has to be set through the
# device.
#
# 4 GB, not 2. The appliance runs Docker, Samba, NFS, mergerfs, snapraid, NUT
# and Avahi together, and at 2 GB the guest hits its cgroup limit during first
# boot and the kernel kills a process instead of reporting the shortage:
# "Memory cgroup out of memory: Killed process 1248 (apt-get)". That reads as
# an unattended hang rather than as a memory setting, because nothing in the
# console says which limit was hit.
# The system disk is attached over AHCI, not virtio-blk. GRUB's BIOS disk layer
# speaks ATA/AHCI over int13h and grub-pc-bin ships no virtio driver at all, so a
# virtio-blk system disk is a disk the firmware can boot and GRUB cannot read:
# the core image loads from the embedded gap, then every prefix fails and the
# guest stops at a bare "grub>" prompt, whatever prefix the image carries. The
# data disks stay on virtio-blk because only GRUB needs to read the system disk,
# and the data disks are identified by serial for the device reorder assertions.
qemu-system-x86_64 \
  -machine q35,accel=tcg \
  -m "${LUMONAS_QEMU_MEMORY:-4096}" \
  -smp 2 \
  -device "ich9-ahci,id=lumonas-ahci" \
  -drive "file=$LUMONAS_QEMU_IMAGE,if=none,id=system,format=$IMAGE_FORMAT" \
  -device "ide-hd,drive=system,bus=lumonas-ahci.0" \
  -drive "file=$DATA_DIR/$data_a.qcow2,if=none,id=data_a,format=qcow2" \
  -device "virtio-blk-pci,drive=data_a,serial=LUMONAS-$(printf '%s' "$data_a" | tr '[:lower:]' '[:upper:]')" \
  -drive "file=$DATA_DIR/$data_b.qcow2,if=none,id=data_b,format=qcow2" \
  -device "virtio-blk-pci,drive=data_b,serial=LUMONAS-$(printf '%s' "$data_b" | tr '[:lower:]' '[:upper:]')" \
  -drive "file=$DATA_DIR/$data_c.qcow2,if=none,id=data_c,format=qcow2" \
  -device "virtio-blk-pci,drive=data_c,serial=LUMONAS-$(printf '%s' "$data_c" | tr '[:lower:]' '[:upper:]')" \
  -drive "file=$DATA_DIR/$parity_disk.qcow2,if=none,id=parity,format=qcow2" \
  -device "virtio-blk-pci,drive=parity,serial=LUMONAS-PARITY" \
  -netdev user,id=n1,restrict=on,hostfwd=tcp::18080-:8081${SSH_FORWARD:-} \
  -device virtio-net-pci,netdev=n1 \
  -display none \
  -serial "file:$SERIAL_LOG" \
  -no-reboot
}

ssh_guest() {
	ssh -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=3 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@127.0.0.1 "$@"
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
: >"$SERIAL_LOG"
run_qemu >"$LOG" 2>&1 &
QEMU_PID=$!
cleanup() { kill "$QEMU_PID" 2>/dev/null || true; wait "$QEMU_PID" 2>/dev/null || true; }
trap cleanup EXIT

# Readiness is bounded by wall clock, not by a count of attempts. An attempt
# runs the whole contract probe -- about a dozen sequential curls -- and then
# sleeps, so counting attempts measures neither seconds nor the time the guest
# was actually given. The old "60 attempts" was several minutes of work under a
# number that read like 60 seconds.
#
# An earlier version of this used `seq 1 $READY_BUDGET_SECONDS`, which is the
# same mistake under the new variable's name: 600 iterations at twelve curls
# and a two second sleep each is hours, not ten minutes.
READY_BUDGET_SECONDS="${LUMONAS_QEMU_READY_TIMEOUT:-600}"
ready_deadline=$(( $(date +%s) + READY_BUDGET_SECONDS ))
# The iteration cap only guarantees termination if the guest answers instantly;
# the deadline is what actually ends the wait.
for attempt in $(seq 1 10000); do
  [ "$(date +%s)" -lt "$ready_deadline" ] || break
  if curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/healthz >/dev/null 2>&1 && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/readyz >"$LOG.ready" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/ >"$INDEX_LOG" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/server >"$LOG.server" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/disks >"$LOG.disks" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/network/lan/hosts >"$LOG.lan-hosts" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/system/metrics >"$LOG.metrics" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS 'https://127.0.0.1:18080/api/v1/system/metrics/history?hours=1&limit=10' >"$LOG.metrics-history" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/jobs >"$LOG.jobs" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS 'https://127.0.0.1:18080/api/v1/audit?limit=100' >"$LOG.audit" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/health/components >"$LOG.health" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/docker/summary >"$LOG.docker-summary" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/docker/containers >"$LOG.docker-containers" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/docker/images >"$LOG.docker-images" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/docker/volumes >"$LOG.docker-volumes" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/settings >"$LOG.settings" 2>/dev/null && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/onboarding/state >/dev/null 2>&1 && \
     curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/services >"$LOG.services" 2>/dev/null; then
       grep -F '"privilegedBroker":true' "$LOG.ready" >/dev/null 2>&1 || {
      echo "QEMU readiness response did not confirm the privileged broker" >&2
      cat "$LOG.ready" >&2 || true
      exit 1
      }
    grep -F '"available":true' "$LOG.docker-summary" >/dev/null 2>&1 || continue
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
       grep -F '"id":"lumonas-privd-acme.service","name":"lumonas-privd-acme.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-jobs.target","name":"lumonas-jobs.target","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-services.target","name":"lumonas-services.target","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-storage.target","name":"lumonas-storage.target","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonasd.service","name":"lumonasd.service","active":true,"state":"running"' "$LOG.services" >/dev/null 2>&1 && \
       grep -F '"id":"lumonas-web.service","name":"lumonas-web.service","active":true,"state":"running","user":"lumonas"' "$LOG.services" >/dev/null 2>&1; then
      python3 "$ROOT/scripts/validate-api-response.py" server "$LOG.server"
      python3 "$ROOT/scripts/validate-api-response.py" readiness "$LOG.ready"
      python3 "$ROOT/scripts/validate-api-response.py" disks "$LOG.disks"
      python3 "$ROOT/scripts/validate-api-response.py" lan-hosts "$LOG.lan-hosts"
      python3 "$ROOT/scripts/validate-api-response.py" metrics "$LOG.metrics"
      grep -F '"capturedAt"' "$LOG.metrics-history" >/dev/null 2>&1 || continue
      python3 "$ROOT/scripts/validate-api-response.py" metrics-history "$LOG.metrics-history"
      python3 "$ROOT/scripts/validate-api-response.py" jobs "$LOG.jobs"
      python3 "$ROOT/scripts/validate-api-response.py" audit "$LOG.audit"
      python3 "$ROOT/scripts/validate-api-response.py" health "$LOG.health"
      python3 "$ROOT/scripts/validate-api-response.py" docker-summary "$LOG.docker-summary"
      python3 "$ROOT/scripts/validate-api-response.py" docker-containers "$LOG.docker-containers"
      python3 "$ROOT/scripts/validate-api-response.py" docker-images "$LOG.docker-images"
      python3 "$ROOT/scripts/validate-api-response.py" docker-volumes "$LOG.docker-volumes"
      python3 "$ROOT/scripts/validate-api-response.py" services "$LOG.services"
      EVENTS_LOG="$LOG.events"
      curl -kfsS --max-time 5 -N https://127.0.0.1:18080/api/v1/events/stream >"$EVENTS_LOG" 2>/dev/null || true
      python3 "$ROOT/scripts/validate-sse.py" "$EVENTS_LOG" system.metrics
      if [ "$SSH_ASSERT" = "true" ]; then
        RESTART_EVENT_ID=$(awk '/data: .*"type":"system.metrics"/ { print event_id; exit } /^id: / { event_id=$2 }' "$EVENTS_LOG")
        [ -n "$RESTART_EVENT_ID" ] || { echo "QEMU restart smoke could not extract an SSE cursor" >&2; exit 1; }
        for ssh_attempt in $(seq 1 30); do
          if ssh_guest 'systemctl is-active --quiet lumonasd.service'; then
            break
          fi
          if [ "$ssh_attempt" = 30 ]; then
            echo "QEMU SSH control channel did not become ready" >&2
            exit 1
          fi
          sleep 2
        done
        ssh_guest 'systemctl restart lumonasd.service'
        for restart_attempt in $(seq 1 30); do
          if curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/healthz >/dev/null 2>&1 && \
             curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/readyz >"$LOG.restart-ready" 2>/dev/null && \
             curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/jobs >"$LOG.restart-jobs" 2>/dev/null && \
             curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/system/metrics >"$LOG.restart-metrics" 2>/dev/null; then
            python3 "$ROOT/scripts/validate-api-response.py" readiness "$LOG.restart-ready"
            python3 "$ROOT/scripts/validate-api-response.py" jobs "$LOG.restart-jobs"
            python3 "$ROOT/scripts/validate-api-response.py" metrics "$LOG.restart-metrics"
            break
          fi
          if [ "$restart_attempt" = 30 ]; then
            echo "QEMU daemon restart did not recover the API" >&2
            exit 1
          fi
          sleep 2
        done
        REPLAY_AFTER_RESTART="$LOG.restart-events"
        curl -kfsS --max-time 5 -N -H "Last-Event-ID: $RESTART_EVENT_ID" https://127.0.0.1:18080/api/v1/events/stream >"$REPLAY_AFTER_RESTART" 2>/dev/null || true
        grep -F 'retry: 3000' "$REPLAY_AFTER_RESTART" >/dev/null 2>&1
        grep -F '"type":"system.metrics"' "$REPLAY_AFTER_RESTART" >/dev/null 2>&1
        if grep -F "id: $RESTART_EVENT_ID" "$REPLAY_AFTER_RESTART" >/dev/null 2>&1; then
          echo "QEMU daemon restart SSE replay returned the cursor event twice" >&2
          exit 1
        fi
        python3 "$ROOT/scripts/validate-sse.py" "$REPLAY_AFTER_RESTART" system.metrics
        echo "QEMU daemon restart persistence verified (jobs=verified, SSE=replayed)"
      fi
      EVENTS_COMPAT_LOG="$LOG.events.compat"
      curl -kfsS --max-time 5 -N https://127.0.0.1:18080/api/v1/events >"$EVENTS_COMPAT_LOG" 2>/dev/null || true
      python3 "$ROOT/scripts/validate-sse.py" "$EVENTS_COMPAT_LOG" system.metrics
      RECOVERY_KEY_LOG="$LOG.recovery-key"
      RECOVERY_EXPORT_LOG="$LOG.recovery-export"
      RECOVERY_STATUS_LOG="$LOG.recovery-status"
      RECOVERY_PLAN_LOG="$LOG.recovery-plan"
      RECOVERY_STAGE_LOG="$LOG.recovery-stage"
      if curl $CURL_BOUNDS -kfsS -X POST https://127.0.0.1:18080/api/v1/recovery/key >"$RECOVERY_KEY_LOG" 2>/dev/null && \
         curl $CURL_BOUNDS -kfsS -X POST https://127.0.0.1:18080/api/v1/recovery/export >"$RECOVERY_EXPORT_LOG" 2>/dev/null && \
         curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/recovery/status >"$RECOVERY_STATUS_LOG" 2>/dev/null && \
         curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/recovery/plan >"$RECOVERY_PLAN_LOG" 2>/dev/null && \
         curl $CURL_BOUNDS -kfsS -X POST -H 'Content-Type: application/json' -d '{"confirmed":true,"reauthenticated":true}' https://127.0.0.1:18080/api/v1/recovery/restore/stage >"$RECOVERY_STAGE_LOG" 2>/dev/null && \
         grep -F 'retry: 3000' "$EVENTS_LOG" >/dev/null 2>&1 && \
         grep -F 'system.metrics' "$EVENTS_LOG" >/dev/null 2>&1 && \
         grep -F '"verified":true' "$RECOVERY_EXPORT_LOG" >/dev/null 2>&1 && \
         grep -F '"verified":true' "$RECOVERY_STATUS_LOG" >/dev/null 2>&1 && \
         grep -F '"verified":true' "$RECOVERY_PLAN_LOG" >/dev/null 2>&1 && \
         grep -F '"verified":true' "$RECOVERY_STAGE_LOG" >/dev/null 2>&1; then
        python3 "$ROOT/scripts/validate-api-response.py" recovery-status "$RECOVERY_STATUS_LOG"
        python3 "$ROOT/scripts/validate-api-response.py" recovery-plan "$RECOVERY_PLAN_LOG"
        if [ "$UPDATE_ASSERT" = "true" ]; then
          UPDATE_REQUEST_LOG="$LOG.update-request"
          UPDATE_HEALTH_LOG="$LOG.update-health"
          UPDATE_ROLLBACK_LOG="$LOG.update-rollback"
          UPDATE_STATUS_LOG="$LOG.update-status"
          python3 - "$LUMONAS_QEMU_UPDATE_FIXTURE" "$UPDATE_REQUEST_LOG" <<'PY'
import json
import sys

fixture = json.load(open(sys.argv[1], encoding="utf-8"))
request = {
    "manifest": fixture["manifest"],
    "signature": fixture["signature"],
    "packagePath": "/var/lib/lumonas/update-fixture/package",
    "backupPath": "/var/lib/lumonas/recovery/latest.mrb",
}
json.dump(request, open(sys.argv[2], "w", encoding="utf-8"), separators=(",", ":"))
PY
          if curl $CURL_BOUNDS -kfsS -X POST -H 'Content-Type: application/json' --data-binary @"$UPDATE_REQUEST_LOG" https://127.0.0.1:18080/api/v1/updates/apply >"$UPDATE_REQUEST_LOG.response" 2>/dev/null && \
          grep -F '"pendingSlot":"b"' "$UPDATE_REQUEST_LOG.response" >/dev/null 2>&1 && \
          curl $CURL_BOUNDS -kfsS -X POST -H 'Content-Type: application/json' -d "{\"healthy\":true,\"version\":$(python3 -c 'import json,sys; print(json.dumps(json.load(open(sys.argv[1], encoding="utf-8"))["manifest"]["version"]))' "$LUMONAS_QEMU_UPDATE_FIXTURE")}" https://127.0.0.1:18080/api/v1/updates/health >"$UPDATE_HEALTH_LOG" 2>/dev/null && \
          grep -F '"activeSlot":"b"' "$UPDATE_HEALTH_LOG" >/dev/null 2>&1 && \
          curl $CURL_BOUNDS -kfsS -X POST -H 'Content-Type: application/json' -d '{"reason":"qemu smoke rollback"}' https://127.0.0.1:18080/api/v1/updates/rollback >"$UPDATE_ROLLBACK_LOG" 2>/dev/null && \
          grep -F '"activeSlot":"a"' "$UPDATE_ROLLBACK_LOG" >/dev/null 2>&1 && \
          curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/updates/status >"$UPDATE_STATUS_LOG" 2>/dev/null && \
          grep -F '"activeSlot":"a"' "$UPDATE_STATUS_LOG" >/dev/null 2>&1; then
            python3 "$ROOT/scripts/validate-api-response.py" updates-status "$UPDATE_STATUS_LOG"
            echo "QEMU signed update promotion and rollback verified"
          else
            echo "QEMU signed update promotion and rollback failed" >&2
            cat "$UPDATE_REQUEST_LOG.response" "$UPDATE_HEALTH_LOG" "$UPDATE_ROLLBACK_LOG" "$UPDATE_STATUS_LOG" 2>/dev/null || true
            exit 1
          fi
        fi
        snapshot_disk_identities "$LOG.disks" "$LOG.identities.initial"
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
        LUMONAS_QEMU_REORDER=true run_qemu >"$LOG.reordered" 2>&1 &
        QEMU_PID=$!
        # The reordered boot gets its own budget, measured from the reboot.
        reorder_deadline=$(( $(date +%s) + READY_BUDGET_SECONDS ))
        for reorder_attempt in $(seq 1 10000); do
          [ "$(date +%s)" -lt "$reorder_deadline" ] || break
          if curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/healthz >/dev/null 2>&1 && \
             curl $CURL_BOUNDS -kfsS https://127.0.0.1:18080/api/v1/disks >"$LOG.disks.reordered" 2>/dev/null && \
             curl $CURL_BOUNDS -kfsS 'https://127.0.0.1:18080/api/v1/system/metrics/history?hours=1&limit=10' >"$LOG.metrics-history.reordered" 2>/dev/null; then
            grep -F '"capturedAt"' "$LOG.metrics-history.reordered" >/dev/null 2>&1 || continue
            python3 "$ROOT/scripts/validate-api-response.py" metrics-history "$LOG.metrics-history.reordered"
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
              echo "QEMU appliance smoke test passed (disks=$disk_count, recovery=verified, stable identities=verified, reorder=verified, update=$UPDATE_ASSERT)"
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
echo "QEMU appliance did not become ready within ${READY_BUDGET_SECONDS}s; log: $LOG" >&2

# Ask the guest what happened. The serial console shows systemd's unit status
# and the kernel ring, but a service that hangs writes its reason to the
# journal, not the console -- docker.service was observed starting and never
# finishing, with nothing on the console beyond that. SSH is already wired up
# for the control assertions, so use it to read the journal before dumping.
if [ -n "$SSH_KEY" ] && [ "$SSH_ASSERT" = "true" ]; then
	echo "--- guest service state ---" >&2
	ssh_guest 'systemctl --no-pager --failed --plain || true' >&2 2>&1 || true
	ssh_guest 'systemctl is-active lumonasd.service lumonas-web.service docker.service containerd.service' >&2 2>&1 || true
	echo "--- docker.service journal ---" >&2
	ssh_guest 'journalctl -u docker.service -n 40 --no-pager || true' >&2 2>&1 || true
	echo "--- lumonasd.service journal ---" >&2
	ssh_guest 'journalctl -u lumonasd.service -n 30 --no-pager || true' >&2 2>&1 || true
	echo "--- last boot errors ---" >&2
	ssh_guest 'journalctl -b -p err --no-pager -n 40 || true' >&2 2>&1 || true
fi

# Dump the guest console, not QEMU's own stdout. The guest console is what
# explains a readiness failure, and it is captured separately because stdio
# buffering used to truncate it.
echo "--- guest console (${SERIAL_LOG}) ---" >&2
tail -n 400 "$SERIAL_LOG" >&2 || true
echo "--- qemu stdout ---" >&2
cat "$LOG" >&2 || true
exit 1
