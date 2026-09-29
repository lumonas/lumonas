#!/bin/sh
set -eu

ASSERT_MODE="${LUMONAS_PRIVILEGED_STORAGE_ASSERT:-false}"
if [ "$(id -u)" -ne 0 ]; then
	if [ "$ASSERT_MODE" = "true" ]; then
		echo "root is required for privileged storage loopback assertions" >&2
		exit 1
	fi
	echo "privileged storage loopback test skipped: root is required" >&2
	exit 0
fi

for command in go lsblk losetup mount umount mkfs.ext4 wipefs findmnt blkid mktemp python3 truncate sfdisk partx; do
	command -v "$command" >/dev/null 2>&1 || {
		echo "$command is required for privileged storage loopback assertions" >&2
		exit 1
	}
done

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-privileged-storage.XXXXXX")"
LOOP=""
PARTITION_LOOP=""
UNSTABLE_LOOP=""
WORKER_PID=""
MOUNT_PATH=""
PARTITION_MOUNT_PATH=""
cleanup() {
	set +e
	if [ -n "$PARTITION_MOUNT_PATH" ]; then
		umount "$PARTITION_MOUNT_PATH" 2>/dev/null || true
		rmdir "$PARTITION_MOUNT_PATH" 2>/dev/null || true
	fi
	if [ -n "$MOUNT_PATH" ]; then
		umount "$MOUNT_PATH" 2>/dev/null || true
		rmdir "$MOUNT_PATH" 2>/dev/null || true
	fi
	if [ -n "$WORKER_PID" ]; then
		kill "$WORKER_PID" 2>/dev/null || true
		wait "$WORKER_PID" 2>/dev/null || true
	fi
	if [ -n "$LOOP" ]; then
		losetup -d "$LOOP" 2>/dev/null || true
	fi
	if [ -n "$PARTITION_LOOP" ]; then
		losetup -d "$PARTITION_LOOP" 2>/dev/null || true
	fi
	if [ -n "$UNSTABLE_LOOP" ]; then
		losetup -d "$UNSTABLE_LOOP" 2>/dev/null || true
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

SOCKET="$WORK/storage.sock"
IMAGE="$WORK/storage.img"
PARTITION_IMAGE="$WORK/partitioned-storage.img"
WORKER_LOG="$WORK/privd-storage.log"
truncate -s 96M "$IMAGE"
LOOP="$(losetup --find --show "$IMAGE")"
# A disposable loop device has no serial or WWN. Give the main fixture a GPT
# disk GUID so the broker exercises the same stable-identity path as a real
# appliance disk instead of relying on /dev/loopN.
printf 'label: gpt\n' | sfdisk --no-reread "$LOOP" >/dev/null
truncate -s 128M "$PARTITION_IMAGE"
PARTITION_LOOP="$(losetup --find --show --partscan "$PARTITION_IMAGE")"
printf 'label: gpt\n,96M,L\n' | sfdisk --no-reread "$PARTITION_LOOP" >/dev/null
partx --update "$PARTITION_LOOP" 2>/dev/null || true
partx --add "$PARTITION_LOOP" 2>/dev/null || true
PARTITION_DEVICE="${PARTITION_LOOP}p1"
for attempt in $(seq 1 10); do
	[ -b "$PARTITION_DEVICE" ] && break
	sleep 1
done
[ -b "$PARTITION_DEVICE" ] || { echo "partition device did not appear: $PARTITION_DEVICE" >&2; exit 1; }
PARTITION_MOUNT_PATH="$WORK/partition-mount"
mkdir -p "$PARTITION_MOUNT_PATH"
mkfs.ext4 -F "$PARTITION_DEVICE" >/dev/null
mount "$PARTITION_DEVICE" "$PARTITION_MOUNT_PATH"

PRIVD_BIN="${LUMONAS_PRIVD_BIN:-$WORK/lumonas-privd}"
if [ -z "${LUMONAS_PRIVD_BIN:-}" ]; then
	GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" \
		go build -trimpath -o "$PRIVD_BIN" "$ROOT/cmd/lumonas-privd"
fi
[ -x "$PRIVD_BIN" ] || { echo "lumonas-privd binary is not executable: $PRIVD_BIN" >&2; exit 1; }
"$PRIVD_BIN" -worker storage -socket "$SOCKET" >"$WORKER_LOG" 2>&1 &
WORKER_PID=$!
for attempt in $(seq 1 30); do
	[ -S "$SOCKET" ] && break
	sleep 1
done
[ -S "$SOCKET" ] || { echo "storage worker socket did not appear" >&2; cat "$WORKER_LOG" >&2 || true; exit 1; }

discover_identity() {
	loop_device=$1
	state_file=$2
	lsblk -J -b -o NAME,PATH,TYPE,SIZE,MODEL,SERIAL,WWN,UUID,PARTUUID,PTUUID "$loop_device" >"$WORK/disks.json"
	python3 - "$WORK/disks.json" "$loop_device" "$state_file" <<'PY'
import json
import sys

source, loop, target = sys.argv[1:]
devices = json.load(open(source, encoding="utf-8")).get("blockdevices", [])
# A loopback attachment is reported by lsblk as type "loop" until it carries a
# partition table, and as type "disk" once one is written. Accept either so the
# same helper serves both the partitioned and unpartitioned fixtures.
disk = next(
    (item for item in devices if item.get("type") in ("disk", "loop") and item.get("path") == loop),
    None,
)
if disk is None:
    raise SystemExit(f"loop device was not returned by lsblk: {loop}")
wwn = str(disk.get("wwn") or "").strip()
serial = str(disk.get("serial") or "").strip()
ptuuid = str(disk.get("ptuuid") or "").strip()
uuid = str(disk.get("uuid") or "").strip()
identity = {
    "id": "wwn:" + wwn if wwn else "serial:" + serial if serial else "gpt:" + ptuuid if ptuuid else "uuid:" + uuid if uuid else "path:" + loop,
    "sizeBytes": int(disk.get("size") or 0),
    "serial": serial,
    "wwn": wwn,
    "gptDiskGuid": ptuuid,
    "partitionUuid": str(disk.get("partuuid") or "").strip(),
    "filesystemUuid": uuid,
}
if identity["sizeBytes"] <= 0:
    raise SystemExit("loop device has no capacity")
json.dump(identity, open(target, "w", encoding="utf-8"), sort_keys=True)
PY
}

send_request() {
	operation=$1
	operation_id=$2
	identity_file=$3
	requested_json=$4
	response_file=$5
	expires_at=${6:-}
	python3 - "$SOCKET" "$operation" "$operation_id" "$identity_file" "$requested_json" "$response_file" "$expires_at" <<'PY'
import datetime
import json
import socket
import sys

socket_path, operation, operation_id, identity_path, requested_json, response_path, expires_at = sys.argv[1:]
identity = json.load(open(identity_path, encoding="utf-8"))
expected = {key: value for key, value in identity.items() if key != "id" and value not in ("", 0)}
requested = json.loads(requested_json)
if not expires_at:
    expires_at = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(minutes=10)).isoformat().replace("+00:00", "Z")
request = {
    "operation": operation,
    "operationId": operation_id,
    "planHash": operation_id or "loopback-plan",
    "targetDiskId": identity["id"],
    "expectedIdentity": {key: str(value) for key, value in expected.items()},
    "requestedState": requested,
    "expiresAt": expires_at,
    "confirmed": True,
}
connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
connection.settimeout(15)
connection.connect(socket_path)
connection.sendall((json.dumps(request) + "\n").encode())
chunks = []
while True:
    chunk = connection.recv(65536)
    if not chunk:
        break
    chunks.append(chunk)
    if b"\n" in chunk:
        break
connection.close()
payload = b"".join(chunks).splitlines()[0]
json.dump(json.loads(payload.decode()), open(response_path, "w", encoding="utf-8"), sort_keys=True)
PY
}

