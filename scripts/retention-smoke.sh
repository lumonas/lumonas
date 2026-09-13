#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
export GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}"
export GOPATH="${GOPATH:-/tmp/lumonas-gopath}"

cd "$ROOT"
go test -count=1 ./internal/store -run '^Test(PruneEventsKeepsNewestWindow|EventsAfterReplaysInInsertionOrder|OperationalRetentionKeepsActiveAndNewestHistory|PruneOperationalHistoryRunsWithDefaultPolicy|OperationalRetentionPrunesExpiredSessionsAndResolvedAlerts|PruneConfigGenerationsKeepsPendingAndNewestCommitted)$'
echo "operational retention smoke test passed"
