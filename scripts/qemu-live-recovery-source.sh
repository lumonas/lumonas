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
	-netdev user,id=n1,hostfwd=tcp::18083-:8081 \
	-device virtio-net-pci,netdev=n1 \
	-nographic \
	-serial mon:stdio \
	-no-reboot >"$SOURCE_LOG" 2>&1 &
SOURCE_PID=$!

source_ready=false
for attempt in $(seq 1 120); do
	if curl -fsS http://127.0.0.1:18083/healthz >/dev/null 2>&1 && \
		curl -fsS http://127.0.0.1:18083/readyz >/dev/null 2>&1 && \
		curl -fsS http://127.0.0.1:18083/api/v1/server >/dev/null 2>&1; then
		source_ready=true
		break
	fi
	if ! kill -0 "$SOURCE_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
[ "$source_ready" = true ] || { echo "live recovery source appliance never became ready" >&2; cat "$SOURCE_LOG" >&2 || true; exit 1; }

curl -fsS -X POST http://127.0.0.1:18083/api/v1/recovery/key >"$WORK/source-key.json"
python3 -c 'import json,sys; value=json.load(open(sys.argv[1],encoding="utf-8")).get("key","").strip(); assert value; open(sys.argv[2],"w",encoding="utf-8").write(value+"\n")' "$WORK/source-key.json" "$WORK/recovery.key"
curl -fsS http://127.0.0.1:18083/api/v1/disks >"$WORK/source-disks.json"
python3 -c 'import json,sys; disks=json.load(open(sys.argv[1],encoding="utf-8")); roles={d["id"]:"data" for d in disks if "DATA" in d.get("id","").upper()}; assert len(roles)>=2; json.dump({"serverName":"source-recovery-nas","roles":roles,"protection":{"syncTime":"","scrubDay":""},"recovery":{"autoConfigBackup":False,"keyAcknowledged":True}},open(sys.argv[2],"w",encoding="utf-8"))' "$WORK/source-disks.json" "$WORK/source-onboarding.json"
curl -fsS -X POST -H 'Content-Type: application/json' --data-binary @"$WORK/source-onboarding.json" http://127.0.0.1:18083/api/v1/onboarding/complete >"$WORK/source-onboarding-result.json"
curl -fsS -X POST -H 'Content-Type: application/json' -d '{"name":"operator","password":"operator-password-123","managementRole":"admin"}' http://127.0.0.1:18083/api/v1/users >"$WORK/source-user.json"
curl -fsS -X POST -H 'Content-Type: application/json' -d '{"id":"share-media","name":"Media","path":"/srv/pools/media","enabled":true,"protocols":[{"protocol":"smb","enabled":true}],"access":[]}' http://127.0.0.1:18083/api/v1/shares >"$WORK/source-share.json"
curl -fsS -X POST -H 'Content-Type: application/json' --data-binary @- http://127.0.0.1:18083/api/v1/docker/stacks >"$WORK/source-stack.json" <<'JSON'
{"name":"media","composeYaml":"services:\n  media:\n    image: example/media:latest\n    volumes:\n      - /srv/lumonas/docker/appdata/media:/config\n"}
JSON
curl -fsS -X POST http://127.0.0.1:18083/api/v1/recovery/export >"$WORK/source-export.json"
curl -fsS -X POST -H 'Content-Type: application/json' -d '{"action":"poweroff","confirmed":true,"reauthenticated":true}' http://127.0.0.1:18083/api/v1/power/shutdown >/dev/null 2>&1 || true
for attempt in $(seq 1 60); do
	if ! kill -0 "$SOURCE_PID" 2>/dev/null; then
		break
	fi
	sleep 2
done
if kill -0 "$SOURCE_PID" 2>/dev/null; then
	kill "$SOURCE_PID" 2>/dev/null || true
	wait "$SOURCE_PID" 2>/dev/null || true
fi
SOURCE_PID=""

mount -o loop,ro "$SOURCE_RAW" "$SOURCE_MOUNT"
cp "$SOURCE_MOUNT/var/lib/lumonas/recovery/latest.mrb" "$WORK/latest.mrb"
cmp -s "$WORK/recovery.key" "$SOURCE_MOUNT/var/lib/lumonas/recovery/recovery.key"
umount "$SOURCE_MOUNT"
echo "Live source appliance recovery bundle exported: $WORK/latest.mrb"
