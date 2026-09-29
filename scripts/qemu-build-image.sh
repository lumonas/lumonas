#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUTPUT="${LUMONAS_QEMU_IMAGE:-$ROOT/build/qemu/lumonas-debian13.raw}"
WORK="${LUMONAS_QEMU_WORKDIR:-$ROOT/build/qemu/work}"
DEB="${LUMONAS_DEB:-$ROOT/lumonas_${LUMONAS_VERSION:-0.1.0-dev}_amd64.deb}"
UPDATE_FIXTURE="${LUMONAS_UPDATE_FIXTURE:-}"
SSH_PUBLIC_KEY="${LUMONAS_QEMU_SSH_PUBLIC_KEY:-}"
SIZE="${LUMONAS_QEMU_DISK_SIZE:-4G}"
DEBIAN_MIRROR="${LUMONAS_DEBIAN_MIRROR:-https://snapshot.debian.org/archive/debian/20260201T000000Z/}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || printf '%s' 0)}"
SOURCE_COMMIT="${LUMONAS_SOURCE_COMMIT:-$(git -C "$ROOT" log -1 --format=%H 2>/dev/null || printf '%s' unknown)}"
case "$SOURCE_DATE_EPOCH" in
	''|*[!0-9]*) echo "SOURCE_DATE_EPOCH must be a non-negative integer" >&2; exit 1 ;;
esac
[ -n "$SOURCE_COMMIT" ] || { echo "LUMONAS_SOURCE_COMMIT must not be empty" >&2; exit 1; }
[ -n "$DEBIAN_MIRROR" ] || { echo "LUMONAS_DEBIAN_MIRROR must not be empty" >&2; exit 1; }
case "$DEBIAN_MIRROR" in
	https://*) ;;
	*) echo "LUMONAS_DEBIAN_MIRROR must use HTTPS" >&2; exit 1 ;;
esac
export SOURCE_DATE_EPOCH

for command in debootstrap qemu-img sfdisk losetup partx mkfs.ext4 mkfs.vfat blkid mount umount grub-install; do
  command -v "$command" >/dev/null 2>&1 || { echo "$command is required" >&2; exit 1; }
done
[ "$(id -u)" -eq 0 ] || { echo "Run this builder as root (for example: sudo $0)" >&2; exit 1; }
[ -f "$DEB" ] || { echo "Build the Debian package first: $DEB" >&2; exit 1; }
[ "$(dpkg-deb -f "$DEB" Architecture)" = "amd64" ] || {
	echo "the x86-64 QEMU appliance requires an amd64 LumoNAS package" >&2
	exit 1
}
[ -z "$UPDATE_FIXTURE" ] || [ -f "$UPDATE_FIXTURE" ] || { echo "update fixture not found: $UPDATE_FIXTURE" >&2; exit 1; }
[ ! -e "$OUTPUT" ] || { echo "Refusing to overwrite existing image: $OUTPUT" >&2; exit 1; }
[ ! -e "$WORK" ] || { echo "Refusing to overwrite existing work directory: $WORK" >&2; exit 1; }

mkdir -p "$(dirname "$OUTPUT")" "$WORK/mnt"
qemu-img create -f raw "$OUTPUT" "$SIZE" >/dev/null
sfdisk "$OUTPUT" <<'PARTITIONS'
label: gpt
unit: sectors
first-lba: 2048
# The BIOS boot partition must be named by its full GPT type GUID. sfdisk
# rejects both the "bios_grub" name and the numeric type code 4 with
# "Failed to add #1 partition: Invalid argument", and then writes no partition
# table at all, so the image builds as an unpartitioned file and every later
# step fails. Reproduced with util-linux 2.41.
2048,4096,21686148-6449-6E6F-744E-656564454649
6144,262144,uefi
268288,,linux
PARTITIONS

# A GPT table that sfdisk half-wrote would otherwise surface much later as a
# confusing mount or grub-install failure, so confirm the layout took.
sfdisk --verify "$OUTPUT" >/dev/null

