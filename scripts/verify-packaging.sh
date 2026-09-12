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

for unit in mynas-web.service mynasd.service mynas-privd.service mynas-privd-storage.service mynas-privd-network.service mynas-privd-power.service mynas-privd-general.service; do
	[ -f "$SYSTEMD/$unit" ] || { echo "missing systemd unit: $unit" >&2; exit 1; }
	require_line "$SYSTEMD/$unit" 'NoNewPrivileges=true'
	require_line "$SYSTEMD/$unit" 'ProtectSystem=strict'
	require_line "$SYSTEMD/$unit" 'MemoryMax='
	require_line "$SYSTEMD/$unit" 'TasksMax='
done

require_line "$SYSTEMD/mynas-web.service" 'User=mynas'
require_line "$SYSTEMD/mynasd.service" 'User=mynas'
require_line "$SYSTEMD/mynas-privd.service" 'User=root'
require_line "$SYSTEMD/mynas-privd.service" 'CapabilityBoundingSet='
require_line "$SYSTEMD/mynasd.service" 'ReadWritePaths=/var/lib/mynas /srv/mynas'
require_line "$SYSTEMD/mynas-privd-storage.service" 'CapabilityBoundingSet=CAP_SYS_ADMIN CAP_SYS_RAWIO'
require_line "$SYSTEMD/mynas-privd-network.service" 'CapabilityBoundingSet=CAP_NET_ADMIN'
require_line "$SYSTEMD/mynas-privd-power.service" 'CapabilityBoundingSet=CAP_SYS_BOOT'
require_line "$SYSTEMD/mynas-privd-general.service" 'CapabilityBoundingSet=CAP_CHOWN CAP_DAC_OVERRIDE CAP_FOWNER CAP_SETGID CAP_SETUID'

require_line "$ROOT/packaging/debian/postinst" 'useradd --system'
require_line "$ROOT/packaging/debian/postinst" '/var/lib/mynas/secrets'
require_line "$ROOT/packaging/debian/postinst" '/etc/mynas/mynasd.env'
require_line "$ROOT/packaging/debian/postinst" 'MYNAS_WEB_ROOT=/usr/share/lumonas/web'
require_line "$ROOT/packaging/debian/postinst" '/etc/mynas/tls/server.key'
require_line "$ROOT/packaging/debian/postinst" 'mynas-privd.service'
require_line "$ROOT/installer/build-iso.sh" 'dpkg-scanpackages'
require_line "$ROOT/installer/build-iso.sh" 'lumonas-local.list'
echo "LumoNAS packaging policy checks passed"
