#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
POSTINST="$ROOT/packaging/debian/postinst"

grep -F 'if [ -d /run/systemd/system ] && command -v systemctl' "$POSTINST" >/dev/null
grep -F 'systemctl start "$unit"' "$POSTINST" >/dev/null
grep -F 'systemctl start lumonas' "$POSTINST" >/dev/null 2>&1 && {
	echo "postinst unexpectedly hard-coded a non-ordered service start" >&2
	exit 1
} || true

python3 - "$POSTINST" <<'PY'
import pathlib
import sys

text = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
live = text.split('if [ -d /run/systemd/system ]', 1)[1].split('else', 1)[0]
if 'systemctl start "$unit" || true' in live:
    raise SystemExit("live systemd postinst still ignores service start failures")
if 'systemctl enable ' not in live or 'systemctl daemon-reload' not in live:
    raise SystemExit("live systemd postinst does not reload and enable the service graph")
print("LumoNAS postinst systemd failure policy passed")
PY
