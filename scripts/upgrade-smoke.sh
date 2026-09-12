#!/bin/sh
set -eu

OLD_PACKAGE="${1:-}"
NEW_PACKAGE="${2:-}"
if [ -z "$OLD_PACKAGE" ] || [ -z "$NEW_PACKAGE" ]; then
	echo "usage: $0 OLD_PACKAGE.deb NEW_PACKAGE.deb" >&2
	exit 2
fi
[ -f "$OLD_PACKAGE" ] || { echo "old package not found: $OLD_PACKAGE" >&2; exit 1; }
[ -f "$NEW_PACKAGE" ] || { echo "new package not found: $NEW_PACKAGE" >&2; exit 1; }

if command -v docker >/dev/null 2>&1; then
	RUNTIME=docker
elif command -v podman >/dev/null 2>&1; then
	RUNTIME=podman
else
	echo "docker or podman is required for Debian upgrade smoke testing" >&2
	exit 1
fi

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OLD_NAME=$(basename "$OLD_PACKAGE")
NEW_NAME=$(basename "$NEW_PACKAGE")
EXPECTED_VERSION=$(dpkg-deb -f "$NEW_PACKAGE" Version)

"$RUNTIME" run --rm \
	-v "$ROOT:/packages:ro" \
	-e OLD_PACKAGE="$OLD_NAME" \
	-e NEW_PACKAGE="$NEW_NAME" \
	-e EXPECTED_VERSION="$EXPECTED_VERSION" \
	debian:trixie-slim \
	/bin/sh -euxc '
apt-get update
apt-get install -y --no-install-recommends systemd passwd ca-certificates
dpkg -i "/packages/$OLD_PACKAGE"
printf "%s\n" "LumoNAS administrator marker" >>/etc/lumonas/lumonasd.env
dpkg -i "/packages/$NEW_PACKAGE"
test "$(dpkg-query -W -f="${Version}" lumonas)" = "$EXPECTED_VERSION"
grep -Fx "LumoNAS administrator marker" /etc/lumonas/lumonasd.env
test -d /var/lib/lumonas/recovery
test -d /srv/lumonas
test -f /etc/docker/daemon.json
test -x /usr/lib/lumonas/lumonasd
test -x /usr/lib/lumonas/lumonas-recover
test -f /lib/systemd/system/lumonasd.service
test -f /lib/systemd/system/lumonas-web.service
dpkg --audit
'

echo "Debian package upgrade smoke test passed: $OLD_NAME -> $NEW_NAME"
