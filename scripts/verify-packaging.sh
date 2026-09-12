#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SYSTEMD="$ROOT/packaging/systemd"

require_line() {
	file=$1
	pattern=$2
	if ! grep -F "$pattern" "$file" >/dev/null 2>&1; then
		echo "missing '$pattern' in $file" >&2
		exit 1
	fi
}

for unit in lumonas-web.service lumonasd.service lumonas-privd.service lumonas-privd-storage.service lumonas-privd-network.service lumonas-privd-power.service lumonas-privd-general.service; do
	[ -f "$SYSTEMD/$unit" ] || { echo "missing systemd unit: $unit" >&2; exit 1; }
	require_line "$SYSTEMD/$unit" 'NoNewPrivileges=true'
	require_line "$SYSTEMD/$unit" 'ProtectSystem=strict'
	require_line "$SYSTEMD/$unit" 'MemoryMax='
	require_line "$SYSTEMD/$unit" 'TasksMax='
	require_line "$SYSTEMD/$unit" 'Group=lumonas'
done

for key in NoNewPrivileges=true ProtectSystem=strict MemoryMax= TasksMax= Group=lumonas; do
	require_line "$SYSTEMD/lumonas-runtime.service" "$key"
done
require_line "$SYSTEMD/lumonas-runtime.service" 'PrivateTmp=false'

require_line "$SYSTEMD/lumonas-web.service" 'User=lumonas'
require_line "$SYSTEMD/lumonasd.service" 'User=lumonas'
require_line "$SYSTEMD/lumonas-privd.service" 'User=root'
require_line "$SYSTEMD/lumonas-privd.service" 'CapabilityBoundingSet='
require_line "$SYSTEMD/lumonasd.service" 'ReadWritePaths=/var/lib/lumonas /srv/lumonas'
require_line "$SYSTEMD/lumonasd.service" 'Requires=lumonas-privd.service lumonas-privd-storage.service lumonas-privd-network.service lumonas-privd-power.service lumonas-privd-general.service'
for worker in storage network power general; do
  require_line "$SYSTEMD/lumonas-privd-$worker.service" 'Requires=lumonas-privd.service'
done
require_line "$SYSTEMD/lumonas-privd-storage.service" 'CapabilityBoundingSet=CAP_SYS_ADMIN CAP_SYS_RAWIO'
require_line "$SYSTEMD/lumonas-privd-network.service" 'CapabilityBoundingSet=CAP_NET_ADMIN'
require_line "$SYSTEMD/lumonas-privd-power.service" 'CapabilityBoundingSet=CAP_SYS_BOOT'
require_line "$SYSTEMD/lumonas-privd-general.service" 'CapabilityBoundingSet=CAP_CHOWN CAP_DAC_OVERRIDE CAP_FOWNER CAP_SETGID CAP_SETUID'
require_line "$SYSTEMD/lumonas-privd-general.service" '/etc/exports.d /etc/ssh/sshd_config.d'
require_line "$SYSTEMD/lumonas-runtime.service" 'EnvironmentFile=-/etc/lumonas/runtime.env'
require_line "$SYSTEMD/lumonas-runtime.service" 'ExecStart=/usr/lib/lumonas/lumonas-privd --runtime'
require_line "$SYSTEMD/lumonas-runtime.service" 'CapabilityBoundingSet=CAP_SYS_ADMIN CAP_SYS_MODULE CAP_SYS_RESOURCE'

require_line "$ROOT/packaging/debian/postinst" 'useradd --system'
require_line "$ROOT/packaging/debian/postinst" '/var/lib/lumonas/secrets'
require_line "$ROOT/packaging/debian/postinst" '/etc/lumonas/lumonasd.env'
require_line "$ROOT/packaging/debian/postinst" 'LUMONAS_WEB_ROOT=/usr/share/lumonas/web'
require_line "$ROOT/packaging/debian/postinst" '/etc/lumonas/tls/server.key'
require_line "$ROOT/packaging/build-deb.sh" 'build-manifest.json'
require_line "$ROOT/packaging/build-deb.sh" 'daemon.json.lumonas'
require_line "$ROOT/packaging/debian/postinst" '/etc/docker/daemon.json'
require_line "$ROOT/packaging/docker-daemon.json" '"max-size": "10m"'
require_line "$ROOT/packaging/docker-daemon.json" '"max-file": "3"'
require_line "$ROOT/packaging/debian/postinst" 'lumonas-privd.service'
require_line "$ROOT/packaging/debian/postinst" 'LUMONAS_ZRAM_ENABLED=false'
require_line "$ROOT/scripts/upgrade-service-order-smoke.sh" 'lumonas-runtime.service'
require_line "$ROOT/scripts/release-artifacts.sh" 'sha256sum "$artifact"'
require_line "$ROOT/scripts/release-artifacts-smoke.sh" 'verify-release.sh'
require_line "$ROOT/scripts/installer-signature-policy-smoke.sh" 'LUMONAS_REPO_SIGN_KEY is required'
require_line "$ROOT/scripts/permission-smoke.sh" '/etc/lumonas/runtime.env'
require_line "$ROOT/scripts/qemu-build-image.sh" 'lumonas-runtime.service'
require_line "$ROOT/cmd/lumonasd/share_configs.go" '/etc/lumonas/tls/server.crt'
require_line "$ROOT/cmd/lumonasd/share_configs.go" '/etc/lumonas/tls/server.key'
if grep -F '/etc/lumonas/tls/tls.crt' "$ROOT/cmd/lumonasd/share_configs.go" >/dev/null 2>&1 || grep -F '/etc/lumonas/tls/tls.key' "$ROOT/cmd/lumonasd/share_configs.go" >/dev/null 2>&1; then
	echo "share configuration still references obsolete FTPS certificate paths" >&2
	exit 1
