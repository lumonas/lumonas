#!/bin/sh
set -eu

SOURCE_IMAGE=${1:-}
WORK=${2:-}
[ -f "$SOURCE_IMAGE" ] || { echo "live recovery source image not found: $SOURCE_IMAGE" >&2; exit 1; }
[ -n "$WORK" ] || { echo "live recovery source work directory is required" >&2; exit 1; }
for command in qemu-img qemu-system-x86_64 mount umount curl python3; do
	command -v "$command" >/dev/null 2>&1 || { echo "$command is required" >&2; exit 1; }
done

SOURCE_RAW="$WORK/source.raw"
SOURCE_MOUNT="$WORK/source-mount"
SOURCE_DATA="$WORK/source-disks"
SOURCE_LOG="$WORK/live-source.log"
SOURCE_PID=""
SOURCE_API="https://127.0.0.1:18083"
cleanup() {
	set +e
	if [ -n "$SOURCE_PID" ]; then
		kill "$SOURCE_PID" 2>/dev/null || true
		wait "$SOURCE_PID" 2>/dev/null || true
	fi
	umount "$SOURCE_MOUNT" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

cp "$SOURCE_IMAGE" "$SOURCE_RAW"
mkdir -p "$SOURCE_MOUNT" "$SOURCE_DATA"
mount -o loop "$SOURCE_RAW" "$SOURCE_MOUNT"
mkdir -p "$SOURCE_MOUNT/var/lib/lumonas"
printf '%s\n' 'live-source-recovery-secret' >"$SOURCE_MOUNT/var/lib/lumonas/source-recovery-secret"
# The file is created from the host before the guest's lumonas UID exists;
# this disposable test image uses a readable mode so the unprivileged daemon
# can consume it. The image is destroyed by the harness after the run.
chmod 0644 "$SOURCE_MOUNT/var/lib/lumonas/source-recovery-secret"
printf '%s\n' 'LUMONAS_RECOVERY_SECRETS_FILE=/var/lib/lumonas/source-recovery-secret' >>"$SOURCE_MOUNT/etc/lumonas/lumonasd.env"
mkdir -p "$SOURCE_MOUNT/srv/lumonas/docker/appdata/media"
printf '%s\n' 'mode: live-source' >"$SOURCE_MOUNT/srv/lumonas/docker/appdata/media/config.yaml"
umount "$SOURCE_MOUNT"
for disk in data1 data2 data3 parity; do
	qemu-img create -f qcow2 "$SOURCE_DATA/$disk.qcow2" 1G >/dev/null
done

qemu-system-x86_64 \
	-machine q35,accel=tcg \
	-m 2048 \
	-smp 2 \
	-drive "file=$SOURCE_RAW,if=virtio,format=raw,serial=LUMONAS-SYSTEM" \
	-drive "file=$SOURCE_DATA/data1.qcow2,if=virtio,format=qcow2,serial=LUMONAS-DATA1" \
	-drive "file=$SOURCE_DATA/data2.qcow2,if=virtio,format=qcow2,serial=LUMONAS-DATA2" \
	-drive "file=$SOURCE_DATA/data3.qcow2,if=virtio,format=qcow2,serial=LUMONAS-DATA3" \
	-drive "file=$SOURCE_DATA/parity.qcow2,if=virtio,format=qcow2,serial=LUMONAS-PARITY" \
	-netdev user,id=n1,restrict=on,hostfwd=tcp::18083-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$SOURCE_LOG" 2>&1 &
SOURCE_PID=$!

source_ready=false
for attempt in $(seq 1 120); do
	if curl -kfsS "$SOURCE_API/healthz" >/dev/null 2>&1 && \
		curl -kfsS "$SOURCE_API/readyz" >"$WORK/source-ready.json" 2>/dev/null && \
		curl -kfsS "$SOURCE_API/api/v1/server" >"$WORK/source-server.json" 2>/dev/null && \
		curl -kfsS "$SOURCE_API/api/v1/services" >"$WORK/source-services.json" 2>/dev/null; then
		source_ready=true
		break
	fi
	if ! kill -0 "$SOURCE_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
[ "$source_ready" = true ] || { echo "live recovery source appliance never became ready" >&2; cat "$SOURCE_LOG" >&2 || true; exit 1; }
grep -F '"privilegedBroker":true' "$WORK/source-ready.json" >/dev/null
python3 "$ROOT/scripts/validate-api-response.py" server "$WORK/source-server.json"
python3 "$ROOT/scripts/validate-api-response.py" services "$WORK/source-services.json"

curl -kfsS -X POST "$SOURCE_API/api/v1/recovery/key" >"$WORK/source-key.json"
python3 -c 'import json,sys; value=json.load(open(sys.argv[1],encoding="utf-8")).get("key","").strip(); assert value; open(sys.argv[2],"w",encoding="utf-8").write(value+"\n")' "$WORK/source-key.json" "$WORK/recovery.key"
curl -kfsS "$SOURCE_API/api/v1/disks" >"$WORK/source-disks.json"
python3 - "$WORK/source-disks.json" "$WORK/source-onboarding.json" "$WORK/source-protection.json" "$WORK/source-layout.json" <<'PY'
import json
import re
import sys

disks = json.load(open(sys.argv[1], encoding="utf-8"))
data = [disk["id"] for disk in disks if "DATA" in disk.get("id", "").upper()]
parity = [disk["id"] for disk in disks if "PARITY" in disk.get("id", "").upper()]
assert len(data) >= 2 and parity
roles = {item: "data" for item in data}
roles[parity[0]] = "parity"
json.dump({"serverName": "source-recovery-nas", "roles": roles, "protection": {"syncTime": "", "scrubDay": ""}, "recovery": {"autoConfigBackup": False, "keyAcknowledged": True}}, open(sys.argv[2], "w", encoding="utf-8"))
json.dump({"parityDiskId": parity[0], "dataDiskIds": data}, open(sys.argv[3], "w", encoding="utf-8"))
def branch(disk_id):
    return "/srv/disks/" + re.sub(r"[^A-Za-z0-9._-]", "_", disk_id)
json.dump({"data": data, "parity": parity[0], "all": data + parity, "branches": {item: branch(item) for item in data + parity}}, open(sys.argv[4], "w", encoding="utf-8"))
PY
curl -kfsS -X POST -H 'Content-Type: application/json' --data-binary @"$WORK/source-onboarding.json" "$SOURCE_API/api/v1/onboarding/complete" >"$WORK/source-onboarding-result.json"
curl -kfsS -X POST -H 'Content-Type: application/json' -d '{"reauthenticated":true}' "$SOURCE_API/api/v1/storage/safety/unlock" >"$WORK/source-storage-unlock.json"

storage_plan_and_confirm() {
	local disk_id=$1
	local mount_path=$2
	local suffix=$3
	python3 - "$disk_id" "$mount_path" "$WORK/storage-plan-$suffix.json" <<'PY'
import json
import sys
disk_id, mount_path, output = sys.argv[1:]
json.dump({"action": "filesystem.create", "diskId": disk_id, "requestedState": {"filesystem": "ext4", "mountPath": mount_path, "label": "lumonas"}}, open(output, "w", encoding="utf-8"))
PY
	curl -kfsS -X POST -H 'Content-Type: application/json' --data-binary @"$WORK/storage-plan-$suffix.json" "$SOURCE_API/api/v1/storage/operations/plan" >"$WORK/storage-plan-result-$suffix.json"
	python3 - "$WORK/storage-plan-result-$suffix.json" "$WORK/storage-confirm-$suffix.json" <<'PY'
import json
import sys
plan = json.load(open(sys.argv[1], encoding="utf-8"))
json.dump({"planHash": plan["planHash"], "reauthenticated": True, "storageSafetyUnlocked": True}, open(sys.argv[2], "w", encoding="utf-8"))
PY
	local operation_id
	operation_id=$(python3 - "$WORK/storage-plan-result-$suffix.json" <<'PY'
import json
import sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["operationId"])
PY
)
	curl -kfsS -X POST -H 'Content-Type: application/json' --data-binary @"$WORK/storage-confirm-$suffix.json" "$SOURCE_API/api/v1/storage/operations/$operation_id/confirm" >"$WORK/storage-confirm-result-$suffix.json"
}

