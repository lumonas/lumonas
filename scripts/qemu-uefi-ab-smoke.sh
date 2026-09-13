#!/bin/sh
# Release-blocking UEFI A/B smoke. The BIOS smoke deliberately proves that
# BootNext fails closed without EFI variables; this harness proves the other
# side of the boundary: a GPT image boots through OVMF, the typed broker
# writes the inactive disk, BootNext survives a real reboot, and the kernel
# comes back from that disk.
set -eu

ASSERT_MODE="${LUMONAS_UEFI_AB_ASSERT:-false}"
if [ "$ASSERT_MODE" != "true" ]; then
	echo "Set LUMONAS_UEFI_AB_ASSERT=true with LUMONAS_QEMU_IMAGE to run the UEFI A/B smoke" >&2
	exit 0
fi

IMAGE="${LUMONAS_QEMU_IMAGE:-}"
[ -f "$IMAGE" ] || { echo "LUMONAS_QEMU_IMAGE is required" >&2; exit 1; }
KEY="${LUMONAS_QEMU_SSH_KEY:-}"
[ -s "$KEY" ] || { echo "LUMONAS_QEMU_SSH_KEY is required" >&2; exit 1; }
for command in qemu-system-x86_64 qemu-img ssh curl python3; do
	command -v "$command" >/dev/null 2>&1 || { echo "$command is required" >&2; exit 1; }
done

OVMF_CODE="${LUMONAS_OVMF_CODE:-}"
OVMF_VARS_TEMPLATE="${LUMONAS_OVMF_VARS:-}"
if [ -z "$OVMF_CODE" ]; then
	for candidate in /usr/share/OVMF/OVMF_CODE_4M.fd /usr/share/OVMF/OVMF_CODE.fd; do
		if [ -f "$candidate" ]; then OVMF_CODE="$candidate"; break; fi
	done
fi
if [ -z "$OVMF_VARS_TEMPLATE" ]; then
	for candidate in /usr/share/OVMF/OVMF_VARS_4M.fd /usr/share/OVMF/OVMF_VARS.fd; do
		if [ -f "$candidate" ]; then OVMF_VARS_TEMPLATE="$candidate"; break; fi
	done
fi
[ -f "$OVMF_CODE" ] || { echo "OVMF_CODE firmware is required" >&2; exit 1; }
[ -f "$OVMF_VARS_TEMPLATE" ] || { echo "OVMF_VARS firmware template is required" >&2; exit 1; }

DATA_DIR="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-uefi-ab.XXXXXX")"
SSH_PORT="${LUMONAS_UEFI_AB_SSH_PORT:-18024}"
WEB_PORT="${LUMONAS_UEFI_AB_WEB_PORT:-18025}"
QEMU_PID=""
cleanup() {
	status=$?
	set +e
	if [ -n "$QEMU_PID" ]; then
		kill "$QEMU_PID" 2>/dev/null || true
		wait "$QEMU_PID" 2>/dev/null || true
	fi
	rm -rf "$DATA_DIR"
	exit "$status"
}
trap cleanup EXIT INT TERM

# Keep the inactive slot a real disk image, not a blank scratch disk. The
# guest then proves that its EFI entry points at a bootable GPT disk after the
# broker rewrites it.
qemu-img convert -O qcow2 "$IMAGE" "$DATA_DIR/slot-b.qcow2"
qemu-img create -f qcow2 "$DATA_DIR/stage.qcow2" 5G >/dev/null
cp "$OVMF_VARS_TEMPLATE" "$DATA_DIR/OVMF_VARS.fd"

start_guest() {
	qemu-system-x86_64 \
		-machine q35,accel=tcg \
		-m 2048 \
		-smp 2 \
		-drive "if=pflash,format=raw,readonly=on,file=$OVMF_CODE" \
		-drive "if=pflash,format=raw,file=$DATA_DIR/OVMF_VARS.fd" \
		-drive "file=$IMAGE,if=virtio,format=raw,serial=LUMONAS-SLOTA" \
		-drive "file=$DATA_DIR/slot-b.qcow2,if=virtio,format=qcow2,serial=LUMONAS-SLOTB" \
		-drive "file=$DATA_DIR/stage.qcow2,if=virtio,format=qcow2,serial=LUMONAS-STAGE" \
		-netdev user,id=n1,restrict=on,hostfwd=tcp::"$SSH_PORT"-:22,hostfwd=tcp::"$WEB_PORT"-:8081 \
		-device virtio-net-pci,netdev=n1 \
		-nographic \
		-serial mon:stdio >"$DATA_DIR/qemu.log" 2>&1 &
	QEMU_PID=$!
}

ssh_guest() {
	ssh -i "$KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=3 \
		-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@127.0.0.1 "$@"
}

start_guest
for attempt in $(seq 1 90); do
	if curl -kfsS "https://127.0.0.1:$WEB_PORT/healthz" >/dev/null 2>&1 && ssh_guest true >/dev/null 2>&1; then
		break
	fi
	if [ "$attempt" = 90 ]; then
		echo "UEFI guest never became ready" >&2
		cat "$DATA_DIR/qemu.log" >&2 || true
		exit 1
	fi
	sleep 2
