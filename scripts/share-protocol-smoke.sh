#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
export GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}"
export GOPATH="${GOPATH:-/tmp/lumonas-gopath}"
cd "$ROOT"

go test -count=1 ./internal/shares -run '^TestProtocolConfigValidators'
echo "share protocol validation smoke test passed"
