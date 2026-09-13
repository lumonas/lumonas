#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}"
GOPATH="${GOPATH:-/tmp/lumonas-gopath}"

GOCACHE="$GOCACHE" GOPATH="$GOPATH" go test -count=1 ./internal/recovery -run 'TestPersistVerified(PublishesAtomicLatestAndVersionedCopies|RejectsTamperedBundleBeforeCreatingDirectory)$'
echo "LumoNAS recovery persistence smoke passed"