assert_ok() {
	response=$1
	python3 - "$response" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
if not payload.get("ok"):
    raise SystemExit(payload.get("error", "privileged operation failed"))
PY
}

assert_error() {
	response=$1
	needle=$2
	python3 - "$response" "$needle" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
if payload.get("ok") or sys.argv[2] not in payload.get("error", ""):
    raise SystemExit(f"expected rejected operation containing {sys.argv[2]!r}: {payload}")
PY
}

UNSTABLE_IMAGE="$WORK/unstable-identity.img"
truncate -s 64M "$UNSTABLE_IMAGE"
UNSTABLE_LOOP="$(losetup --find --show "$UNSTABLE_IMAGE")"
discover_identity "$UNSTABLE_LOOP" "$WORK/unstable-identity.json"
UNSTABLE_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$WORK/unstable-identity.json")"
case "$UNSTABLE_ID" in
	path:*) ;;
	*) echo "unpartitioned loop fixture unexpectedly has a stable identity: $UNSTABLE_ID" >&2; exit 1 ;;
esac
send_request filesystem.format unstable-path "$WORK/unstable-identity.json" '{"filesystem":"ext4"}' "$WORK/unstable-format.json"
assert_error "$WORK/unstable-format.json" "no stable identity"

discover_identity "$LOOP" "$WORK/identity-before-format.json"
DISK_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$WORK/identity-before-format.json")"
DISK_SIZE="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["sizeBytes"])' "$WORK/identity-before-format.json")"
[ -n "$DISK_ID" ] && [ "$DISK_SIZE" -gt 0 ]

