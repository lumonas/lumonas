#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
CONFIG="$ROOT/packaging/systemd/journald-lumonas.conf"
[ -f "$CONFIG" ] || { echo "journald retention configuration is missing" >&2; exit 1; }
DOCKER_CONFIG="$ROOT/packaging/docker-daemon.json"
[ -f "$DOCKER_CONFIG" ] || { echo "Docker logging configuration is missing" >&2; exit 1; }

for setting in SystemMaxUse=200M RuntimeMaxUse=100M MaxRetentionSec=30day MaxFileSec=7day; do
	grep -Fx "$setting" "$CONFIG" >/dev/null 2>&1 || {
		echo "missing journald retention setting: $setting" >&2
		exit 1
	}
done

python3 - "$DOCKER_CONFIG" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    config = json.load(handle)
assert config.get("log-driver") == "json-file"
assert config.get("log-opts", {}).get("max-size") == "10m"
assert config.get("log-opts", {}).get("max-file") == "3"
PY

echo "LumoNAS journald and Docker retention policies verified"
