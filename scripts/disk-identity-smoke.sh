#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}"
GOPATH="${GOPATH:-/tmp/lumonas-gopath}"
export GOCACHE GOPATH

cd "$ROOT"
go test -count=1 ./internal/collector -run 'Test(DisksUseStableIdentityAcrossDevicePathChanges|DisksPromoteMountedPartitionMetadata|MountedPartitionWinsOverUnmountedFilesystemChild|StableIDFallsBackInSafeOrder|ParseUdevPropertiesIgnoresMalformedLines|EnrichFromUdevFillsMissingStableIdentity|EnrichFromUdevPreservesLsblkIdentity)$'
echo "LumoNAS disk identity collector smoke checks passed"
