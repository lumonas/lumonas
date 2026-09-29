#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
python3 - "$ROOT/packaging/debian/prerm" "$ROOT/packaging/debian/postinst" "$ROOT/packaging/build-deb.sh" <<'PY'
import pathlib
import sys

prerm = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
postinst = pathlib.Path(sys.argv[2]).read_text(encoding="utf-8")
build_deb = pathlib.Path(sys.argv[3]).read_text(encoding="utf-8")

def assert_order(source, names, label):
    positions = []
    for name in names:
        position = source.find(name)
        if position < 0:
            raise SystemExit(f"{label} is missing {name}")
        positions.append(position)
    if positions != sorted(positions):
        raise SystemExit(f"{label} dependency order is invalid")

assert_order(prerm, [
    "lumonas-web.service", "lumonas-jobs.target", "lumonas-services.target",
    "lumonas-storage.target", "lumonasd.service",
    "lumonas-runtime.service",
    "lumonas-privd-acme.service", "lumonas-privd-general.service", "lumonas-privd-power.service",
    "lumonas-privd-network.service", "lumonas-privd-storage.service",
    "lumonas-privd.service",
], "upgrade stop")
assert_order(postinst, [
	"runuser -u lumonas -- /usr/lib/lumonas/lumonas-migrate",
	"systemctl daemon-reload",
	"lumonas-runtime.service",
    "lumonas-privd.service", "lumonas-privd-storage.service",
    "lumonas-privd-network.service", "lumonas-privd-power.service",
    "lumonas-privd-general.service", "lumonas-privd-acme.service", "lumonas-jobs.target",
    "lumonas-services.target", "lumonas-storage.target", "lumonasd.service",
    "lumonas-web.service",
], "upgrade start")
live_prerm = prerm.split('if [ -d /run/systemd/system ]', 1)[1].split('else', 1)[0]
if 'systemctl stop "$unit" || true' in live_prerm:
    raise SystemExit("live systemd upgrade stop still ignores service failures")
if 'systemctl stop "$unit"' not in live_prerm:
    raise SystemExit("live systemd upgrade stop path is missing")
if 'cp "$ROOT/packaging/debian/prerm" "$OUT/DEBIAN/prerm"' not in build_deb:
    raise SystemExit("Debian build does not package prerm")
PY
echo "LumoNAS upgrade service ordering checks passed"
