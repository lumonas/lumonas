#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUTPUT="${LUMONAS_QEMU_IMAGE:-$ROOT/build/qemu/lumonas-debian13.raw}"
WORK="${LUMONAS_QEMU_WORKDIR:-$ROOT/build/qemu/work}"
DEB="${LUMONAS_DEB:-$ROOT/lumonas_${LUMONAS_VERSION:-0.1.0-dev}_amd64.deb}"
SIZE="${LUMONAS_QEMU_DISK_SIZE:-4G}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || printf '%s' 0)}"
SOURCE_COMMIT="${LUMONAS_SOURCE_COMMIT:-$(git -C "$ROOT" log -1 --format=%H 2>/dev/null || printf '%s' unknown)}"
case "$SOURCE_DATE_EPOCH" in
	''|*[!0-9]*) echo "SOURCE_DATE_EPOCH must be a non-negative integer" >&2; exit 1 ;;
esac
[ -n "$SOURCE_COMMIT" ] || { echo "LUMONAS_SOURCE_COMMIT must not be empty" >&2; exit 1; }
export SOURCE_DATE_EPOCH

for command in debootstrap qemu-img mkfs.ext4 grub-install; do
  command -v "$command" >/dev/null 2>&1 || { echo "$command is required" >&2; exit 1; }
done
[ "$(id -u)" -eq 0 ] || { echo "Run this builder as root (for example: sudo $0)" >&2; exit 1; }
[ -f "$DEB" ] || { echo "Build the Debian package first: $DEB" >&2; exit 1; }
[ ! -e "$OUTPUT" ] || { echo "Refusing to overwrite existing image: $OUTPUT" >&2; exit 1; }
[ ! -e "$WORK" ] || { echo "Refusing to overwrite existing work directory: $WORK" >&2; exit 1; }

mkdir -p "$(dirname "$OUTPUT")" "$WORK/mnt"
qemu-img create -f raw "$OUTPUT" "$SIZE" >/dev/null
mkfs.ext4 -F "$OUTPUT" >/dev/null
mount -o loop "$OUTPUT" "$WORK/mnt"

cleanup() {
  umount -R "$WORK/mnt/dev" 2>/dev/null || true
  umount -R "$WORK/mnt/proc" 2>/dev/null || true
  umount -R "$WORK/mnt/sys" 2>/dev/null || true
  umount "$WORK/mnt" 2>/dev/null || true
}
trap cleanup EXIT

debootstrap --arch=amd64 --variant=minbase trixie "$WORK/mnt" https://deb.debian.org/debian
mount --rbind /dev "$WORK/mnt/dev"
mount --make-rslave "$WORK/mnt/dev"
mount -t proc proc "$WORK/mnt/proc"
mount --rbind /sys "$WORK/mnt/sys"
mount --make-rslave "$WORK/mnt/sys"
rm -f "$WORK/mnt/etc/resolv.conf"
cp /etc/resolv.conf "$WORK/mnt/etc/resolv.conf"
cp "$DEB" "$WORK/mnt/tmp/lumonas.deb"

chroot "$WORK/mnt" /usr/bin/env LUMONAS_SOURCE_COMMIT="$SOURCE_COMMIT" LUMONAS_SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" /bin/sh -eux <<'EOF'
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
  systemd systemd-sysv systemd-resolved linux-image-amd64 grub-pc openssh-server curl ca-certificates openssl \
  iproute2 util-linux smartmontools lm-sensors nut nut-client e2fsprogs xfsprogs mergerfs snapraid \
  network-manager docker.io docker-compose samba samba-common-bin nfs-kernel-server rsync vsftpd \
  avahi-daemon nftables
dpkg -i /tmp/lumonas.deb || apt-get -f install -y
rm -f /tmp/lumonas.deb
mkdir -p /usr/share/doc/lumonas
{
  echo "formatVersion=1"
  echo "sourceCommit=$LUMONAS_SOURCE_COMMIT"
  echo "sourceDateEpoch=$LUMONAS_SOURCE_DATE_EPOCH"
  echo "packages:"
  dpkg-query -W -f='${Package}\t${Version}\n' | sort
} >/usr/share/doc/lumonas/qemu-package-manifest.txt
systemd-analyze verify /lib/systemd/system/lumonas-runtime.service /lib/systemd/system/lumonas-privd.service /lib/systemd/system/lumonas-privd-general.service /lib/systemd/system/lumonas-privd-network.service /lib/systemd/system/lumonas-privd-power.service /lib/systemd/system/lumonas-privd-storage.service /lib/systemd/system/lumonas-web.service /lib/systemd/system/lumonasd.service
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
cat >/etc/fstab <<'FSTAB'
/dev/vda / ext4 defaults 0 1
FSTAB
systemctl enable NetworkManager.service NetworkManager-wait-online.service systemd-resolved.service docker.service smbd.service avahi-daemon.service lumonas-runtime.service lumonas-privd.service lumonas-privd-storage.service lumonas-privd-network.service lumonas-privd-power.service lumonas-privd-general.service lumonasd.service lumonas-web.service || true
systemctl disable systemd-networkd.service systemd-networkd-wait-online.service || true
ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf
passwd -l root || true
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
cat >/etc/default/grub <<'GRUB'
GRUB_CMDLINE_LINUX_DEFAULT="quiet console=ttyS0,115200n8"
GRUB_TERMINAL="serial"
GRUB_SERIAL_COMMAND="serial --speed=115200 --unit=0 --word=8 --parity=no --stop=1"
GRUB_TIMEOUT=1
GRUB
update-grub
EOF

grub-install --target=i386-pc --recheck --boot-directory="$WORK/mnt/boot" "$OUTPUT"
echo "Created Debian 13 QEMU image: $OUTPUT"
