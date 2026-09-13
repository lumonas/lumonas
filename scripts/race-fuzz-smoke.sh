#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-race-cache}"
GOPATH="${GOPATH:-/tmp/lumonas-gopath}"
export GOCACHE GOPATH

cd "$ROOT"
go test -race ./cmd/lumonasd ./cmd/lumonas-privd ./internal/backup ./internal/diagnostics ./internal/docker ./internal/network ./internal/recovery ./internal/shares ./internal/storage ./internal/store
LUMONAS_FUZZ_TIME="${LUMONAS_FUZZ_TIME:-5s}" bash scripts/fuzz-smoke.sh
echo "LumoNAS race and fuzz smoke checks passed"
