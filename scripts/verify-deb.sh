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
command -v file >/dev/null 2>&1 || {
	 echo "file is required" >&2
	 exit 1
}
[ -f "$PACKAGE" ] || { echo "package not found: $PACKAGE" >&2; exit 1; }

CONTROL_DIR=$(mktemp -d "${TMPDIR:-/tmp}/lumonas-deb-control.XXXXXX")
DATA_DIR=$(mktemp -d "${TMPDIR:-/tmp}/lumonas-deb-data.XXXXXX")
cleanup() { rm -rf "$CONTROL_DIR" "$DATA_DIR"; }
trap cleanup EXIT INT TERM
dpkg-deb -e "$PACKAGE" "$CONTROL_DIR"
dpkg-deb -x "$PACKAGE" "$DATA_DIR"

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
	file "$DATA_DIR/usr/lib/lumonas/$binary" | grep -E 'ELF 64-bit.*x86-64' >/dev/null 2>&1 || {
		echo "$binary is not a Linux amd64 executable" >&2
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

[ -f "$DATA_DIR/etc/docker/daemon.json.lumonas" ] || {
	echo "Docker logging baseline is missing from the package" >&2
	exit 1
}
python3 - "$DATA_DIR/etc/docker/daemon.json.lumonas" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    config = json.load(handle)
assert config["log-driver"] == "json-file"
assert config["log-opts"] == {"max-size": "10m", "max-file": "3"}
PY

for path in \
	./usr/share/lumonas/web/index.html \
	./usr/share/lumonas/catalog/apps.json \
	./usr/share/lumonas/build-manifest.json \
	./etc/lumonas/lumonasd.env.example \
	./etc/lumonas/lumonas-web.env.example; do
	require_path "$path"
done

python3 - "$DATA_DIR/usr/share/lumonas/build-manifest.json" "$(field Version)" "$(field Depends)" "$(field Recommends)" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    manifest = json.load(handle)

if manifest.get("package") != "lumonas":
    raise SystemExit("manifest package mismatch")
if manifest.get("version") != sys.argv[2]:
    raise SystemExit("manifest version mismatch")
if manifest.get("architecture") != "amd64":
    raise SystemExit("manifest architecture mismatch")
if manifest.get("debianDepends") != sys.argv[3]:
    raise SystemExit("manifest Depends mismatch")
if manifest.get("debianRecommends") != sys.argv[4]:
    raise SystemExit("manifest Recommends mismatch")
for key in ("sourceCommit", "goVersion", "frontendLockSHA256", "catalogSHA256"):
    if not manifest.get(key):
        raise SystemExit(f"manifest field is empty: {key}")
PY

[ -x "$CONTROL_DIR/postinst" ] || { echo "package postinst is missing or not executable" >&2; exit 1; }

printf '%s\n' "$CONTENTS" | awk '{print $6}' | grep -E '^\./(var/lib/lumonas|srv/lumonas)' >/dev/null 2>&1 && {
	echo "package must not ship mutable runtime state" >&2
	exit 1
}

echo "LumoNAS Debian artifact verified: $PACKAGE"
