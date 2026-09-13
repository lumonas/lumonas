#!/bin/sh
set -eu

# Black-box smoke test for the real lumonasd process. This deliberately exercises
# the HTTP boundary instead of calling handlers in-process, so it catches
# startup, migration, routing, cookie, and middleware regressions together.

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/lumonas-api-smoke.XXXXXX")
LISTEN_ADDR=${LUMONAS_API_SMOKE_LISTEN:-127.0.0.1:18080}
BASE_URL="http://${LISTEN_ADDR}"
DB_PATH="$TEMP_DIR/lumonas.db"
LOG_PATH="$TEMP_DIR/lumonasd.log"
COOKIE_JAR="$TEMP_DIR/cookies.txt"
ADMIN_PASSWORD='api-smoke-password-123'
SERVER_PID=''
RESTART_SSE_PID=''

cleanup() {
	if [ -n "$RESTART_SSE_PID" ]; then
		kill "$RESTART_SSE_PID" 2>/dev/null || true
		wait "$RESTART_SSE_PID" 2>/dev/null || true
	fi
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

if [ -n "${LUMONAS_API_SMOKE_BIN:-}" ]; then
	LUMONASD_BIN=$LUMONAS_API_SMOKE_BIN
else
	LUMONASD_BIN="$TEMP_DIR/lumonasd"
	(
		cd "$ROOT_DIR"
		GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" \
		GOPATH="${GOPATH:-/tmp/lumonas-gopath}" \
			go build -trimpath -o "$LUMONASD_BIN" ./cmd/lumonasd
	)
fi

if [ ! -x "$LUMONASD_BIN" ]; then
	echo "lumonasd binary is not executable: $LUMONASD_BIN" >&2
	exit 1
fi

validate_response() {
	python3 "$ROOT_DIR/scripts/validate-api-response.py" "$1" "$2"
}

start_server() {
	LUMONAS_DB_PATH="$DB_PATH" \
	LUMONAS_AUTH_REQUIRED=true \
	LUMONAS_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
	LUMONAS_CATALOG_FILE="$ROOT_DIR/catalog/apps.json" \
	LUMONAS_STACK_ROOT="$TEMP_DIR/stacks" \
	LUMONAS_RECOVERY_DIR="$TEMP_DIR/recovery" \
	"$LUMONASD_BIN" -listen "$LISTEN_ADDR" >"$LOG_PATH" 2>&1 &
	SERVER_PID=$!
}

start_server

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
			echo "lumonasd exited before $endpoint became ready" >&2
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
			${CSRF_TOKEN:+-H "X-CSRF-Token: $CSRF_TOKEN"} \
			-d "$request_body" "$BASE_URL$endpoint")
	else
		status=$(curl -sS -o "$response_path" -w '%{http_code}' \
			-c "$COOKIE_JAR" -b "$COOKIE_JAR" \
			${CSRF_TOKEN:+-H "X-CSRF-Token: $CSRF_TOKEN"} \
			-X "$method" "$BASE_URL$endpoint")
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
assert_status_and_body GET /readyz 200 '"privilegedBroker":true'
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

CSRF_TOKEN=$(sed -n 's/.*"csrfToken":"\([^"]*\)".*/\1/p' "$TEMP_DIR/login.json")
if [ -z "$CSRF_TOKEN" ]; then
	echo "login response did not contain csrfToken" >&2
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
validate_response server "$TEMP_DIR/server.json"

