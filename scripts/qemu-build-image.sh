#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUTPUT="${MYNAS_QEMU_IMAGE:-$ROOT/build/qemu/mynas-debian13.raw}"
WORK="${MYNAS_QEMU_WORKDIR:-$ROOT/build/qemu/work}"
DEB="${MYNAS_DEB:-$ROOT/lumonas_${MYNAS_VERSION:-0.1.0-dev}_amd64.deb}"
SIZE="${MYNAS_QEMU_DISK_SIZE:-4G}"

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

chroot "$WORK/mnt" /bin/sh -eux <<'EOF'
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
  systemd systemd-sysv linux-image-amd64 grub-pc openssh-server curl ca-certificates \
  iproute2 util-linux smartmontools lm-sensors nut mergerfs snapraid \
  network-manager docker.io docker-compose samba samba-common-bin avahi-daemon
dpkg -i /tmp/lumonas.deb || apt-get -f install -y
rm -f /tmp/lumonas.deb
mkdir -p /etc/systemd/network /etc/systemd/system/mynas-web.service.d
cat >/etc/systemd/network/20-ethernet.network <<'NETWORK'
[Match]
Name=en* eth*

[Network]
DHCP=yes
NETWORK
cat >/etc/systemd/system/mynas-web.service.d/qemu.conf <<'DROPIN'
[Service]
Environment=MYNAS_WEB_LISTEN=0.0.0.0:8081
DROPIN
cat >/etc/fstab <<'FSTAB'
/dev/vda / ext4 defaults 0 1
FSTAB
systemctl enable systemd-networkd.service systemd-resolved.service docker.service smbd.service avahi-daemon.service mynas-privd.service mynas-privd-storage.service mynas-privd-network.service mynas-privd-power.service mynas-privd-general.service mynasd.service mynas-web.service || true
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
