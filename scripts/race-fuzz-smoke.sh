#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-race-cache}"
GOPATH="${GOPATH:-/tmp/lumonas-gopath}"
export GOCACHE GOPATH

cd "$ROOT"
# Race every Go package so a release cannot hide a concurrency regression in
# a collector, event hub, migration, or command service omitted from a hand-
# maintained package allow-list.
go test -race ./...
LUMONAS_FUZZ_TIME="${LUMONAS_FUZZ_TIME:-5s}" bash scripts/fuzz-smoke.sh
echo "LumoNAS race and fuzz smoke checks passed"
