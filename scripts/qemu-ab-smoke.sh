#!/bin/sh
# A/B slot smoke: boots the installed appliance image and exercises the
# slot-image pipeline end to end — stage, privileged write to a spare disk,
# digest verification, BootNext fail-closed behavior on BIOS guests, and the
# confirm conflict path. The full UEFI BootNext boot-flip requires UEFI QEMU
# firmware and is documented in docs/21 as the next acceptance step.
set -eu

ASSERT_MODE="${LUMONAS_AB_ASSERT:-false}"

if [ "$ASSERT_MODE" != "true" ]; then
	echo "Set LUMONAS_AB_ASSERT=true with LUMONAS_QEMU_IMAGE to run the A/B slot smoke" >&2
	exit 0
fi
command -v qemu-system-x86_64 >/dev/null 2>&1 || { echo "qemu-system-x86_64 is required" >&2; exit 1; }
command -v qemu-img >/dev/null 2>&1 || { echo "qemu-img is required" >&2; exit 1; }
[ -n "${LUMONAS_QEMU_IMAGE:-}" ] && [ -f "$LUMONAS_QEMU_IMAGE" ] || { echo "LUMONAS_QEMU_IMAGE is required" >&2; exit 1; }
[ -n "${LUMONAS_QEMU_SSH_KEY:-}" ] && [ -f "${LUMONAS_QEMU_SSH_KEY:-}" ] || { echo "LUMONAS_QEMU_SSH_KEY is required (existing qemu harness key)" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "python3 is required" >&2; exit 1; }
command -v ssh >/dev/null 2>&1 || { echo "ssh is required" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }

IMAGE_FORMAT="${LUMONAS_QEMU_IMAGE_FORMAT:-raw}"
DATA_DIR="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-ab.XXXXXX")"
trap 'rm -rf "$DATA_DIR"' EXIT INT TERM
qemu-img create -f qcow2 "$DATA_DIR/slot-b.qcow2" 1G >/dev/null

SSH_PORT="${LUMONAS_AB_SSH_PORT:-2223}"

start_guest() {
	qemu-system-x86_64 \
		-machine q35,accel=tcg \
		-m 2048 \
		-smp 2 \
		-drive "file=$LUMONAS_QEMU_IMAGE,if=virtio,format=$IMAGE_FORMAT" \
		-drive "file=$DATA_DIR/slot-b.qcow2,if=virtio,format=qcow2,serial=LUMONAS-SLOTB" \
		-netdev user,id=n1,restrict=on,hostfwd=tcp::"$SSH_PORT"-:22 \
		-device virtio-net-pci,netdev=n1 \
		-nographic \
		-serial mon:stdio >/dev/null 2>&1 &
}

ssh_guest() {
	ssh -i "$LUMONAS_QEMU_SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=3 \
		-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@127.0.0.1 "$@"
}

GUEST_PID=""
stop_guest() {
	if [ -n "$GUEST_PID" ]; then
		kill "$GUEST_PID" 2>/dev/null || true
		wait "$GUEST_PID" 2>/dev/null || true
	fi
}
cleanup() {
	stop_guest
	rm -rf "$DATA_DIR"
}
trap cleanup EXIT INT TERM

start_guest
GUEST_PID=$!

guest_ready=0
for _ in $(seq 1 90); do
	if ssh_guest true >/dev/null 2>&1; then
		guest_ready=1
		break
	fi
	sleep 2
done
[ "$guest_ready" = "1" ] || { echo "guest never became reachable over SSH" >&2; exit 1; }

# 1. Stage a fixture slot image inside the guest with a correct manifest.
ssh_guest 'set -eu
mkdir -p /var/lib/lumonas/updates/slot-b
printf "ab-slot-image-payload" > /var/lib/lumonas/updates/slot-b/image
python3 - <<PY
import hashlib, json, os, time
path = "/var/lib/lumonas/updates/slot-b/image"
data = open(path, "rb").read()
manifest = {
    "formatVersion": 1,
    "version": "0.0.0-ab-smoke",
    "packageSha256": hashlib.sha256(data).hexdigest(),
    "packageSize": len(data),
    "publishedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
}
open("/var/lib/lumonas/updates/slot-b/image-manifest.json", "w").write(json.dumps(manifest))
PY'

# 2. Privileged write to the spare disk through the storage worker socket.
ssh_guest 'set -eu
python3 - <<PY
import hashlib, json, socket
path = "/var/lib/lumonas/updates/slot-b/image"
digest = hashlib.sha256(open(path, "rb").read()).hexdigest()
request = {
    "operation": "system.slot.write",
    "operationId": "ab-smoke-write",
    "planHash": digest,
    "confirmed": True,
    "requestedState": {"imagePath": path, "targetDevice": "/dev/vdb", "expectedDigest": digest},
}
client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect("/run/lumonas/storage.sock")
client.sendall(json.dumps(request).encode())
response = json.loads(client.recv(65536))
assert response.get("ok") is True, response
assert response["data"]["digestSha256"] == digest, response
print("slot write ok")
PY'

# 3. Tampered staging must fail: rewrite the staged image with a bad digest.
ssh_guest 'set -eu
python3 - <<PY
import json, socket
request = {
    "operation": "system.slot.write",
    "operationId": "ab-smoke-tamper",
    "planHash": "bad",
    "confirmed": True,
    "requestedState": {
        "imagePath": "/var/lib/lumonas/updates/slot-b/image",
        "targetDevice": "/dev/vdb",
        "expectedDigest": "0" * 64,
    },
}
client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect("/run/lumonas/storage.sock")
client.sendall(json.dumps(request).encode())
response = json.loads(client.recv(65536))
assert response.get("ok") is not True, response
print("tamper rejected ok")
PY'

# 4. BootNext on a BIOS guest fails closed.
ssh_guest 'set -eu
python3 - <<PY
import json, socket
request = {
    "operation": "system.slot.bootnext",
    "operationId": "ab-smoke-bootnext",
    "planHash": "boot",
    "confirmed": True,
    "requestedState": {"entry": "0002"},
}
client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect("/run/lumonas/storage.sock")
client.sendall(json.dumps(request).encode())
response = json.loads(client.recv(65536))
assert response.get("ok") is not True, response
print("bootnext fail-closed ok")
PY'

# 5. Confirming without a healthy pending boot conflicts at the API layer.
STATUS="$(curl -kfsS https://127.0.0.1:18080/api/v1/updates/status 2>/dev/null || echo)"
[ -n "$STATUS" ] || { echo "updates status unavailable through the API" >&2; exit 1; }
CONFIRM_RESPONSE="$DATA_DIR/confirm.json"
CONFIRM_STATUS=$(curl -ksS -o "$CONFIRM_RESPONSE" -w '%{http_code}' -X POST https://127.0.0.1:18080/api/v1/updates/slot/confirm 2>/dev/null || true)
[ "$CONFIRM_STATUS" = "409" ] || {
	echo "slot confirm should fail closed without a pending slot (HTTP $CONFIRM_STATUS)" >&2
	cat "$CONFIRM_RESPONSE" >&2 || true
	exit 1
}

echo "LumoNAS A/B slot smoke passed"