python3 - "$WORK/source-layout.json" <<'PY' >"$WORK/source-storage-ids"
import json
import sys
layout = json.load(open(sys.argv[1], encoding="utf-8"))
for disk_id in layout["all"]:
    print(disk_id)
PY
storage_index=0
while IFS= read -r disk_id; do
	storage_index=$((storage_index + 1))
	mount_path=$(python3 - "$WORK/source-layout.json" "$disk_id" <<'PY'
import json
import sys
layout = json.load(open(sys.argv[1], encoding="utf-8"))
print(layout["branches"][sys.argv[2]])
PY
)
	storage_plan_and_confirm "$disk_id" "$mount_path" "$storage_index"
done <"$WORK/source-storage-ids"

python3 - "$WORK/source-layout.json" "$WORK/source-pool-plan.json" <<'PY'
import json
import sys
layout = json.load(open(sys.argv[1], encoding="utf-8"))
json.dump({"name": "media", "diskIds": layout["data"]}, open(sys.argv[2], "w", encoding="utf-8"))
PY
curl -kfsS -X POST -H 'Content-Type: application/json' --data-binary @"$WORK/source-pool-plan.json" "$SOURCE_API/api/v1/storage/pools/plan" >"$WORK/source-pool-result.json"
python3 - "$WORK/source-pool-result.json" "$WORK/source-pool-confirm.json" <<'PY'
import json
import sys
plan = json.load(open(sys.argv[1], encoding="utf-8"))
json.dump({"planHash": plan["planHash"], "reauthenticated": True, "storageSafetyUnlocked": True}, open(sys.argv[2], "w", encoding="utf-8"))
PY
pool_operation=$(python3 - "$WORK/source-pool-result.json" <<'PY'
import json
import sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["operationId"])
PY
)
curl -kfsS -X POST -H 'Content-Type: application/json' --data-binary @"$WORK/source-pool-confirm.json" "$SOURCE_API/api/v1/storage/pools/$pool_operation/confirm" >"$WORK/source-pool-confirm-result.json"
curl -kfsS -X PUT -H 'Content-Type: application/json' --data-binary @"$WORK/source-protection.json" "$SOURCE_API/api/v1/storage/protection/config" >"$WORK/source-protection-result.json"
curl -kfsS "$SOURCE_API/api/v1/storage/mounts" >"$WORK/source-mounts.json"
curl -kfsS "$SOURCE_API/api/v1/pools" >"$WORK/source-pools.json"
curl -kfsS -X POST -H 'Content-Type: application/json' -d '{"id":"lan","uuid":"11111111-1111-1111-1111-111111111111","name":"LAN","interface":"eth0","enabled":true,"type":"ethernet","ipv4":{"method":"auto"},"ipv6":{"method":"disabled"},"reauthenticated":true}' "$SOURCE_API/api/v1/network/connections" >"$WORK/source-network.json"
curl -kfsS "$SOURCE_API/api/v1/network/connections" >"$WORK/source-networks.json"
curl -kfsS -X POST -H 'Content-Type: application/json' -d '{"name":"operator","password":"operator-password-123","managementRole":"admin"}' "$SOURCE_API/api/v1/users" >"$WORK/source-user.json"
SOURCE_COOKIES="$WORK/source-cookies.txt"
curl -kfsS -c "$SOURCE_COOKIES" -X POST -H 'Content-Type: application/json' -d '{"username":"operator","password":"operator-password-123"}' "$SOURCE_API/api/v1/auth/login" >"$WORK/source-login.json"
SOURCE_CSRF=$(python3 -c 'import json,sys; value=json.load(open(sys.argv[1],encoding="utf-8")).get("csrfToken","").strip(); assert value; print(value)' "$WORK/source-login.json")
curl -kfsS -b "$SOURCE_COOKIES" -H "X-CSRF-Token: $SOURCE_CSRF" -X POST -H 'Content-Type: application/json' -d '{"id":"share-media","name":"Media","path":"/srv/pools/media","enabled":true,"protocols":[{"protocol":"smb","enabled":true}],"access":[]}' "$SOURCE_API/api/v1/shares" >"$WORK/source-share.json"
curl -kfsS -b "$SOURCE_COOKIES" -H "X-CSRF-Token: $SOURCE_CSRF" -X POST -H 'Content-Type: application/json' --data-binary @- "$SOURCE_API/api/v1/docker/stacks" >"$WORK/source-stack.json" <<'JSON'
{"name":"media","composeYaml":"services:\n  media:\n    image: example/media:latest\n    volumes:\n      - /srv/lumonas/docker/appdata/media:/config\n"}
JSON
curl -kfsS -b "$SOURCE_COOKIES" -H "X-CSRF-Token: $SOURCE_CSRF" -X POST "$SOURCE_API/api/v1/recovery/export" >"$WORK/source-export.json"
if ! curl -kfsS -b "$SOURCE_COOKIES" -H "X-CSRF-Token: $SOURCE_CSRF" -X POST -H 'Content-Type: application/json' -d '{"action":"poweroff","confirmed":true,"reauthenticated":true}' "$SOURCE_API/api/v1/power/shutdown" >/dev/null 2>&1; then
	echo "live recovery source shutdown request failed" >&2
	cat "$SOURCE_LOG" >&2 || true
	exit 1
fi
for attempt in $(seq 1 60); do
	if ! kill -0 "$SOURCE_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
if kill -0 "$SOURCE_PID" 2>/dev/null; then
	echo "live recovery source appliance did not power off cleanly" >&2
	cat "$SOURCE_LOG" >&2 || true
	exit 1
fi
wait "$SOURCE_PID"
SOURCE_PID=""

mount -o loop,ro "$SOURCE_RAW" "$SOURCE_MOUNT"
cp "$SOURCE_MOUNT/var/lib/lumonas/recovery/latest.mrb" "$WORK/latest.mrb"
cmp -s "$WORK/recovery.key" "$SOURCE_MOUNT/var/lib/lumonas/recovery/recovery.key"
umount "$SOURCE_MOUNT"
echo "Live source appliance recovery bundle exported: $WORK/latest.mrb"
