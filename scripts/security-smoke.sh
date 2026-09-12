#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

# Keep this deliberately high-confidence. Documentation and unit tests contain
# words such as "secret" by design; this gate looks for credential formats that
# should never be committed to the repository.
if git -C "$ROOT" grep -nE -- \
	'-----BEGIN (RSA|EC|OPENSSH|DSA|PGP) PRIVATE KEY-----|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|xox[baprs]-[0-9A-Za-z-]{20,}' \
	-- .; then
	echo "high-confidence credential material found in tracked files" >&2
	exit 1
fi

cd "$ROOT"
go test -count=1 ./internal/diagnostics ./internal/privileged ./internal/runner ./cmd/lumonas-privd \
	-run 'TestRedactionRemovesSecretCanaries|TestBundleRejectsUnsafeNamesAndRedactsText|TestPrivilegedProtocolRejectsUnknownOperation|TestWorkerRejectsOperationsOutsideItsCapabilityDomain|TestExecuteRejectsStaleIdentity|TestClientDeadlineInterruptsPendingResponse|TestCommandRunnerKillsDescendantsAfterTimeout|TestCombinedOutputContextKillsDescendantsOnCancellation|TestWaitProcessGroupKillsNetworkCheckpointDescendants|TestOutputContextRejectsExcessiveOutput|TestCombinedOutputRejectsExcessiveOutput'
echo "LumoNAS security smoke checks passed"