send_request filesystem.format loop-format "$WORK/identity-before-format.json" '{"filesystem":"ext4"}' "$WORK/format.json"
assert_ok "$WORK/format.json"

discover_identity "$LOOP" "$WORK/identity-after-format.json"
mkdir -p /srv/disks
MOUNT_PATH="$(mktemp -d /srv/disks/lumonas-privileged-loopback.XXXXXX)"
MOUNT_REQUEST="$(python3 -c 'import json,sys; print(json.dumps({"filesystem":"ext4","mountPath":sys.argv[1],"readOnly":True}))' "$MOUNT_PATH")"
send_request filesystem.mount loop-mount "$WORK/identity-after-format.json" "$MOUNT_REQUEST" "$WORK/mount.json"
assert_ok "$WORK/mount.json"
if touch "$MOUNT_PATH/must-remain-read-only" 2>/dev/null; then
	echo "privileged read-only mount unexpectedly accepted a write" >&2
	exit 1
fi

send_request disk.erase loop-mounted-erase "$WORK/identity-after-format.json" '{}' "$WORK/mounted-erase.json"
assert_error "$WORK/mounted-erase.json" "mounted"
UNMOUNT_REQUEST="$(python3 -c 'import json,sys; print(json.dumps({"mountPath":sys.argv[1]}))' "$MOUNT_PATH")"
send_request filesystem.unmount loop-unmount "$WORK/identity-after-format.json" "$UNMOUNT_REQUEST" "$WORK/unmount.json"
assert_ok "$WORK/unmount.json"

python3 - "$WORK/identity-after-format.json" "$WORK/identity-stale.json" <<'PY'
import json
import sys
identity = json.load(open(sys.argv[1], encoding="utf-8"))
identity["serial"] = "stale-loopback-identity"
json.dump(identity, open(sys.argv[2], "w", encoding="utf-8"), sort_keys=True)
PY
send_request disk.erase loop-stale "$WORK/identity-stale.json" '{}' "$WORK/stale.json"
assert_error "$WORK/stale.json" "identity mismatch"

discover_identity "$PARTITION_LOOP" "$WORK/partition-identity.json"
send_request disk.erase partition-mounted "$WORK/partition-identity.json" '{}' "$WORK/partition-mounted.json"
assert_error "$WORK/partition-mounted.json" "mounted"

send_request disk.erase '' "$WORK/identity-after-format.json" '{}' "$WORK/missing-operation-id.json"
assert_error "$WORK/missing-operation-id.json" "operationId is required"
send_request disk.erase loop-expired "$WORK/identity-after-format.json" '{}' "$WORK/expired.json" "2000-01-01T00:00:00Z"
assert_error "$WORK/expired.json" "expired"

send_request disk.erase loop-erase "$WORK/identity-after-format.json" '{}' "$WORK/erase.json"
assert_ok "$WORK/erase.json"
if blkid "$LOOP" >/dev/null 2>&1; then
	echo "privileged erase left a filesystem signature" >&2
	exit 1
fi

echo "privileged storage loopback smoke passed (format, read-only mount, mounted-disk rejection, mounted-partition rejection, stale identity, missing operation ID, expiry, erase)"
