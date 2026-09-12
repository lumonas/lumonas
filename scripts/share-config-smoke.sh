#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
if grep -F '/etc/lumonas/tls/tls.crt' "$ROOT/cmd/lumonasd/share_configs.go" >/dev/null 2>&1 || grep -F '/etc/lumonas/tls/tls.key' "$ROOT/cmd/lumonasd/share_configs.go" >/dev/null 2>&1; then
	echo "obsolete FTPS certificate path found" >&2
	exit 1
fi
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" go test ./cmd/lumonasd ./internal/shares -run 'Test(FTP|PrepareShareConfigs|ProtocolRenderers)'
echo "LumoNAS share configuration smoke checks passed"
