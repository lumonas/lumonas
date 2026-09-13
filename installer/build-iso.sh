#!/bin/sh
set -eu

VERSION="${1:-0.1.0-dev}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORK="${LUMONAS_ISO_WORKDIR:-$ROOT/build/iso-live}"
DEB="${LUMONAS_DEB:-$ROOT/lumonas_${VERSION}_amd64.deb}"
REPO_ORIGIN="LumoNAS"
REPO_SIGN_KEY="${LUMONAS_REPO_SIGN_KEY:-}"
DEBIAN_MIRROR="${LUMONAS_DEBIAN_MIRROR:-http://deb.debian.org/debian}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || date +%s)}"
SOURCE_COMMIT="${LUMONAS_SOURCE_COMMIT:-$(git -C "$ROOT" log -1 --format=%H 2>/dev/null || printf '%s' unknown)}"
case "$SOURCE_DATE_EPOCH" in
	''|*[!0-9]*) echo "SOURCE_DATE_EPOCH must be a non-negative integer" >&2; exit 1 ;;
esac
[ -n "$SOURCE_COMMIT" ] || { echo "LUMONAS_SOURCE_COMMIT must not be empty" >&2; exit 1; }
export SOURCE_DATE_EPOCH
if [ "${LUMONAS_REQUIRE_REPO_SIGNATURE:-false}" = "true" ] && [ -z "$REPO_SIGN_KEY" ]; then
	echo "LUMONAS_REPO_SIGN_KEY is required for a signed offline repository" >&2
	exit 1
fi
REPO_SIGNATURE_REQUIRED=false
if [ "${LUMONAS_REQUIRE_REPO_SIGNATURE:-false}" = "true" ]; then
	REPO_SIGNATURE_REQUIRED=true
fi

command -v lb >/dev/null 2>&1 || { echo "live-build is required" >&2; exit 1; }
command -v dpkg-deb >/dev/null 2>&1 || { echo "dpkg-deb is required" >&2; exit 1; }
command -v dpkg-scanpackages >/dev/null 2>&1 || { echo "dpkg-scanpackages (dpkg-dev) is required" >&2; exit 1; }
command -v apt-ftparchive >/dev/null 2>&1 || { echo "apt-ftparchive (apt-utils) is required" >&2; exit 1; }
[ -f "$DEB" ] || { echo "Build the Debian package first: $DEB" >&2; exit 1; }

rm -rf "$WORK"
mkdir -p "$WORK/config/package-lists" "$WORK/config/hooks/live" "$WORK/config/includes.chroot/opt/lumonas-repo/pool/main/l/lumonas" "$WORK/config/includes.chroot/usr/share/doc/lumonas" "$WORK/config/includes.chroot/etc/apt/preferences.d"
cp "$DEB" "$WORK/config/includes.chroot/opt/lumonas-repo/pool/main/l/lumonas/lumonas.deb"
REPO_DIR="$WORK/config/includes.chroot/opt/lumonas-repo"
(cd "$REPO_DIR" && dpkg-scanpackages --multiversion pool /dev/null > Packages && gzip -9c Packages > Packages.gz)
cd "$REPO_DIR"
printf 'Origin: %s\nLabel: %s\nSuite: stable\nCodename: stable\nDate: %s\nArchitectures: amd64\nComponents: main\nDescription: Embedded LumoNAS offline repository\n' \
	"$REPO_ORIGIN" "$REPO_ORIGIN" "$(date -u -R -d "@$SOURCE_DATE_EPOCH")" > Release.tmp
apt-ftparchive release -c /dev/null \
	-o "APT::FTPArchive::Release::Origin=$REPO_ORIGIN" \
	-o "APT::FTPArchive::Release::Label=$REPO_ORIGIN" \
	-o "APT::FTPArchive::Release::Suite=stable" \
	-o "APT::FTPArchive::Release::Codename=stable" \
	-o "APT::FTPArchive::Release::Architectures=amd64" \
	-o "APT::FTPArchive::Release::Components=main" \
	Packages Packages.gz >> Release.tmp 2>/dev/null
sort -u -o Release.tmp Release.tmp
mv Release.tmp Release

