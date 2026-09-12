#!/bin/sh
set -eu

VERSION="${1:-0.1.0-dev}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORK="${LUMONAS_ISO_WORKDIR:-$ROOT/build/iso-live}"
DEB="${LUMONAS_DEB:-$ROOT/lumonas_${VERSION}_amd64.deb}"

command -v lb >/dev/null 2>&1 || { echo "live-build is required" >&2; exit 1; }
command -v dpkg-deb >/dev/null 2>&1 || { echo "dpkg-deb is required" >&2; exit 1; }
command -v dpkg-scanpackages >/dev/null 2>&1 || { echo "dpkg-scanpackages (dpkg-dev) is required" >&2; exit 1; }
[ -f "$DEB" ] || { echo "Build the Debian package first: $DEB" >&2; exit 1; }

rm -rf "$WORK"
mkdir -p "$WORK/config/package-lists" "$WORK/config/hooks/live" "$WORK/config/includes.chroot/opt/lumonas-repo/pool/main/l/lumonas" "$WORK/config/includes.chroot/usr/share/doc/lumonas"
cp "$DEB" "$WORK/config/includes.chroot/opt/lumonas-repo/pool/main/l/lumonas/lumonas.deb"
(cd "$WORK/config/includes.chroot/opt/lumonas-repo" && dpkg-scanpackages pool /dev/null > Packages && gzip -9c Packages > Packages.gz)
cat > "$WORK/config/package-lists/lumonas.list.chroot" <<'EOF'
network-manager
avahi-daemon
nftables
openssh-server
samba
nfs-kernel-server
rsync
smartmontools
e2fsprogs
lm-sensors
nut
mergerfs
snapraid
docker.io
docker-compose
EOF
cat > "$WORK/config/hooks/live/020-install-lumonas.hook.chroot" <<'EOF'
#!/bin/sh
set -eu
cat >/etc/apt/sources.list.d/lumonas-local.list <<'APT'
deb [trusted=yes] file:/opt/lumonas-repo ./
APT
dpkg -i /opt/lumonas-repo/pool/main/l/lumonas/lumonas.deb
cat >/etc/lumonas/lumonas-web.env <<'ENV'
LUMONAS_WEB_LISTEN=0.0.0.0:8081
LUMONAS_WEB_ROOT=/usr/share/lumonas/web
LUMONAS_API_URL=http://127.0.0.1:8080
ENV
chown root:lumonas /etc/lumonas/lumonas-web.env
chmod 0640 /etc/lumonas/lumonas-web.env
systemctl enable lumonas-privd.service lumonas-privd-storage.service lumonas-privd-network.service lumonas-privd-power.service lumonas-privd-general.service lumonasd.service lumonas-web.service
EOF
chmod 0755 "$WORK/config/hooks/live/020-install-lumonas.hook.chroot"
if [ "${LUMONAS_ENABLE_RECOVERY_SMOKE:-false}" = "true" ]; then
	cat > "$WORK/config/hooks/live/030-recovery-smoke.hook.chroot" <<'EOF'
#!/bin/sh
set -eu
cat >/usr/local/sbin/lumonas-recovery-iso-smoke <<'SCRIPT'
#!/bin/sh
set -eu
recovery_device=""
target_device=""
for attempt in $(seq 1 30); do
	if [ -e /dev/disk/by-id/virtio-LUMONAS-RECOVERY ]; then
		recovery_device=/dev/disk/by-id/virtio-LUMONAS-RECOVERY
	fi
	if [ -e /dev/disk/by-id/virtio-LUMONAS-REPLACEMENT ]; then
		target_device=/dev/disk/by-id/virtio-LUMONAS-REPLACEMENT
	fi
	[ -n "$recovery_device" ] && [ -n "$target_device" ] && break
	sleep 1
done
[ -n "$recovery_device" ] && [ -n "$target_device" ] || exit 0
mkdir -p /mnt/lumonas-recovery /mnt/lumonas-target
mount -o ro "$recovery_device" /mnt/lumonas-recovery
[ -f /mnt/lumonas-recovery/.lumonas-recovery-test ] || exit 0
mkfs.ext4 -F "$target_device" >/dev/null
mount "$target_device" /mnt/lumonas-target
/usr/lib/lumonas/lumonas-recover \
	--bundle /mnt/lumonas-recovery/latest.mrb \
	--key-file /mnt/lumonas-recovery/recovery.key \
	--root /mnt/lumonas-target --apply \
	> /mnt/lumonas-target/recovery-result.json
printf '%s\n' recovery-applied > /mnt/lumonas-target/recovery-success
sync
systemctl poweroff
SCRIPT
chmod 0755 /usr/local/sbin/lumonas-recovery-iso-smoke
cat >/etc/systemd/system/lumonas-recovery-smoke.service <<'UNIT'
[Unit]
Description=LumoNAS offline recovery smoke test
After=lumonas-web.service systemd-udev-settle.service
Wants=lumonas-web.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/lumonas-recovery-iso-smoke
User=root
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
UNIT
systemctl enable lumonas-recovery-smoke.service
EOF
	chmod 0755 "$WORK/config/hooks/live/030-recovery-smoke.hook.chroot"
fi
cat > "$WORK/config/includes.chroot/usr/share/doc/lumonas/build-manifest.txt" <<EOF
LumoNAS release: $VERSION
Baseline: Debian 13 (Trixie)
Core package: lumonas.deb
EOF

(cd "$WORK" && lb config --distribution trixie --architectures amd64 --binary-images iso-hybrid --debian-installer live --archive-areas "main contrib non-free-firmware" --apt-indices false)
(cd "$WORK" && lb build)
mkdir -p "$ROOT/build/releases"
cp "$WORK"/live-image-amd64.hybrid.iso "$ROOT/build/releases/lumonas-$VERSION-amd64.iso"
cd "$ROOT/build/releases"
sha256sum "lumonas-$VERSION-amd64.iso" > SHA256SUMS
echo "Created $ROOT/build/releases/lumonas-$VERSION-amd64.iso"
