#!/bin/sh
set -eu

PACKAGE=${1:-}
if [ -z "$PACKAGE" ]; then
	echo "usage: $0 PACKAGE.deb" >&2
	exit 2
fi
[ -f "$PACKAGE" ] || { echo "package not found: $PACKAGE" >&2; exit 1; }

if command -v docker >/dev/null 2>&1; then
	RUNTIME=docker
elif command -v podman >/dev/null 2>&1; then
	RUNTIME=podman
else
	echo "docker or podman is required for package permission smoke testing" >&2
	exit 1
fi

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
PACKAGE_NAME=$(basename "$PACKAGE")

"$RUNTIME" run --rm \
	-v "$ROOT:/packages:ro" \
	-e PACKAGE_NAME="$PACKAGE_NAME" \
	debian:trixie-slim \
	/bin/sh -euxc '
apt-get update
apt-get install -y --no-install-recommends ca-certificates openssl passwd systemd util-linux
dpkg -i "/packages/$PACKAGE_NAME"

[ "$(stat -c "%U:%G:%a" /etc/lumonas)" = "root:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /etc/lumonas/lumonasd.env)" = "root:lumonas:640" ]
[ "$(stat -c "%U:%G:%a" /etc/lumonas/lumonas-web.env)" = "root:lumonas:640" ]
[ "$(stat -c "%U:%G:%a" /etc/lumonas/runtime.env)" = "root:lumonas:640" ]
[ "$(stat -c "%U:%G:%a" /var/lib/lumonas)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /var/lib/lumonas/secrets)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /var/lib/lumonas/recovery)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /srv/lumonas)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /srv/disks)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /srv/pools)" = "lumonas:lumonas:750" ]

runuser -u lumonas -- sh -eu -c "
	test -r /etc/lumonas/lumonasd.env
	test -r /etc/lumonas/lumonas-web.env
	test -r /etc/lumonas/runtime.env
	test ! -w /etc/lumonas/lumonasd.env
	test ! -w /etc/lumonas/lumonas-web.env
	test ! -w /etc/lumonas/runtime.env
	test ! -w /etc/lumonas
	touch /var/lib/lumonas/.permission-smoke
	touch /var/lib/lumonas/recovery/.permission-smoke
	touch /srv/lumonas/.permission-smoke
	touch /srv/disks/.permission-smoke
	touch /srv/pools/.permission-smoke
	rm -f /var/lib/lumonas/.permission-smoke /var/lib/lumonas/recovery/.permission-smoke /srv/lumonas/.permission-smoke /srv/disks/.permission-smoke /srv/pools/.permission-smoke
"

test "$(id -u lumonas)" -ne 0
test "$(id -g lumonas)" -ne 0
systemctl list-unit-files lumonas-web.service lumonasd.service >/dev/null
dpkg --audit
'

echo "LumoNAS package permission smoke test passed: $PACKAGE_NAME"