fi
require_line "$ROOT/packaging/debian/prerm" 'lumonas-web.service'
require_line "$ROOT/packaging/build-deb.sh" 'DEBIAN/prerm'
require_line "$ROOT/scripts/api-smoke.sh" 'start_server()'
require_line "$ROOT/scripts/api-smoke.sh" 'daemon restarted before the job completed'
require_line "$ROOT/scripts/api-smoke.sh" 'Last-Event-ID'
require_line "$ROOT/installer/build-iso.sh" 'dpkg-scanpackages'
require_line "$ROOT/installer/build-iso.sh" 'lumonas-local.list'
require_line "$ROOT/installer/build-iso.sh" 'LUMONAS_WEB_LISTEN=0.0.0.0:8081'
require_line "$ROOT/installer/build-iso.sh" 'LUMONAS_ENABLE_RECOVERY_SMOKE'
require_line "$ROOT/installer/build-iso.sh" 'apt-ftparchive'
require_line "$ROOT/installer/build-iso.sh" 'InRelease'
require_line "$ROOT/installer/build-iso.sh" 'signed-by=/usr/share/keyrings/lumonas-archive-keyring.gpg'
require_line "$ROOT/installer/build-iso.sh" 'etc/apt/preferences.d/lumonas'
require_line "$ROOT/installer/build-iso.sh" 'Pin-Priority: 1001'
require_line "$ROOT/installer/build-iso.sh" 'LUMONAS_REPO_SIGN_KEY'
require_line "$ROOT/installer/build-iso.sh" 'LUMONAS_REQUIRE_REPO_SIGNATURE'
require_line "$ROOT/installer/build-iso.sh" 'test -s InRelease'
require_line "$ROOT/scripts/qemu-smoke.sh" '<title>LumoNAS</title>'
require_line "$ROOT/scripts/qemu-smoke.sh" '<div id="root"></div>'
require_line "$ROOT/scripts/qemu-smoke.sh" 'snapshot_disk_identities'
require_line "$ROOT/scripts/qemu-smoke.sh" 'device reorder did not change any transient device path'
require_line "$ROOT/scripts/qemu-smoke.sh" '"tmpfs"'
require_line "$ROOT/scripts/qemu-smoke.sh" 'lumonas-runtime.service'
require_line "$ROOT/scripts/frontend-runtime-smoke.sh" 'production frontend bundle contains the MSW bootstrap'
for worker in storage network power general; do
  require_line "$ROOT/scripts/qemu-smoke.sh" "lumonas-privd-$worker.service"