if [ -n "$REPO_SIGN_KEY" ]; then
	command -v gpg >/dev/null 2>&1 || { echo "gpg is required when LUMONAS_REPO_SIGN_KEY is set" >&2; exit 1; }
	GNUPGHOME="${LUMONAS_REPO_GNUPGHOME:-$ROOT/build/repo-gnupg}"
	export GNUPGHOME
	mkdir -p "$GNUPGHOME"
	chmod 0700 "$GNUPGHOME"
	gpg --batch --pinentry-mode loopback --yes --detach-sign --default-key "$REPO_SIGN_KEY" --output Release.gpg Release
	gpg --batch --pinentry-mode loopback --yes --clearsign --default-key "$REPO_SIGN_KEY" --output InRelease Release
	gpg --batch --yes --export "$REPO_SIGN_KEY" > "$WORK/config/includes.chroot/usr/share/keyrings/lumonas-archive-keyring.gpg"
	test -s Release.gpg
	test -s InRelease
	test -s "$WORK/config/includes.chroot/usr/share/keyrings/lumonas-archive-keyring.gpg"
	chmod 0644 "$WORK/config/includes.chroot/usr/share/keyrings/lumonas-archive-keyring.gpg"
	REPO_SOURCE="deb [signed-by=/usr/share/keyrings/lumonas-archive-keyring.gpg] file:/opt/lumonas-repo ./"
	echo "Embedded APT repository signed with $REPO_SIGN_KEY"
else
	echo "WARNING: LUMONAS_REPO_SIGN_KEY is not set — embedding an UNSIGNED repository ([trusted=yes])." >&2
	echo "WARNING: Set it to a GPG key fingerprint to ship a signed offline install." >&2
	REPO_SOURCE="deb [trusted=yes] file:/opt/lumonas-repo ./"
fi
cd "$ROOT"

# Pin the LumoNAS core package to the embedded repository so a stray mirror
# copy can never shadow the ISO payload (same ISO = same installed core).
cat > "$WORK/config/includes.chroot/etc/apt/preferences.d/lumonas" <<EOF
Package: lumonas
Pin: release o=$REPO_ORIGIN
Pin-Priority: 1001

Package: lumonas-privd
Pin: release o=$REPO_ORIGIN
Pin-Priority: 1001
EOF
cat > "$WORK/config/package-lists/lumonas.list.chroot" <<'EOF'
network-manager
systemd-resolved
avahi-daemon
nftables
openssh-server
samba
samba-common-bin
nfs-kernel-server
rsync
vsftpd
smartmontools
e2fsprogs
xfsprogs
lm-sensors
nut
nut-client
curl
mergerfs
snapraid
docker.io
docker-compose
grub-pc-bin
EOF
cat > "$WORK/config/hooks/live/020-install-lumonas.hook.chroot" <<EOF
#!/bin/sh
set -eu
cat >/etc/apt/sources.list.d/lumonas-local.list <<'APT'
$REPO_SOURCE
APT
REPO_SIGNATURE_REQUIRED="$REPO_SIGNATURE_REQUIRED"
if ! apt-get update -o Dir::Etc::sourcelist="sources.list.d/lumonas-local.list" -o Dir::Etc::sourceparts="-" -o APT::Get::List-Cleanup="0"; then
	if [ "\$REPO_SIGNATURE_REQUIRED" = "true" ]; then
		echo "signed LumoNAS repository metadata could not be verified" >&2
		exit 1
	fi
	dpkg -i /opt/lumonas-repo/pool/main/l/lumonas/lumonas.deb
elif ! apt-get install -y --allow-downgrades lumonas; then
	if [ "\$REPO_SIGNATURE_REQUIRED" = "true" ]; then
		echo "signed LumoNAS repository package installation failed" >&2
		exit 1
	fi
	dpkg -i /opt/lumonas-repo/pool/main/l/lumonas/lumonas.deb
