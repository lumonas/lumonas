#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
CONFIG="$ROOT/packaging/systemd/journald-lumonas.conf"
[ -f "$CONFIG" ] || { echo "journald retention configuration is missing" >&2; exit 1; }

for setting in SystemMaxUse=200M RuntimeMaxUse=100M MaxRetentionSec=30day MaxFileSec=7day; do
	grep -Fx "$setting" "$CONFIG" >/dev/null 2>&1 || {
		echo "missing journald retention setting: $setting" >&2
		exit 1
	}
done

echo "LumoNAS journald retention policy verified"
