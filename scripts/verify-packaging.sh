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

require_line "$SYSTEMD/lumonas-web.service" 'User=lumonas'
require_line "$SYSTEMD/lumonasd.service" 'User=lumonas'
require_line "$SYSTEMD/lumonas-privd.service" 'User=root'
require_line "$SYSTEMD/lumonas-privd.service" 'CapabilityBoundingSet='
require_line "$SYSTEMD/lumonasd.service" 'ReadWritePaths=/var/lib/lumonas /srv/lumonas'
require_line "$SYSTEMD/lumonas-privd-storage.service" 'CapabilityBoundingSet=CAP_SYS_ADMIN CAP_SYS_RAWIO'
require_line "$SYSTEMD/lumonas-privd-network.service" 'CapabilityBoundingSet=CAP_NET_ADMIN'
require_line "$SYSTEMD/lumonas-privd-power.service" 'CapabilityBoundingSet=CAP_SYS_BOOT'
require_line "$SYSTEMD/lumonas-privd-general.service" 'CapabilityBoundingSet=CAP_CHOWN CAP_DAC_OVERRIDE CAP_FOWNER CAP_SETGID CAP_SETUID'

require_line "$ROOT/packaging/debian/postinst" 'useradd --system'
require_line "$ROOT/packaging/debian/postinst" '/var/lib/lumonas/secrets'
require_line "$ROOT/packaging/debian/postinst" '/etc/lumonas/lumonasd.env'
require_line "$ROOT/packaging/debian/postinst" 'LUMONAS_WEB_ROOT=/usr/share/lumonas/web'
require_line "$ROOT/packaging/debian/postinst" '/etc/lumonas/tls/server.key'
require_line "$ROOT/packaging/debian/postinst" 'lumonas-privd.service'
require_line "$ROOT/installer/build-iso.sh" 'dpkg-scanpackages'
require_line "$ROOT/installer/build-iso.sh" 'lumonas-local.list'
if command -v systemd-analyze >/dev/null 2>&1; then
	systemd-analyze verify "$SYSTEMD"/*.service
fi
echo "LumoNAS packaging policy checks passed"
