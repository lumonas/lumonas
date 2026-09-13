#!/bin/sh
set -eu

# LumoNAS local development launcher.
#
#   scripts/dev.sh        # frontend only, served by the MSW mock backend (default)
#   scripts/dev.sh full   # lumonasd + lumonas-privd workers + frontend on the real API
#
# The frontend runs last and in the foreground: Ctrl-C stops everything.
# Logs for the background processes land in build/dev/logs/.

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DEV_DIR=${LUMONAS_DEV_DIR:-"$ROOT_DIR/build/dev"}
LOG_DIR="$DEV_DIR/logs"
BIN_DIR="$DEV_DIR/bin"
API_ADDR=${LUMONASD_LISTEN:-127.0.0.1:8080}
API_URL="http://$API_ADDR"
PRIVD_DIR="$DEV_DIR/privd"

LUMONASD_PID=''
PRIVD_PID=''
WORKER_PIDS=''

usage() {
	echo "usage: $0 [mock|full]" >&2
	exit 2
}

cleanup() {
	for pid in $WORKER_PIDS $PRIVD_PID $LUMONASD_PID; do
		kill "$pid" 2>/dev/null || true
	done
	for pid in $WORKER_PIDS $PRIVD_PID $LUMONASD_PID; do
		wait "$pid" 2>/dev/null || true
	done
}

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "dev environment requires '$1'" >&2
		exit 1
	fi
}

wait_for() {
	url=$1
	name=$2
	pid=${3:-}
	attempt=0
	while [ "$attempt" -lt 150 ]; do
		status=$(curl -sS -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)
		if [ "$status" = "200" ]; then
			return 0
		fi
		if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
			echo "$name exited before $url became ready; last log lines:" >&2
			tail -n 20 "$LOG_DIR/lumonasd.log" >&2 2>/dev/null || true
			return 1
		fi
		attempt=$((attempt + 1))
		sleep 0.2
	done
	echo "timed out waiting for $name at $url" >&2
	return 1
}

MODE=${1:-mock}
[ $# -le 1 ] || usage

case "$MODE" in
mock)
	require_command pnpm
	if [ ! -d "$ROOT_DIR/web/node_modules" ]; then
		echo "installing web dependencies..."
		(cd "$ROOT_DIR/web" && pnpm install)
	fi
	echo "frontend on http://localhost:5173 (mock backend, login: admin + any 4+ char password)"
	cd "$ROOT_DIR/web"
	exec pnpm dev
	;;
full)
	require_command go
	require_command curl
	require_command pnpm
	cd "$ROOT_DIR"
	mkdir -p "$DEV_DIR" "$LOG_DIR" "$PRIVD_DIR" "$BIN_DIR"

	if [ ! -d "$ROOT_DIR/web/node_modules" ]; then
		echo "installing web dependencies..."
		(cd "$ROOT_DIR/web" && pnpm install)
	fi

	echo "building binaries into $BIN_DIR..."
	GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" \
		GOPATH="${GOPATH:-/tmp/lumonas-gopath}" \
		go build -o "$BIN_DIR/lumonasd" ./cmd/lumonasd
	GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" \
		GOPATH="${GOPATH:-/tmp/lumonas-gopath}" \
		go build -o "$BIN_DIR/lumonas-privd" ./cmd/lumonas-privd

	trap cleanup EXIT INT TERM

	for worker in storage network power general; do
		LUMONAS_PRIVD_WORKER_DIR="$PRIVD_DIR" \
			"$BIN_DIR/lumonas-privd" -worker "$worker" -socket "$PRIVD_DIR/$worker.sock" \
			>"$LOG_DIR/privd-$worker.log" 2>&1 &
		WORKER_PIDS="$WORKER_PIDS $!"
	done

	# Workers create their sockets on startup; a missing socket means the
	# worker crashed (see build/dev/logs/privd-*.log).
	attempt=0
	while [ "$attempt" -lt 25 ]; do
		ready=1
		for worker in storage network power general; do
			[ -S "$PRIVD_DIR/$worker.sock" ] || ready=0
		done
		[ "$ready" = "1" ] && break
		attempt=$((attempt + 1))
		sleep 0.2
	done
	if [ "$ready" != "1" ]; then
		echo "privileged workers did not start; last log lines:" >&2
		cat "$LOG_DIR"/privd-*.log >&2
		exit 1
	fi

	LUMONAS_PRIVD_WORKER_DIR="$PRIVD_DIR" \
		"$BIN_DIR/lumonas-privd" -socket "$PRIVD_DIR/privd.sock" \
		>"$LOG_DIR/privd-broker.log" 2>&1 &
	PRIVD_PID=$!

	LUMONAS_DB_PATH="$DEV_DIR/lumonas.db" \
		LUMONAS_AUTH_REQUIRED="${LUMONAS_AUTH_REQUIRED:-}" \
		LUMONAS_ADMIN_PASSWORD="${LUMONAS_ADMIN_PASSWORD:-dev-password-123}" \
		LUMONAS_CATALOG_FILE="$ROOT_DIR/catalog/apps.json" \
		LUMONAS_STACK_ROOT="$DEV_DIR/stacks" \
		LUMONAS_RECOVERY_DIR="$DEV_DIR/recovery" \
		LUMONAS_PRIVD_SOCKET="$PRIVD_DIR/privd.sock" \
		"$BIN_DIR/lumonasd" -listen "$API_ADDR" \
		>"$LOG_DIR/lumonasd.log" 2>&1 &
	LUMONASD_PID=$!

	wait_for "$API_URL/readyz" "lumonasd" "$LUMONASD_PID"
	echo "backend  on $API_URL (db: $DEV_DIR/lumonas.db)"
	echo "workers  privd broker + storage/network/power/general on $PRIVD_DIR"
	echo "frontend on http://localhost:5173 (real API via proxy, logs: $LOG_DIR)"

	cd "$ROOT_DIR/web"
	# Foreground child (not exec): the EXIT trap must survive to stop the
	# backend and workers when the dev server exits.
	VITE_USE_MOCKS=false pnpm dev
	;;
*)
	usage
	;;
esac
