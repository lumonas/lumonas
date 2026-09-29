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

case "$FUZZ_PARALLEL" in
	''|*[!0-9]*) echo "LUMONAS_FUZZ_PARALLEL must be a positive integer" >&2; exit 1 ;;
esac

# duration_seconds converts a Go duration into fractional seconds, so the
# harness can tell a fuzzing-clock expiry from a defect found inside the budget.
# It handles the units the fuzz budget realistically uses: h, m, s, and ms.
# Prints nothing and fails on anything it does not understand, which also makes
# it the validator for LUMONAS_FUZZ_TIME.
duration_seconds() {
	rest=$1
	[ -n "$rest" ] || return 1
	total=0
	saw_unit=0
	while [ -n "$rest" ]; do
		number=${rest%%[!0-9.]*}
		if [ -z "$number" ]; then
			return 1
		fi
		rest=${rest#"$number"}
		case "$number" in
			*.*.* | .* | *.) return 1 ;;
		esac
		unit=${rest%%[!smh.]*}
		rest=${rest#"$unit"}
		case "$unit" in
			h) total=$(awk -v t="$total" -v n="$number" 'BEGIN { printf "%.3f", t + n * 3600 }') ;;
			m) total=$(awk -v t="$total" -v n="$number" 'BEGIN { printf "%.3f", t + n * 60 }') ;;
			s) total=$(awk -v t="$total" -v n="$number" 'BEGIN { printf "%.3f", t + n }') ;;
			ms) total=$(awk -v t="$total" -v n="$number" 'BEGIN { printf "%.3f", t + n / 1000 }') ;;
			*) return 1 ;;
		esac
		saw_unit=1
	done
	[ "$saw_unit" = 1 ] || return 1
	printf '%s' "$total"
}

if ! budget_seconds=$(duration_seconds "$FUZZ_TIME"); then
	echo "LUMONAS_FUZZ_TIME must be a Go duration such as 30s, 5m, or 1m30s" >&2
	exit 1
fi
case "$budget_seconds" in
	0 | 0.0* | 0.00*) echo "LUMONAS_FUZZ_TIME must be greater than zero" >&2; exit 1 ;;
esac

cd "$ROOT"

# is_harness_timeout reports whether a failing run stopped because the fuzzer
# ran out of time, rather than because the code under test misbehaved.
#
# When -fuzztime expires, the fuzzing coordinator cancels the context its
# workers share. A worker that is part way through an execution is torn down
# with it, and the run is reported as
#
#     --- FAIL: FuzzX (6.00s)
#         context deadline exceeded
#
# which is the harness clock, not a finding. A real defect is different in kind:
# it prints a stack trace, a data race report, or a differential between the
# fuzzed and non-fuzzed run. The duration also gives it away, because it lands
# on the budget rather than inside it.
is_harness_timeout() {
	output=$1
	target=$2

	case "$output" in
		*"context deadline exceeded"*) ;;
		*) return 1 ;;
	esac
	# A crash, race, or differential is a real finding regardless of the clock.
	case "$output" in
		*"DATA RACE"* | *"panic:"* | *"failing input"* | *"crash:"*) return 1 ;;
	esac

	# The discriminator: a harness timeout lands on the fuzzing budget, because
	# that is when the coordinator cancels the workers. A defect trips inside
	# it. Compare the reported duration against the budget rather than trusting
	# the message alone.
	reported=$(echo "$output" | sed -n "s/^--- FAIL: ${target} (\([0-9][0-9.]*\)s).*/\1/p" | head -1)
	[ -n "$reported" ] || return 1
	budget=$(duration_seconds "$FUZZ_TIME") || return 1
	# Allow a small tolerance: Go reports the whole test, budget plus setup.
	awk -v got="$reported" -v want="$budget" 'BEGIN { exit !(got + 1.0 >= want) }' || return 1
	return 0
}

run_fuzz() {
	package=$1
	target=$2
	echo "running $target in $package for $FUZZ_TIME (parallel $FUZZ_PARALLEL)"

	attempts=0
	while [ "$attempts" -lt 2 ]; do
		attempts=$((attempts + 1))
		output="$(go test -count=1 -parallel "$FUZZ_PARALLEL" -run '^$' \
			-fuzz "^${target}$" -fuzztime "$FUZZ_TIME" "$package" 2>&1)" && {
			echo "$output"
			return 0
		}
		echo "$output"

		# Only the harness clock is retried, and only once. A target that stops
		# the same way twice in a row is a real failure, as is anything that
		# failed for a different reason.
		if [ "$attempts" -ge 2 ] || ! is_harness_timeout "$output" "$target"; then
			echo "fuzz target failed: $target ($package)" >&2
			return 1
		fi
		echo "NOTE: $target stopped when the fuzzing budget expired rather than" >&2
		echo "      reporting a finding; retrying once to separate the harness" >&2
		echo "      clock from a real defect." >&2
	done
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