status=$(curl -sS -o "$TEMP_DIR/disks.json" -w '%{http_code}' \
	-c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/v1/disks")
if [ "$status" != 200 ] || ! head -c 1 "$TEMP_DIR/disks.json" | grep '\[' >/dev/null 2>&1; then
	echo "authenticated disk discovery failed (HTTP $status)" >&2
	sed -n '1,120p' "$TEMP_DIR/disks.json" >&2
	exit 1
fi
validate_response disks "$TEMP_DIR/disks.json"

status=$(curl -sS -o "$TEMP_DIR/metrics.json" -w '%{http_code}' \
	-c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/v1/system/metrics")
[ "$status" = 200 ] || { echo "authenticated metrics request failed (HTTP $status)" >&2; exit 1; }
validate_response metrics "$TEMP_DIR/metrics.json"

status=$(curl -sS -o "$TEMP_DIR/jobs.json" -w '%{http_code}' \
	-c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/v1/jobs")
[ "$status" = 200 ] || { echo "authenticated jobs request failed (HTTP $status)" >&2; exit 1; }
validate_response jobs "$TEMP_DIR/jobs.json"

assert_authenticated_status_and_body GET /api/v1/settings 200 '"runtime"'
assert_authenticated_status_and_body GET /api/v1/ups/config 200 '"names":[]'
assert_authenticated_status_and_body PATCH /api/v1/ups/config 200 '"names":[]' '{"names":[]}'
assert_authenticated_status_and_body POST /api/v1/recovery/key 200 '"key"'
assert_authenticated_status_and_body POST /api/v1/recovery/export 201 '"verified":true'
assert_authenticated_status_and_body GET /api/v1/recovery/status 200 '"verified":true'
assert_authenticated_status_and_body GET '/api/v1/power/shutdown/plan?action=poweroff' 200 '"name":"stop-jobs"'
assert_authenticated_status_and_body POST /api/v1/updates/check 202 '"type":"updates.check"'

# The event stream must be live, not merely routable. The metrics loop emits
# within the timeout window after the initial SSE retry frame.
SSE_PATH="$TEMP_DIR/events.sse"
curl -sS --max-time 5 -N -b "$COOKIE_JAR" "$BASE_URL/api/v1/events/stream" >"$SSE_PATH" 2>/dev/null || true
if ! grep -F 'retry: 3000' "$SSE_PATH" >/dev/null 2>&1 || ! grep -F 'system.metrics' "$SSE_PATH" >/dev/null 2>&1; then
	echo "authenticated SSE stream did not deliver system metrics" >&2
	sed -n '1,80p' "$SSE_PATH" >&2
	exit 1
fi
python3 "$ROOT_DIR/scripts/validate-sse.py" "$SSE_PATH" system.metrics

# Restart the real daemon against the same SQLite state. The metrics event is
# the replay cursor; the short SMART job is deliberately interrupted while it
# is queued/running, and its persisted event must be replayable after restart.
RESTART_SSE_PATH="$TEMP_DIR/restart-events.sse"
curl -sS --max-time 10 -N -b "$COOKIE_JAR" "$BASE_URL/api/v1/events/stream" >"$RESTART_SSE_PATH" 2>/dev/null &
RESTART_SSE_PID=$!
for attempt in $(seq 1 30); do
	if grep -F '"type":"system.metrics"' "$RESTART_SSE_PATH" >/dev/null 2>&1; then
		break
	fi
	if ! kill -0 "$RESTART_SSE_PID" 2>/dev/null; then
		echo "restart SSE stream exited before a replay cursor was observed" >&2
		exit 1
	fi
	sleep 0.2
done
LAST_EVENT_ID=$(awk '/data: .*"type":"system.metrics"/ { print event_id; exit } /^id: / { event_id=$2 }' "$RESTART_SSE_PATH")
[ -n "$LAST_EVENT_ID" ] || { echo "could not extract SSE replay cursor" >&2; exit 1; }
DISK_ID=$(sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$TEMP_DIR/disks.json" | head -n 1)

if [ -z "$DISK_ID" ]; then
	kill "$RESTART_SSE_PID" 2>/dev/null || true
	wait "$RESTART_SSE_PID" 2>/dev/null || true
	RESTART_SSE_PID=''
	if [ "${LUMONAS_API_SMOKE_ASSERT:-false}" = "true" ]; then
		echo "could not select a disk for restart job in assertion mode" >&2
		exit 1
	fi
	echo "API restart job probe skipped: no Linux disk inventory is available" >&2
else
	JOB_PATH="$TEMP_DIR/restart-job.json"
	JOB_STATUS=$(curl -sS -o "$JOB_PATH" -w '%{http_code}' \
	-c "$COOKIE_JAR" -b "$COOKIE_JAR" -H "X-CSRF-Token: $CSRF_TOKEN" \
	-X POST -H 'Content-Type: application/json' \
	-d "{\"type\":\"smart.short\",\"resourceId\":\"$DISK_ID\"}" \
	"$BASE_URL/api/v1/jobs")
	[ "$JOB_STATUS" = 202 ] || { echo "restart job was not accepted (HTTP $JOB_STATUS)" >&2; sed -n '1,80p' "$JOB_PATH" >&2; exit 1; }
	JOB_ID=$(sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$JOB_PATH" | head -n 1)
	[ -n "$JOB_ID" ] || { echo "restart job response did not contain an id" >&2; exit 1; }
	kill "$RESTART_SSE_PID" 2>/dev/null || true
	wait "$RESTART_SSE_PID" 2>/dev/null || true
	RESTART_SSE_PID=''
	kill "$SERVER_PID" 2>/dev/null || true
	wait "$SERVER_PID" 2>/dev/null || true
	SERVER_PID=''
	start_server
	wait_for_status /healthz 200
	RESTARTED_JOB_PATH="$TEMP_DIR/restarted-job.json"
	RESTARTED_JOB_STATUS=$(curl -sS -o "$RESTARTED_JOB_PATH" -w '%{http_code}' \
	-c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/api/v1/jobs/$JOB_ID")
	[ "$RESTARTED_JOB_STATUS" = 200 ] || { echo "restarted job lookup failed (HTTP $RESTARTED_JOB_STATUS)" >&2; exit 1; }
	grep -F '"state":"failed"' "$RESTARTED_JOB_PATH" >/dev/null
	grep -F 'daemon restarted before the job completed' "$RESTARTED_JOB_PATH" >/dev/null
	REPLAY_PATH="$TEMP_DIR/replayed-events.sse"
	curl -sS --max-time 5 -N -b "$COOKIE_JAR" -H "Last-Event-ID: $LAST_EVENT_ID" "$BASE_URL/api/v1/events/stream" >"$REPLAY_PATH" 2>/dev/null || true
	grep -F 'retry: 3000' "$REPLAY_PATH" >/dev/null
	grep -F '"type":"job.state_changed"' "$REPLAY_PATH" >/dev/null
	if grep -F "id: $LAST_EVENT_ID" "$REPLAY_PATH" >/dev/null 2>&1; then
		echo "SSE replay returned the Last-Event-ID cursor event twice" >&2
		exit 1
	fi
	python3 "$ROOT_DIR/scripts/validate-sse.py" "$REPLAY_PATH" job.state_changed
	grep -F '"correlationId"' "$REPLAY_PATH" >/dev/null
fi

# SnapRAID jobs are accepted and delegated to the privileged broker.
assert_authenticated_status_and_body POST /api/v1/jobs 202 '"state":"queued"' '{"type":"snapraid.sync","resourceId":"smoke-test"}'

assert_status_and_body POST /api/v1/auth/logout 200 '"status":"logged_out"'
assert_status_and_body GET /api/v1/server 401 'authentication required'

echo "API smoke test passed ($BASE_URL)"
