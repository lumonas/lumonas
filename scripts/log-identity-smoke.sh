#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
UNIT_DIR="$ROOT/packaging/systemd"

for unit in \
	lumonas-web.service lumonasd.service lumonas-privd.service \
	lumonas-privd-storage.service lumonas-privd-network.service \
	lumonas-privd-power.service lumonas-privd-general.service lumonas-privd-acme.service lumonas-runtime.service; do
	file="$UNIT_DIR/$unit"
	identifier=${unit%.service}
	[ -f "$file" ] || { echo "missing service unit: $unit" >&2; exit 1; }
	grep -Fx 'StandardOutput=journal' "$file" >/dev/null || {
		echo "$unit must send stdout to journald" >&2
		exit 1
	}
	grep -Fx 'StandardError=journal' "$file" >/dev/null || {
		echo "$unit must send stderr to journald" >&2
		exit 1
	}
	grep -Fx "SyslogIdentifier=$identifier" "$file" >/dev/null || {
		echo "$unit must use SyslogIdentifier=$identifier" >&2
		exit 1
	}
done

echo "LumoNAS service journal identifiers verified"
