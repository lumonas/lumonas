#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}"
GOPATH="${GOPATH:-/tmp/lumonas-gopath}"
FUZZ_TIME="${LUMONAS_FUZZ_TIME:-5s}"
# Go spawns one fuzz worker per GOMAXPROCS by default. On a loaded CI runner
# that many workers contend for CPU, and a worker can be descheduled long
# enough for its execution to blow the fuzzing deadline, which is reported as
# a spurious "context deadline exceeded" failure. Bound the fan-out so each
# execution gets a fair share of the runner.
FUZZ_PARALLEL="${LUMONAS_FUZZ_PARALLEL:-2}"
export GOCACHE GOPATH

case "$FUZZ_TIME" in
	''|*[!0-9smh.]*) echo "LUMONAS_FUZZ_TIME must be a Go duration" >&2; exit 1 ;;
esac
case "$FUZZ_PARALLEL" in
	''|*[!0-9]*) echo "LUMONAS_FUZZ_PARALLEL must be a positive integer" >&2; exit 1 ;;
esac

cd "$ROOT"
run_fuzz() {
	package=$1
	target=$2
	echo "running $target in $package for $FUZZ_TIME (parallel $FUZZ_PARALLEL)"
	go test -count=1 -parallel "$FUZZ_PARALLEL" -run '^$' -fuzz "^${target}$" -fuzztime "$FUZZ_TIME" "$package"
}

run_fuzz ./internal/backup FuzzRemoteObjectValidation
run_fuzz ./internal/diagnostics FuzzRedactText
run_fuzz ./internal/diagnostics FuzzSupportBundleNames
run_fuzz ./internal/docker FuzzComposeStructure
run_fuzz ./internal/docker FuzzBuildCompose
run_fuzz ./internal/network FuzzWiFiPSKValidation
run_fuzz ./internal/network FuzzNetworkConnectionValidation
run_fuzz ./internal/recovery FuzzVerifyBundle
run_fuzz ./internal/recovery FuzzPlanBundle
run_fuzz ./internal/storage FuzzValidateSnapraidConfig
run_fuzz ./internal/storage FuzzRenderMountUnits
run_fuzz ./internal/storage FuzzStoragePlanValidation
run_fuzz ./internal/shares FuzzValidateGeneratedShareConfigs
run_fuzz ./internal/shares FuzzManagedShareValidation
echo "LumoNAS fuzz smoke checks passed"
