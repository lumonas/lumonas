#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}"
export GOCACHE

command -v go >/dev/null 2>&1 || { echo "go is required for recovery API smoke testing" >&2; exit 1; }
(
	cd "$ROOT"
	go test ./cmd/lumonasd -run '^(TestRecoveryExportAndApplyConfiguredRuntime|TestRecoveryExportFailsClosedWhenDiskIdentityCollectionFails|TestRecoveryExportRequiresNASIdentity)$' -count=1
)
echo "LumoNAS configured recovery API smoke test passed"