fi
mkdir -p /usr/share/doc/lumonas
{
  echo "formatVersion=1"
  echo "sourceCommit=$SOURCE_COMMIT"
  echo "sourceDateEpoch=$SOURCE_DATE_EPOCH"
  echo "packages:"
  dpkg-query -W -f='\${Package}\t\${Version}\n' | sort
} >/usr/share/doc/lumonas/iso-package-manifest.txt
cat >/etc/lumonas/lumonas-web.env <<'ENV'
LUMONAS_WEB_LISTEN=0.0.0.0:8081
LUMONAS_WEB_ROOT=/usr/share/lumonas/web
LUMONAS_API_URL=http://127.0.0.1:8080
LUMONAS_WEB_TLS_CERT=/etc/lumonas/tls/server.crt
LUMONAS_WEB_TLS_KEY=/etc/lumonas/tls/server.key
LUMONAS_COOKIE_SECURE=true
ENV
chown root:lumonas /etc/lumonas/lumonas-web.env
chmod 0640 /etc/lumonas/lumonas-web.env
systemctl enable lumonas-runtime.service lumonas-privd.service lumonas-privd-storage.service lumonas-privd-network.service lumonas-privd-power.service lumonas-privd-general.service lumonasd.service lumonas-web.service
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
source_mode=false
[ -f /mnt/lumonas-recovery/.lumonas-recovery-source ] && source_mode=true
mkfs.ext4 -F "$target_device" >/dev/null
mount "$target_device" /mnt/lumonas-target
# The recovery medium is an installer, not only a payload extractor. Copy the
# live Debian runtime onto the replacement filesystem while excluding live
# mounts and mutable state that the verified bundle will restore below.
rsync -aHAX --numeric-ids --one-file-system \
	--exclude=/dev/** --exclude=/proc/** --exclude=/sys/** --exclude=/run/** \
	--exclude=/tmp/** --exclude=/mnt/** --exclude=/var/lib/lumonas/** \
	--exclude=/srv/lumonas/** \
	/ /mnt/lumonas-target/
mkdir -p /mnt/lumonas-target/{dev,proc,sys,run,tmp,var/lib/lumonas,srv/lumonas}
root_uuid=$(blkid -s UUID -o value "$target_device")
[ -n "$root_uuid" ]
printf 'UUID=%s / ext4 defaults 0 1\n' "$root_uuid" > /mnt/lumonas-target/etc/fstab
printf '%s\n' 'lumonas-recovered' > /mnt/lumonas-target/etc/hostname
mkdir -p /mnt/lumonas-target/etc/NetworkManager/system-connections
cat >/mnt/lumonas-target/etc/NetworkManager/system-connections/recovery-ethernet.nmconnection <<'NETWORK'
[connection]
id=recovery-ethernet
type=ethernet
autoconnect=true
autoconnect-retries=0

[ipv4]
method=auto

[ipv6]
method=auto
NETWORK
chmod 600 /mnt/lumonas-target/etc/NetworkManager/system-connections/recovery-ethernet.nmconnection
grub-install --target=i386-pc --recheck --boot-directory=/mnt/lumonas-target/boot "$target_device"
kernel_path=$(find /mnt/lumonas-target/boot -maxdepth 1 -type f -name 'vmlinuz-*' | sort | tail -n 1)
initrd_path=$(find /mnt/lumonas-target/boot -maxdepth 1 -type f -name 'initrd.img-*' | sort | tail -n 1)
if [ -z "$kernel_path" ] && [ -f /mnt/lumonas-target/live/vmlinuz ]; then
	cp /mnt/lumonas-target/live/vmlinuz /mnt/lumonas-target/boot/vmlinuz-recovery
	kernel_path=/mnt/lumonas-target/boot/vmlinuz-recovery
fi
if [ -z "$initrd_path" ] && [ -f /mnt/lumonas-target/live/initrd.img ]; then
	cp /mnt/lumonas-target/live/initrd.img /mnt/lumonas-target/boot/initrd.img-recovery
	initrd_path=/mnt/lumonas-target/boot/initrd.img-recovery
fi
[ -n "$root_uuid" ] && [ -n "$kernel_path" ] && [ -n "$initrd_path" ]
kernel_name=${kernel_path##*/}
initrd_name=${initrd_path##*/}
cat >/mnt/lumonas-target/boot/grub/grub.cfg <<GRUB
insmod ext2
search --no-floppy --fs-uuid --set=root $root_uuid
set timeout=1
set serial=0
serial --unit=0 --speed=115200
terminal_input console serial
terminal_output console serial
menuentry 'LumoNAS recovered appliance' {
    linux /boot/$kernel_name root=UUID=$root_uuid ro quiet console=ttyS0,115200n8
    initrd /boot/$initrd_name
}
GRUB
/usr/lib/lumonas/lumonas-recover \
	--bundle /mnt/lumonas-recovery/latest.mrb \
	--key-file /mnt/lumonas-recovery/recovery.key \
	--root /mnt/lumonas-target --apply \
	> /mnt/lumonas-target/recovery-result.json
backend_pid=""
cleanup_backend() {
	if [ -n "$backend_pid" ]; then
		kill "$backend_pid" 2>/dev/null || true
		wait "$backend_pid" 2>/dev/null || true
	fi
}
trap cleanup_backend EXIT INT TERM
LUMONASD_LISTEN=127.0.0.1:18083 \
LUMONAS_DB_PATH=/mnt/lumonas-target/var/lib/lumonas/lumonas.db \
LUMONAS_SHARES_FILE=/mnt/lumonas-target/var/lib/lumonas/shares.json \
LUMONAS_SNAPRAID_CONFIG=/mnt/lumonas-target/etc/lumonas/snapraid.conf \
LUMONAS_STACK_ROOT=/mnt/lumonas-target/srv/lumonas/docker/stacks \
LUMONAS_RECOVERY_DIR=/mnt/lumonas-target/var/lib/lumonas/recovery \
LUMONAS_AUTH_REQUIRED=false \
	/usr/lib/lumonas/lumonasd > /mnt/lumonas-target/restored-lumonasd.log 2>&1 &
backend_pid=$!
backend_ready=false
for attempt in $(seq 1 30); do
	if curl -fsS http://127.0.0.1:18083/healthz >/dev/null 2>&1 && \
		curl -fsS http://127.0.0.1:18083/readyz >/dev/null 2>&1 && \
		curl -fsS http://127.0.0.1:18083/api/v1/principals > /mnt/lumonas-target/restored-principals.json 2>/dev/null && \
		curl -fsS http://127.0.0.1:18083/api/v1/shares > /mnt/lumonas-target/restored-shares.json 2>/dev/null && \
		curl -fsS http://127.0.0.1:18083/api/v1/storage/mounts > /mnt/lumonas-target/restored-mounts.json 2>/dev/null && \
		curl -fsS http://127.0.0.1:18083/api/v1/network/connections > /mnt/lumonas-target/restored-network.json 2>/dev/null; then
		backend_ready=true
		break
	fi
	sleep 1
done
[ "$backend_ready" = true ]
grep -F 'operator' /mnt/lumonas-target/restored-principals.json >/dev/null
grep -F 'share-media' /mnt/lumonas-target/restored-shares.json >/dev/null
if [ "$source_mode" = false ]; then
  grep -F 'fuse.mergerfs' /mnt/lumonas-target/restored-mounts.json >/dev/null
  grep -F 'serial_DATA1' /mnt/lumonas-target/restored-mounts.json >/dev/null
  grep -F '"id":"lan"' /mnt/lumonas-target/restored-network.json >/dev/null
  grep -F '"interface":"eth0"' /mnt/lumonas-target/restored-network.json >/dev/null
else
  grep -F 'fuse.mergerfs' /mnt/lumonas-target/restored-mounts.json >/dev/null
  grep -F 'serial_LUMONAS-DATA1' /mnt/lumonas-target/restored-mounts.json >/dev/null
  grep -F '"id":"lan"' /mnt/lumonas-target/restored-network.json >/dev/null
  grep -F '"interface":"eth0"' /mnt/lumonas-target/restored-network.json >/dev/null
fi
cleanup_backend
backend_pid=""
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
TimeoutStartSec=90s
FailureAction=poweroff

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
Embedded repository: /opt/lumonas-repo (origin $REPO_ORIGIN, pinned at priority 1001)
Repository signature: $(if [ -n "$REPO_SIGN_KEY" ]; then echo "signed by $REPO_SIGN_KEY"; else echo "UNSIGNED (LUMONAS_REPO_SIGN_KEY not set)"; fi)
Source date epoch: $SOURCE_DATE_EPOCH
Source commit: $SOURCE_COMMIT
EOF

(cd "$WORK" && lb config \
	--distribution trixie \
	--architectures amd64 \
	--mirror-bootstrap "$DEBIAN_MIRROR" \
	--mirror-chroot "$DEBIAN_MIRROR" \
	--mirror-binary "$DEBIAN_MIRROR" \
	--mirror-debian-installer "$DEBIAN_MIRROR" \
	--binary-images iso-hybrid \
	--debian-installer live \
	--archive-areas "main contrib non-free-firmware" \
	--apt-indices false \
	--apt-options "-o APT::Get::Assume-Yes=true -o Acquire::ForceIPv4=true -o Acquire::Retries=5" \
	--firmware-binary false \
	--firmware-chroot false)
(cd "$WORK" && lb build)
mkdir -p "$ROOT/build/releases"
cp "$WORK"/live-image-amd64.hybrid.iso "$ROOT/build/releases/lumonas-$VERSION-amd64.iso"
cd "$ROOT/build/releases"
sha256sum "lumonas-$VERSION-amd64.iso" > SHA256SUMS
echo "Created $ROOT/build/releases/lumonas-$VERSION-amd64.iso"
