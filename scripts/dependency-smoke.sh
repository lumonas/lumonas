#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
REQUIRE_TOOLS="${LUMONAS_REQUIRE_DEPENDENCY_TOOLS:-false}"

# is_transport_failure reports whether a govulncheck run stopped because a
# module could not be fetched, rather than because it found something.
#
# govulncheck loads source through the module cache, and on a fresh runner that
# cache is empty, so the scan reaches out to the module proxy mid-run. The proxy
# occasionally answers with an HTTP/2 INTERNAL_ERROR part way through a module
# zip, which surfaces as a package pattern error and a non-zero exit. That is a
# transport failure, and the scan never happened, so it must not be reported as
# a clean result either.
#
# Integrity failures are the opposite case and are never retried: a checksum
# mismatch or a SECURITY ERROR means the fetched module is not the module the
# project asked for.
is_transport_failure() {
	output=$1
	case "$output" in
		*checksum\ mismatch* | *SECURITY\ ERROR* | *"checksum mismatch"* | *"verifying module"* | *malformed\ go.mod* | *SECURITY\ ERROR*) return 1 ;;
	esac
	case "$output" in
		*"proxy.golang.org"* | *"dial tcp"* | *"connection reset"* | *"stream error"* | *INTERNAL_ERROR* | *"TLS handshake"* | *"unexpected EOF"* | *"EOF"* | *"context deadline exceeded"* | *"i/o timeout"* | *"429 Too Many Requests"* | *"502 Bad Gateway"* | *"503 Service Unavailable"* | *"server gave HTTP response to HTTPS client"* | *"Bad Gateway"*) return 0 ;;
	esac
	return 1
}

if command -v govulncheck >/dev/null 2>&1; then
	cd "$ROOT"
	# Populate the module cache up front so the scan itself is not competing
	# with module downloads, and so a throttled proxy is retried by the Go
	# command, which knows how to walk GOPROXY, rather than by us.
	attempts=0
	while [ "$attempts" -lt 3 ]; do
		attempts=$((attempts + 1))
		if go mod download 2>&1; then
			break
		fi
		echo "go mod download failed (attempt $attempts of 3); retrying" >&2
		if [ "$attempts" -ge 3 ]; then
			echo "could not populate the Go module cache" >&2
			exit 1
		fi
		sleep 5
	done

	attempts=0
	while :; do
		attempts=$((attempts + 1))
		output="$(govulncheck ./... 2>&1)" && {
			echo "$output"
			break
		}
		echo "$output"
		if [ "$attempts" -ge 2 ] || ! is_transport_failure "$output"; then
			exit 1
		fi
		echo "NOTE: the module proxy failed while govulncheck was loading" >&2
		echo "      source, so no result was produced. Retrying once after" >&2
		echo "      re-downloading the module cache." >&2
		go mod download 2>&1 || true
	done
elif [ "$REQUIRE_TOOLS" = "true" ]; then
	echo "govulncheck is required for the dependency gate" >&2
	exit 1
else
	echo "govulncheck is not installed; Go vulnerability scan skipped" >&2
fi

if [ "${LUMONAS_SKIP_NPM_AUDIT:-false}" = "true" ]; then
	echo "pnpm audit explicitly skipped"
elif command -v pnpm >/dev/null 2>&1; then
	cd "$ROOT/web"
	pnpm audit --prod --audit-level high
else
	if [ "$REQUIRE_TOOLS" = "true" ]; then
		echo "pnpm is required for the frontend dependency gate" >&2
		exit 1
	fi
	echo "pnpm is not installed; frontend dependency scan skipped" >&2
fi

echo "LumoNAS dependency checks passed"
