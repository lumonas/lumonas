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
