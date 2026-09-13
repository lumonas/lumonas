#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

python3 "$ROOT/scripts/secret-scan.py" --root "$ROOT"

cd "$ROOT"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" \
	go test -count=1 ./internal/diagnostics ./internal/privileged ./internal/runner ./cmd/lumonas-privd ./cmd/lumonasd \
	-run 'TestRedactionRemovesSecretCanaries|TestBundleRejectsUnsafeNamesAndRedactsText|TestPrivilegedProtocolRejectsUnknownOperation|TestWorkerRejectsOperationsOutsideItsCapabilityDomain|TestExecuteRejectsStaleIdentity|TestClientDeadlineInterruptsPendingResponse|TestCommandRunnerKillsDescendantsAfterTimeout|TestCombinedOutputContextKillsDescendantsOnCancellation|TestWaitProcessGroupKillsNetworkCheckpointDescendants|TestOutputContextRejectsExcessiveOutput|TestCombinedOutputRejectsExcessiveOutput|TestSendChannelRejectsOversizedProviderResponse|TestSupportBundleReportsUnavailableCollections|TestNotificationCredentialFailureIsPersistedAsDeliveryFailure|TestNotificationFailureWindowAccumulatesBeforeSuppression|TestNetworkCheckpointRollsBackWhenPersistenceFails'
echo "LumoNAS security smoke checks passed"