LOOP="$(losetup --find --show --partscan "$OUTPUT")"
case "$LOOP" in
	*[0-9]) EFI_PART="${LOOP}p2"; ROOT_PART="${LOOP}p3" ;;
	*) EFI_PART="${LOOP}2"; ROOT_PART="${LOOP}3" ;;
esac
partx -u "$LOOP"
mkfs.vfat -F 32 -n LUMONAS_EFI "$EFI_PART" >/dev/null
mkfs.ext4 -F -L LUMONAS_ROOT "$ROOT_PART" >/dev/null
mount "$ROOT_PART" "$WORK/mnt"
mkdir -p "$WORK/mnt/boot/efi"
mount "$EFI_PART" "$WORK/mnt/boot/efi"
ROOT_UUID="$(blkid -s UUID -o value "$ROOT_PART")"
EFI_UUID="$(blkid -s UUID -o value "$EFI_PART")"
[ -n "$ROOT_UUID" ] && [ -n "$EFI_UUID" ] || { echo "could not read generated partition UUIDs" >&2; exit 1; }

cleanup() {
	set +e
	umount "$WORK/mnt/boot/efi" 2>/dev/null || true
	umount -R "$WORK/mnt/dev" 2>/dev/null || true
	umount -R "$WORK/mnt/proc" 2>/dev/null || true
	umount -R "$WORK/mnt/sys" 2>/dev/null || true
	umount "$WORK/mnt" 2>/dev/null || true
	if [ -n "${LOOP:-}" ]; then
		partx --delete "$LOOP" 2>/dev/null || true
		losetup -d "$LOOP" 2>/dev/null || true
	fi
}
trap cleanup EXIT

debootstrap --arch=amd64 --variant=minbase trixie "$WORK/mnt" "$DEBIAN_MIRROR"
mount --rbind /dev "$WORK/mnt/dev"
mount --make-rslave "$WORK/mnt/dev"
mount -t proc proc "$WORK/mnt/proc"
mount --rbind /sys "$WORK/mnt/sys"
mount --make-rslave "$WORK/mnt/sys"
rm -f "$WORK/mnt/etc/resolv.conf"
cp /etc/resolv.conf "$WORK/mnt/etc/resolv.conf"
cp "$DEB" "$WORK/mnt/tmp/lumonas.deb"

if [ -n "$UPDATE_FIXTURE" ]; then
	command -v python3 >/dev/null 2>&1 || { echo "python3 is required when LUMONAS_UPDATE_FIXTURE is set" >&2; exit 1; }
	UPDATE_PACKAGE=$(python3 - "$UPDATE_FIXTURE" <<'PY'
import json
import sys

fixture = json.load(open(sys.argv[1], encoding="utf-8"))
path = fixture.get("packagePath", "")
if not path:
    raise SystemExit("update fixture packagePath is empty")
print(path)
PY
)
	[ -f "$UPDATE_PACKAGE" ] || { echo "update fixture package is missing: $UPDATE_PACKAGE" >&2; exit 1; }
	UPDATE_PUBLIC_KEY=$(python3 - "$UPDATE_FIXTURE" <<'PY'
import json
import sys

fixture = json.load(open(sys.argv[1], encoding="utf-8"))
key = fixture.get("publicKey", "")
if not key:
    raise SystemExit("update fixture publicKey is empty")
print(key)
PY
)
	mkdir -p "$WORK/mnt/var/lib/lumonas/update-fixture"
	cp "$UPDATE_PACKAGE" "$WORK/mnt/var/lib/lumonas/update-fixture/package"
	chmod 0640 "$WORK/mnt/var/lib/lumonas/update-fixture/package"
fi

