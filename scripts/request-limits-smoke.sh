#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}"
GOPATH="${GOPATH:-/tmp/lumonas-gopath}"
export GOCACHE GOPATH

cd "$ROOT"
go test -count=1 ./cmd/lumonasd -run '^TestRequestMiddleware(RejectsOversizedJSONBeforeHandler|RejectsOversizedMultipartBeforeHandler|DoesNotParseMultipartBeforeHandler)$'
echo "LumoNAS request-size smoke checks passed"
