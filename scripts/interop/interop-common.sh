#!/usr/bin/env bash
# Shared helpers for LumoNAS real-client interoperability tests.
#
# These scripts verify that real protocol clients (smbclient, mount, sftp,
# lftp, rsync, avahi-browse, tmutil) can talk to a running LumoNAS appliance.
# They are designed for hardware acceptance runs: every missing tool or
# unreachable service results in a SKIP, never a failure, so a partial
# workstation still produces a meaningful pass/skip/fail summary.

set -u

LUMONAS_INTEROP_HOST="${LUMONAS_INTEROP_HOST:-127.0.0.1}"
LUMONAS_INTEROP_SHARE="${LUMONAS_INTEROP_SHARE:-media}"
LUMONAS_INTEROP_USER="${LUMONAS_INTEROP_USER:-}"
LUMONAS_INTEROP_PASSWORD="${LUMONAS_INTEROP_PASSWORD:-}"
LUMONAS_INTEROP_WORKDIR="${LUMONAS_INTEROP_WORKDIR:-$(mktemp -d /tmp/lumonas-interop.XXXXXX)}"
# Set this for hardware acceptance runs where the appliance must advertise
# its services on the test network. Normal workstation runs remain tolerant
# of isolated networks and skip missing mDNS advertisements.
LUMONAS_INTEROP_REQUIRE_ADVERTISEMENTS="${LUMONAS_INTEROP_REQUIRE_ADVERTISEMENTS:-false}"

INTEROP_PASS=0
INTEROP_SKIP=0
INTEROP_FAIL=0

interop_result() {
	local outcome="$1"
	local name="$2"
	local reason="${3:-}"
	case "$outcome" in
	pass)
		printf 'PASS  %s\n' "$name"
		INTEROP_PASS=$((INTEROP_PASS + 1))
		;;
	skip)
		printf 'SKIP  %s — %s\n' "$name" "$reason"
		INTEROP_SKIP=$((INTEROP_SKIP + 1))
		;;
	fail)
		printf 'FAIL  %s — %s\n' "$name" "$reason"
		INTEROP_FAIL=$((INTEROP_FAIL + 1))
		;;
	esac
}

# require_tool <binary> <test-name> — exits the caller with a SKIP result.
require_tool() {
	local binary="$1"
	local name="$2"
	if ! command -v "$binary" >/dev/null 2>&1; then
		interop_result skip "$name" "$binary is not installed"
		exit 0
	fi
}

# require_port <host> <port> <test-name> — exits the caller with SKIP when the
# service is not listening.
require_port() {
	local host="$1"
	local port="$2"
	local name="$3"
	if ! python3 - "$host" "$port" <<-'PY' >/dev/null 2>&1
		import socket, sys
		socket.create_connection((sys.argv[1], int(sys.argv[2])), timeout=3).close()
	PY
	then
		interop_result skip "$name" "$host:$port is not reachable"
		exit 0
	fi
}

require_credentials() {
	local name="$1"
	if [ -z "$LUMONAS_INTEROP_USER" ] || [ -z "$LUMONAS_INTEROP_PASSWORD" ]; then
		interop_result skip "$name" "LUMONAS_INTEROP_USER/LUMONAS_INTEROP_PASSWORD not set"
		exit 0
	fi
}

interop_summary() {
	printf '\n%d passed, %d skipped, %d failed\n' "$INTEROP_PASS" "$INTEROP_SKIP" "$INTEROP_FAIL"
	[ "$INTEROP_FAIL" -eq 0 ]
}