chroot "$WORK/mnt" /usr/bin/env LUMONAS_SOURCE_COMMIT="$SOURCE_COMMIT" LUMONAS_SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" LUMONAS_DEBIAN_MIRROR="$DEBIAN_MIRROR" LUMONAS_QEMU_SSH_PUBLIC_KEY="$SSH_PUBLIC_KEY" LUMONAS_ROOT_UUID="$ROOT_UUID" LUMONAS_EFI_UUID="$EFI_UUID" LUMONAS_LOOP_DEVICE="$LOOP" /bin/sh -eux <<'EOF'
export DEBIAN_FRONTEND=noninteractive
cat >/etc/apt/apt.conf.d/99lumonas-snapshot <<'APT'
Acquire::Check-Valid-Until "false";
APT
apt-get update
# The appliance boots both UEFI and BIOS, and grub-install is run for each
# target below. grub-pc and grub-efi-amd64 declare a Conflicts relationship, so
# installing both in one transaction fails outright and the image is never
# built. grub-pc does not conflict with grub-efi-amd64-bin, which supplies the
# same /usr/lib/grub/x86_64-efi modules that grub-install --target=x86_64-efi
# needs. The unsigned build is the right one here: the UEFI smoke test boots
# OVMF_CODE.fd rather than OVMF_CODE.secboot.fd, so Secure Boot is not in play.
apt-get install -y --no-install-recommends \
  systemd systemd-sysv systemd-resolved linux-image-amd64 grub-pc grub-efi-amd64-bin dosfstools efibootmgr gdisk openssh-server curl ca-certificates openssl certbot \
  iproute2 util-linux smartmontools lm-sensors nut nut-client e2fsprogs xfsprogs cryptsetup mergerfs snapraid \
  network-manager docker.io docker-compose samba samba-common-bin samba-vfs-modules nfs-kernel-server rsync rclone vsftpd \
  avahi-daemon nftables
dpkg -i /tmp/lumonas.deb || apt-get -f install -y
rm -f /tmp/lumonas.deb
# Stop Debian's own update timers. The image is built from a pinned snapshot, and
# the package manifest written below is what the signed update path verifies a
# later system against. If apt-daily runs at boot it fetches the current archive
# instead of the pinned snapshot, so the running system stops matching the
# manifest that describes it, and it mutates the system underneath an update
# mechanism that has its own signing and review. It also competes with first
# boot for memory, which is enough on its own to have apt-get OOM-killed here.
# Mask by symlink: systemctl mask in a chroot is not reliable.
mkdir -p /etc/systemd/system
ln -sf /dev/null /etc/systemd/system/apt-daily.timer
ln -sf /dev/null /etc/systemd/system/apt-daily-upgrade.timer
ln -sf /dev/null /etc/systemd/system/unattended-upgrades.service
if [ -d /var/lib/lumonas/update-fixture ]; then
  chown -R lumonas:lumonas /var/lib/lumonas/update-fixture
  chmod 0640 /var/lib/lumonas/update-fixture/package
fi
mkdir -p /usr/share/doc/lumonas
{
  echo "formatVersion=1"
  echo "sourceCommit=$LUMONAS_SOURCE_COMMIT"
  echo "sourceDateEpoch=$LUMONAS_SOURCE_DATE_EPOCH"
  echo "debianMirror=$LUMONAS_DEBIAN_MIRROR"
  echo "packages:"
  dpkg-query -W -f='${Package}\t${Version}\n' | sort
} >/usr/share/doc/lumonas/qemu-package-manifest.txt
systemd-analyze verify /lib/systemd/system/lumonas-runtime.service /lib/systemd/system/lumonas-privd.service /lib/systemd/system/lumonas-privd-general.service /lib/systemd/system/lumonas-privd-acme.service /lib/systemd/system/lumonas-privd-network.service /lib/systemd/system/lumonas-privd-power.service /lib/systemd/system/lumonas-privd-storage.service /lib/systemd/system/lumonas-web.service /lib/systemd/system/lumonasd.service /lib/systemd/system/lumonas-jobs.target /lib/systemd/system/lumonas-services.target /lib/systemd/system/lumonas-storage.target
mkdir -p /etc/NetworkManager/system-connections /etc/systemd/system/lumonas-web.service.d
cat >/etc/NetworkManager/system-connections/qemu-ethernet.nmconnection <<'NETWORK'
[connection]
id=qemu-ethernet
type=ethernet
autoconnect=true
autoconnect-retries=0

[ipv4]
method=auto

