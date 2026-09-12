#!/bin/sh
set -eu

PACKAGE=${1:-}
if [ -z "$PACKAGE" ]; then
	echo "usage: $0 PACKAGE.deb" >&2
	 exit 2
fi
command -v dpkg-deb >/dev/null 2>&1 || {
	 echo "dpkg-deb is required" >&2
	 exit 1
}
[ -f "$PACKAGE" ] || { echo "package not found: $PACKAGE" >&2; exit 1; }

CONTROL_DIR=$(mktemp -d "${TMPDIR:-/tmp}/lumonas-deb-control.XXXXXX")
cleanup() { rm -rf "$CONTROL_DIR"; }
trap cleanup EXIT INT TERM
dpkg-deb -e "$PACKAGE" "$CONTROL_DIR"

field() {
	dpkg-deb -f "$PACKAGE" "$1"
}

[ "$(field Package)" = "lumonas" ] || { echo "unexpected package name" >&2; exit 1; }
[ "$(field Architecture)" = "amd64" ] || { echo "unexpected package architecture" >&2; exit 1; }
[ -n "$(field Version)" ] || { echo "package version is empty" >&2; exit 1; }

CONTENTS=$(dpkg-deb -c "$PACKAGE")
require_path() {
	path=$1
	printf '%s\n' "$CONTENTS" | awk '{print $6}' | grep -Fx "$path" >/dev/null 2>&1 || {
		echo "package is missing $path" >&2
		exit 1
	}
}

for binary in lumonasd lumonas-web lumonas-privd lumonas-recover; do
	require_path "./usr/lib/lumonas/$binary"
	dpkg-deb -c "$PACKAGE" | awk -v path="./usr/lib/lumonas/$binary" '$6 == path { print $1 }' | grep -E '^-rwx' >/dev/null 2>&1 || {
		echo "$binary is not executable in the package" >&2
		exit 1
	}
done

for unit in \
	lumonas-web.service \
	lumonasd.service \
	lumonas-privd.service \
	lumonas-privd-storage.service \
	lumonas-privd-network.service \
	lumonas-privd-power.service \
	lumonas-privd-general.service; do
	require_path "./lib/systemd/system/$unit"
done

for path in \
	./usr/share/lumonas/web/index.html \
	./usr/share/lumonas/catalog/apps.json \
	./etc/lumonas/lumonasd.env.example \
	./etc/lumonas/lumonas-web.env.example; do
	require_path "$path"
done

[ -x "$CONTROL_DIR/postinst" ] || { echo "package postinst is missing or not executable" >&2; exit 1; }

printf '%s\n' "$CONTENTS" | awk '{print $6}' | grep -E '^\./(var/lib/lumonas|srv/lumonas)' >/dev/null 2>&1 && {
	echo "package must not ship mutable runtime state" >&2
	exit 1
}

echo "LumoNAS Debian artifact verified: $PACKAGE"