done
[ -x "$ROOT/scripts/storage-loopback-smoke.sh" ] || { echo "storage loopback smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/frontend-runtime-smoke.sh" ] || { echo "frontend runtime smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/share-config-smoke.sh" ] || { echo "share configuration smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/iso-smoke.sh" ] || { echo "ISO smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/qemu-recovery-smoke.sh" ] || { echo "QEMU recovery smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/qemu-live-recovery-source.sh" ] || { echo "live recovery source helper must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/recovery-api-smoke.sh" ] || { echo "recovery API smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/security-smoke.sh" ] || { echo "security smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/systemd-smoke.sh" ] || { echo "systemd smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/systemd-security-smoke.sh" ] || { echo "systemd security smoke test must be executable" >&2; exit 1; }
require_line "$ROOT/Makefile" 'storage-loopback:'
require_line "$ROOT/Makefile" 'share-config-smoke:'
require_line "$ROOT/Makefile" 'qemu-recovery-smoke:'
require_line "$ROOT/Makefile" 'qemu-recovery-live:'
require_line "$ROOT/Makefile" 'recovery-fixture:'
require_line "$ROOT/Makefile" 'recovery-api-smoke:'
require_line "$ROOT/Makefile" 'security-smoke:'
require_line "$ROOT/Makefile" 'systemd-smoke:'
require_line "$ROOT/Makefile" 'systemd-security-smoke:'
require_line "$ROOT/Makefile" 'permission-smoke:'
require_line "$ROOT/Makefile" 'log-retention-smoke:'
require_line "$ROOT/Makefile" 'check-api-contract:'
require_line "$ROOT/Makefile" 'upgrade-smoke:'
[ -x "$ROOT/scripts/upgrade-smoke.sh" ] || { echo "upgrade smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/upgrade-service-order-smoke.sh" ] || { echo "upgrade service ordering smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/release-artifacts-smoke.sh" ] || { echo "release artifact smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/installer-signature-policy-smoke.sh" ] || { echo "installer signature policy smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/permission-smoke.sh" ] || { echo "permission smoke test must be executable" >&2; exit 1; }
[ -x "$ROOT/scripts/log-retention-smoke.sh" ] || { echo "log retention smoke test must be executable" >&2; exit 1; }
require_line "$ROOT/packaging/build-deb.sh" 'cmd/lumonas-recover'
require_line "$ROOT/packaging/build-deb.sh" 'cmd/lumonas-migrate'
require_line "$ROOT/packaging/debian/postinst" 'lumonas-migrate'
require_line "$ROOT/packaging/debian/postinst" 'runuser -u lumonas'
require_line "$ROOT/packaging/debian/control" 'avahi-daemon'
require_line "$ROOT/packaging/debian/control" 'vsftpd'
require_line "$ROOT/packaging/debian/control" 'xfsprogs'
require_line "$ROOT/packaging/debian/control" 'mergerfs'
require_line "$ROOT/packaging/debian/control" 'snapraid'
require_line "$ROOT/scripts/storage-loopback-smoke.sh" 'MISMATCH_LOOP=$LAST_LOOP'
require_line "$ROOT/scripts/storage-loopback-smoke.sh" 'wipefs --all --force'
require_line "$ROOT/scripts/storage-loopback-smoke.sh" 'snapraid -c "$SNAPRAID_CONFIG" sync'
require_line "$ROOT/scripts/storage-loopback-smoke.sh" 'missing-snapraid.conf'
require_line "$ROOT/installer/build-iso.sh" 'xfsprogs'
require_line "$ROOT/scripts/qemu-build-image.sh" 'xfsprogs'
require_line "$ROOT/installer/build-iso.sh" 'vsftpd'
require_line "$ROOT/scripts/qemu-build-image.sh" 'NetworkManager.service'
require_line "$ROOT/scripts/qemu-build-image.sh" 'qemu-ethernet.nmconnection'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" 'config-generation.json'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" 'fuse.mergerfs'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" 'cmd/lumonas-recover'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" 'LUMONAS_RECOVER_BIN'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" 'LUMONAS_RECOVERY_SOURCE_IMAGE'
require_line "$ROOT/scripts/qemu-live-recovery-source.sh" 'api/v1/recovery/export'
require_line "$ROOT/scripts/qemu-live-recovery-source.sh" 'filesystem.create'
require_line "$ROOT/scripts/qemu-live-recovery-source.sh" 'storage/pools/plan'
require_line "$ROOT/scripts/qemu-live-recovery-source.sh" 'api/v1/network/connections'
require_line "$ROOT/scripts/qemu-live-recovery-source.sh" 'mode: live-source'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" '"databaseValid":true'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" 'databaseRestored'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" 'secretsRestored'
require_line "$ROOT/installer/build-iso.sh" 'LUMONASD_LISTEN=127.0.0.1:18083'
require_line "$ROOT/installer/build-iso.sh" 'restored-principals.json'
require_line "$ROOT/installer/build-iso.sh" 'restored-shares.json'
require_line "$ROOT/installer/build-iso.sh" 'serial_LUMONAS-DATA1'
require_line "$ROOT/installer/build-iso.sh" 'FailureAction=poweroff'
require_line "$ROOT/scripts/qemu-recovery-smoke.sh" 'recovery guest did not power off before timeout'
require_line "$ROOT/docs/16_TESTING_CI_RELEASE_ENGINEERING.md" 'shared bounded runner'
require_line "$ROOT/docs/16_TESTING_CI_RELEASE_ENGINEERING.md" 'privileged Unix-socket client'
require_line "$ROOT/scripts/qemu-build-image.sh" 'vsftpd'
for dropin in smbd.service.d/lumonas.conf rsync.service.d/lumonas.conf vsftpd.service.d/lumonas.conf; do
	[ -f "$ROOT/packaging/systemd/$dropin" ] || { echo "missing service drop-in: $dropin" >&2; exit 1; }
done
LUMONAS_REQUIRE_SYSTEMD_VERIFY="${LUMONAS_REQUIRE_SYSTEMD_VERIFY:-false}" \
	bash "$ROOT/scripts/systemd-smoke.sh"
LUMONAS_REQUIRE_SYSTEMD_SECURITY="${LUMONAS_REQUIRE_SYSTEMD_SECURITY:-false}" \
	bash "$ROOT/scripts/systemd-security-smoke.sh"
echo "LumoNAS packaging policy checks passed"