done

ssh_guest 'set -eu
SLOT_B_DEVICE=/dev/disk/by-id/virtio-LUMONAS-SLOTB
SLOT_B_ROOT="${SLOT_B_DEVICE}-part3"
[ -b "$SLOT_B_DEVICE" ] && [ -b "$SLOT_B_ROOT" ]
export SLOT_B_DEVICE SLOT_B_ROOT
test "$(findmnt -n -o SOURCE /)" = /dev/vda3
test "$(findmnt -n -o FSTYPE /boot/efi)" = vfat
efibootmgr -c -d /dev/vda -p 2 -L LumoNAS-A -l "\EFI\BOOT\BOOTX64.EFI" >/dev/null
# Give slot B a distinct GPT disk/partition identity before registering its
# EFI entry. The firmware device path embeds that identity.
sgdisk -G "$SLOT_B_DEVICE" >/dev/null
partx --update "$SLOT_B_DEVICE" >/dev/null
efibootmgr -c -d "$SLOT_B_DEVICE" -p 2 -L LumoNAS-B -l "\EFI\BOOT\BOOTX64.EFI" >/dev/null
sed -n "s/^Boot\\([0-9A-Fa-f]\\{4\\}\\).*LumoNAS-B.*/\\1/p" "$(efibootmgr)" | head -n1 >/run/lumonas-uefi-ab-entry
test -s /run/lumonas-uefi-ab-entry
old_uuid=$(blkid -s UUID -o value "$SLOT_B_ROOT")
umount "$SLOT_B_ROOT" 2>/dev/null || true
tune2fs -U random "$SLOT_B_ROOT" >/dev/null
new_uuid=$(blkid -s UUID -o value "$SLOT_B_ROOT")
test -n "$new_uuid" && test "$new_uuid" != "$old_uuid"
mkdir -p /mnt/lumonas-slot-b
mount "$SLOT_B_ROOT" /mnt/lumonas-slot-b
sed -i "s/$old_uuid/$new_uuid/g" /mnt/lumonas-slot-b/etc/fstab /mnt/lumonas-slot-b/boot/grub/grub.cfg
printf "uefi-slot-b\\n" >/mnt/lumonas-slot-b/etc/lumonas/uefi-slot-marker
sync
umount /mnt/lumonas-slot-b
mkfs.ext4 -F /dev/vdc >/dev/null
mount /dev/vdc /var/lib/lumonas/updates
mkdir -p /var/lib/lumonas/updates/slot-b
dd if="$SLOT_B_DEVICE" of=/var/lib/lumonas/updates/slot-b/image bs=16M status=none
sync
test "$(findmnt -n -o SOURCE /)" = /dev/vda3
'

ssh_guest 'set -eu
SLOT_B_DEVICE=/dev/disk/by-id/virtio-LUMONAS-SLOTB
[ -b "$SLOT_B_DEVICE" ]
export SLOT_B_DEVICE
python3 - <<PY
import hashlib, json, os, socket
path = "/var/lib/lumonas/updates/slot-b/image"
digest = hashlib.sha256(open(path, "rb").read()).hexdigest()
request = {
    "operation": "system.slot.write",
    "operationId": "uefi-ab-write",
    "planHash": digest,
    "confirmed": True,
    "requestedState": {"imagePath": path, "targetDevice": os.environ["SLOT_B_DEVICE"], "expectedDigest": digest},
}
client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect("/run/lumonas/storage.sock")
client.sendall(json.dumps(request).encode())
response = json.loads(client.recv(65536))
assert response.get("ok") is True, response
assert response["data"]["digestSha256"] == digest, response
entry = open("/run/lumonas-uefi-ab-entry").read().strip()
request = {
    "operation": "system.slot.bootnext",
    "operationId": "uefi-ab-write",
    "planHash": digest,
    "confirmed": True,
    "requestedState": {"entry": entry},
}
client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect("/run/lumonas/storage.sock")
client.sendall(json.dumps(request).encode())
response = json.loads(client.recv(65536))
assert response.get("ok") is True, response
PY
efibootmgr | grep -F "BootNext: $(cat /run/lumonas-uefi-ab-entry)" >/dev/null
systemctl reboot' || true

# The firmware reboot is expected to drop SSH. Wait for the same guest to
# return and prove that the root filesystem came from the inactive disk.
for attempt in $(seq 1 120); do
	if curl -kfsS "https://127.0.0.1:$WEB_PORT/healthz" >/dev/null 2>&1 && \
		ssh_guest 'test "$(findmnt -n -o SOURCE /)" = /dev/vdb3 && test -f /etc/lumonas/uefi-slot-marker' >/dev/null 2>&1; then
		echo "LumoNAS UEFI A/B boot flip passed"
		exit 0
	fi
	if ! kill -0 "$QEMU_PID" 2>/dev/null; then
		echo "QEMU exited during UEFI BootNext reboot" >&2
		cat "$DATA_DIR/qemu.log" >&2 || true
		exit 1
	fi
	sleep 2
done
echo "UEFI guest did not return from BootNext reboot" >&2
cat "$DATA_DIR/qemu.log" >&2 || true
exit 1