[ipv6]
method=auto
NETWORK
chmod 600 /etc/NetworkManager/system-connections/qemu-ethernet.nmconnection
cat >/etc/systemd/system/lumonas-web.service.d/qemu.conf <<'DROPIN'
[Service]
Environment=LUMONAS_WEB_LISTEN=0.0.0.0:8081
DROPIN
cat >/etc/fstab <<FSTAB
UUID=$LUMONAS_ROOT_UUID / ext4 defaults 0 1
UUID=$LUMONAS_EFI_UUID /boot/efi vfat umask=0077 0 1
FSTAB
systemctl enable NetworkManager.service NetworkManager-wait-online.service systemd-resolved.service docker.service smbd.service avahi-daemon.service ssh.service lumonas-runtime.service lumonas-privd.service lumonas-privd-storage.service lumonas-privd-network.service lumonas-privd-power.service lumonas-privd-general.service lumonas-privd-acme.service lumonas-jobs.target lumonas-services.target lumonas-storage.target lumonasd.service lumonas-web.service || true
systemctl disable systemd-networkd.service systemd-networkd-wait-online.service || true
ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf
passwd -l root || true
if [ -n "$LUMONAS_QEMU_SSH_PUBLIC_KEY" ]; then
  install -d -o root -g root -m 0700 /root/.ssh
  printf '%s\n' "$LUMONAS_QEMU_SSH_PUBLIC_KEY" >/root/.ssh/authorized_keys
  chmod 0600 /root/.ssh/authorized_keys
fi
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
cat >/etc/default/grub <<'GRUB'
GRUB_CMDLINE_LINUX_DEFAULT="quiet console=ttyS0,115200n8"
GRUB_TERMINAL="serial"
GRUB_SERIAL_COMMAND="serial --speed=115200 --unit=0 --word=8 --parity=no --stop=1"
GRUB_TIMEOUT=1
GRUB
update-grub
grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=LumoNAS --removable --no-nvram
grub-install --target=i386-pc --recheck "$LUMONAS_LOOP_DEVICE"

# Prove the boot chain this image claims to have, while it can still be fixed.
# Both grub-install calls report "Installation finished. No error reported."
# even when the result is unusable, and a BIOS image whose /boot/grub/i386-pc is
# empty still boots: SeaBIOS loads the embedded core.img, GRUB reports
# "file `/boot/grub/i386-pc/normal.mod' not found" and drops to "grub rescue>",
# which the smoke test sees only as "the appliance did not become ready".
# Check the artefacts the firmware will actually look for, and retry the BIOS
# install once before giving up, so a bad image fails the build rather than
# being published.
verify_boot_chain() {
	[ -s /boot/grub/grub.cfg ] || { echo "verify_boot_chain: /boot/grub/grub.cfg is missing or empty" >&2; return 1; }
	[ -f /boot/grub/i386-pc/normal.mod ] || { echo "verify_boot_chain: /boot/grub/i386-pc/normal.mod is missing" >&2; return 1; }
	[ -f /boot/grub/i386-pc/linux.mod ] || { echo "verify_boot_chain: /boot/grub/i386-pc/linux.mod is missing" >&2; return 1; }
	[ -s /boot/efi/EFI/BOOT/BOOTX64.EFI ] || { echo "verify_boot_chain: the EFI fallback loader is missing" >&2; return 1; }
	return 0
}
if ! verify_boot_chain; then
	echo "boot chain incomplete after the first install; retrying the BIOS install" >&2
	grub-install --target=i386-pc --recheck "$LUMONAS_LOOP_DEVICE"
	update-grub
	verify_boot_chain
fi
echo "boot chain verified: BIOS (i386-pc) and UEFI (BOOTX64.EFI) loaders present"
EOF

if [ -n "$UPDATE_FIXTURE" ]; then
	cat >>"$WORK/mnt/etc/lumonas/lumonasd.env" <<ENV
LUMONAS_UPDATE_PUBLIC_KEY=$UPDATE_PUBLIC_KEY
ENV
fi

echo "Created Debian 13 QEMU image: $OUTPUT"
