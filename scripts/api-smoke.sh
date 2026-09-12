#!/bin/sh
set -eu

# Black-box smoke test for the real mynasd process. This deliberately exercises
# the HTTP boundary instead of calling handlers in-process, so it catches
# startup, migration, routing, cookie, and middleware regressions together.

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/lumonas-api-smoke.XXXXXX")
LISTEN_ADDR=${MYNAS_API_SMOKE_LISTEN:-127.0.0.1:18080}
BASE_URL="http://${LISTEN_ADDR}"
DB_PATH="$TEMP_DIR/mynas.db"
LOG_PATH="$TEMP_DIR/mynasd.log"
COOKIE_JAR="$TEMP_DIR/cookies.txt"
ADMIN_PASSWORD='api-smoke-password-123'
SERVER_PID=''

cleanup() {
	if [ -n "$SERVER_PID" ]; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	rm -rf "$TEMP_DIR"
}
trap cleanup EXIT INT TERM

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "api smoke test requires '$1'" >&2
		exit 1
	fi
}

require_command curl

if [ -n "${MYNAS_API_SMOKE_BIN:-}" ]; then
	MYNASD_BIN=$MYNAS_API_SMOKE_BIN
else
	MYNASD_BIN="$TEMP_DIR/mynasd"
	(
		cd "$ROOT_DIR"
		GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" \
		GOPATH="${GOPATH:-/tmp/lumonas-gopath}" \
			go build -trimpath -o "$MYNASD_BIN" ./cmd/mynasd
	)
fi

if [ ! -x "$MYNASD_BIN" ]; then
	echo "mynasd binary is not executable: $MYNASD_BIN" >&2
	exit 1
fi

MYNAS_DB_PATH="$DB_PATH" \
MYNAS_AUTH_REQUIRED=true \
MYNAS_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
MYNAS_CATALOG_FILE="$ROOT_DIR/catalog/apps.json" \
MYNAS_STACK_ROOT="$TEMP_DIR/stacks" \
"$MYNASD_BIN" -listen "$LISTEN_ADDR" >"$LOG_PATH" 2>&1 &
SERVER_PID=$!

wait_for_status() {
	endpoint=$1
	expected=$2
	attempt=0
	while [ "$attempt" -lt 75 ]; do
		status=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL$endpoint" 2>/dev/null || true)
		if [ "$status" = "$expected" ]; then
			return 0
		fi
		if ! kill -0 "$SERVER_PID" 2>/dev/null; then
			echo "mynasd exited before $endpoint became ready" >&2
			sed -n '1,160p' "$LOG_PATH" >&2
			return 1
		fi
		attempt=$((attempt + 1))
		sleep 0.2
	done
	echo "timed out waiting for $endpoint (last status: $status)" >&2
	sed -n '1,160p' "$LOG_PATH" >&2
	return 1
}

assert_status_and_body() {
	method=$1
	endpoint=$2
	expected=$3
	body_pattern=$4
	request_body=${5:-}
	response_path="$TEMP_DIR/response.json"

	if [ -n "$request_body" ]; then
		status=$(curl -sS -o "$response_path" -w '%{http_code}' \
			-X "$method" -H 'Content-Type: application/json' \
			-d "$request_body" "$BASE_URL$endpoint")
	else
		status=$(curl -sS -o "$response_path" -w '%{http_code}' -X "$method" "$BASE_URL$endpoint")
	fi
	if [ "$status" != "$expected" ]; then
		echo "$method $endpoint: expected HTTP $expected, got $status" >&2
		sed -n '1,120p' "$response_path" >&2
		exit 1
	fi
	if ! grep -F "$body_pattern" "$response_path" >/dev/null 2>&1; then
		echo "$method $endpoint: response did not contain '$body_pattern'" >&2
		sed -n '1,120p' "$response_path" >&2
		exit 1
	fi
}

assert_authenticated_status_and_body() {
	method=$1
	endpoint=$2
	expected=$3
	body_pattern=$4
	request_body=${5:-}
	response_path="$TEMP_DIR/response-auth.json"
	if [ -n "$request_body" ]; then
		status=$(curl -sS -o "$response_path" -w '%{http_code}' \
			-c "$COOKIE_JAR" -b "$COOKIE_JAR" \
			-X "$method" -H 'Content-Type: application/json' \
			-d "$request_body" "$BASE_URL$endpoint")
	else
		status=$(curl -sS -o "$response_path" -w '%{http_code}' \
			-c "$COOKIE_JAR" -b "$COOKIE_JAR" -X "$method" "$BASE_URL$endpoint")
	fi
	if [ "$status" != "$expected" ]; then
		echo "$method $endpoint: expected HTTP $expected, got $status" >&2
		sed -n '1,120p' "$response_path" >&2
		exit 1
	fi
	if ! grep -F "$body_pattern" "$response_path" >/dev/null 2>&1; then
		echo "$method $endpoint: response did not contain '$body_pattern'" >&2
		sed -n '1,120p' "$response_path" >&2
		exit 1
	fi
}

wait_for_status /healthz 200
assert_status_and_body GET /healthz 200 '"status":"ok"'
assert_status_and_body GET /readyz 200 '"status":"ready"'
assert_status_and_body GET /api/v1/auth/status 200 '"required":true'

# Health stays public, while the normal API is protected when authentication
# is enabled. Verify both sides before establishing a session.
assert_status_and_body GET /api/v1/server 401 'authentication required'
assert_status_and_body POST /api/v1/auth/login 401 'invalid credentials' '{"username":"admin","password":"wrong-password"}'

status=$(curl -sS -o "$TEMP_DIR/login.json" -w '%{http_code}' \
	-c "$COOKIE_JAR" -b "$COOKIE_JAR" \
	-X POST -H 'Content-Type: application/json' \
	-d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" \
	"$BASE_URL/api/v1/auth/login")
if [ "$status" != 200 ] || ! grep -F '"username":"admin"' "$TEMP_DIR/login.json" >/dev/null 2>&1; then
	echo "valid login failed (HTTP $status)" >&2
	sed -n '1,120p' "$TEMP_DIR/login.json" >&2
	exit 1
fi

status=$(curl -sS -o "$TEMP_DIR/server.json" -w '%{http_code}' \
	-c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/v1/server")
if [ "$status" != 200 ] || ! grep -F '"nasUuid"' "$TEMP_DIR/server.json" >/dev/null 2>&1; then
	echo "authenticated server request failed (HTTP $status)" >&2
	sed -n '1,120p' "$TEMP_DIR/server.json" >&2
	exit 1
fi

status=$(curl -sS -o "$TEMP_DIR/disks.json" -w '%{http_code}' \
	-c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/v1/disks")
if [ "$status" != 200 ] || ! head -c 1 "$TEMP_DIR/disks.json" | grep '\[' >/dev/null 2>&1; then
	echo "authenticated disk discovery failed (HTTP $status)" >&2
	sed -n '1,120p' "$TEMP_DIR/disks.json" >&2
	exit 1
fi

# Unsupported jobs must remain fail-closed at the public HTTP boundary.
assert_authenticated_status_and_body POST /api/v1/jobs 501 'only read-only' '{"type":"snapraid.sync","resourceId":"smoke-test"}'

assert_status_and_body POST /api/v1/auth/logout 200 '"status":"logged_out"'
assert_status_and_body GET /api/v1/server 401 'authentication required'

echo "API smoke test passed ($BASE_URL)"
