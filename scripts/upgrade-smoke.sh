#!/bin/sh
set -eu

OLD_PACKAGE="${1:-}"
NEW_PACKAGE="${2:-}"
if [ -z "$OLD_PACKAGE" ] || [ -z "$NEW_PACKAGE" ]; then
	echo "usage: $0 OLD_PACKAGE.deb NEW_PACKAGE.deb" >&2
	exit 2
fi
[ -f "$OLD_PACKAGE" ] || { echo "old package not found: $OLD_PACKAGE" >&2; exit 1; }
[ -f "$NEW_PACKAGE" ] || { echo "new package not found: $NEW_PACKAGE" >&2; exit 1; }

if command -v docker >/dev/null 2>&1; then
	RUNTIME=docker
elif command -v podman >/dev/null 2>&1; then
	RUNTIME=podman
else
	echo "docker or podman is required for Debian upgrade smoke testing" >&2
	exit 1
fi

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OLD_NAME=$(basename "$OLD_PACKAGE")
NEW_NAME=$(basename "$NEW_PACKAGE")
EXPECTED_VERSION=$(dpkg-deb -f "$NEW_PACKAGE" Version)

"$RUNTIME" run --rm \
	-v "$ROOT:/packages:ro" \
	-e OLD_PACKAGE="$OLD_NAME" \
	-e NEW_PACKAGE="$NEW_NAME" \
	-e EXPECTED_VERSION="$EXPECTED_VERSION" \
	debian:trixie-slim \
	/bin/sh -eux <<'CONTAINER'
apt-get update
apt-get install -y --no-install-recommends systemd passwd ca-certificates sqlite3
dpkg -i "/packages/$OLD_PACKAGE"
printf "%s\n" "LumoNAS administrator marker" >>/etc/lumonas/lumonasd.env

# Replace the freshly initialized database with a representative database from
# an older appliance. The new package's postinst must migrate this database
# before any service starts; checking only that dpkg accepts the package would
# miss upgrade-time schema regressions.
rm -f /var/lib/lumonas/lumonas.db /var/lib/lumonas/lumonas.db-shm /var/lib/lumonas/lumonas.db-wal
sqlite3 /var/lib/lumonas/lumonas.db <<'SQL'
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE events (id TEXT PRIMARY KEY, type TEXT NOT NULL, timestamp TEXT NOT NULL, severity TEXT NOT NULL, resource_type TEXT, resource_id TEXT, data_json TEXT NOT NULL);
CREATE TABLE jobs (id TEXT PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL, resource_id TEXT, state TEXT NOT NULL, progress REAL, stage TEXT, created_at TEXT NOT NULL, started_at TEXT, finished_at TEXT, error TEXT);
CREATE TABLE audit_log (id TEXT PRIMARY KEY, timestamp TEXT NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL, outcome TEXT NOT NULL, resource_type TEXT, resource_id TEXT, metadata_json TEXT NOT NULL);
CREATE TABLE network_bindings (service TEXT PRIMARY KEY, config_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE network_firewall (id INTEGER PRIMARY KEY CHECK(id=1), config_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE network_connections (id TEXT PRIMARY KEY, uuid TEXT NOT NULL DEFAULT '', name TEXT NOT NULL, interface TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, config_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
INSERT INTO users VALUES('legacy-user','admin','legacy-hash','2026-01-01T00:00:00Z');
INSERT INTO events VALUES('legacy-event','legacy.test','2026-01-01T00:00:00Z','info',NULL,NULL,'{}');
INSERT INTO jobs VALUES('legacy-job','legacy.test','Legacy job',NULL,'completed',NULL,NULL,'2026-01-01T00:00:00Z',NULL,NULL,NULL);
INSERT INTO audit_log VALUES('legacy-audit','2026-01-01T00:00:00Z','admin','legacy.upgrade','recorded',NULL,NULL,'{}');
INSERT INTO network_bindings VALUES('web','{}','2026-01-01T00:00:00Z');
INSERT INTO network_firewall VALUES(1,'{}','2026-01-01T00:00:00Z');
INSERT INTO network_connections VALUES('lan','legacy-uuid','LAN','eth0',1,'{"id":"lan","name":"LAN","interface":"eth0","enabled":true,"type":"ethernet","ipv4":{"method":"auto"},"ipv6":{"method":"disabled"}}','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z');
SQL
chown lumonas:lumonas /var/lib/lumonas/lumonas.db
chmod 0640 /var/lib/lumonas/lumonas.db
dpkg -i "/packages/$NEW_PACKAGE"
test "$(dpkg-query -W -f='${Version}' lumonas)" = "$EXPECTED_VERSION"
grep -Fx "LumoNAS administrator marker" /etc/lumonas/lumonasd.env
test -d /var/lib/lumonas/recovery
test -d /srv/lumonas
test -f /etc/docker/daemon.json
test "$(stat -c "%U:%G:%a" /etc/lumonas/runtime.env)" = "root:lumonas:640"
test -x /usr/lib/lumonas/lumonasd
test -x /usr/lib/lumonas/lumonas-recover
test -x /usr/lib/lumonas/lumonas-migrate
test -f /var/lib/lumonas/lumonas.db
test "$(sqlite3 /var/lib/lumonas/lumonas.db "SELECT count(*) FROM principals WHERE id='legacy-user' AND management_role='owner';")" = "1"
test "$(sqlite3 /var/lib/lumonas/lumonas.db "SELECT count(*) FROM events WHERE id='legacy-event';")" = "1"
test "$(sqlite3 /var/lib/lumonas/lumonas.db "SELECT count(*) FROM pragma_table_info('events') WHERE name IN ('correlation_id','operation_id','generation');")" = "3"
test "$(sqlite3 /var/lib/lumonas/lumonas.db "SELECT count(*) FROM pragma_table_info('jobs') WHERE name IN ('correlation_id','operation_id','plan_hash','actor','generation');")" = "5"
test "$(sqlite3 /var/lib/lumonas/lumonas.db "SELECT count(*) FROM service_bindings WHERE service='web';")" = "1"
test "$(sqlite3 /var/lib/lumonas/lumonas.db "SELECT count(*) FROM firewall_policies WHERE id=1;")" = "1"
test "$(sqlite3 /var/lib/lumonas/lumonas.db "SELECT status FROM network_connections WHERE id='lan';")" = "configured"
test "$(sqlite3 /var/lib/lumonas/lumonas.db "SELECT value FROM meta WHERE key='config_generation';")" = "1"
test -f /lib/systemd/system/lumonas-runtime.service
test -f /lib/systemd/system/lumonasd.service
test -f /lib/systemd/system/lumonas-web.service
dpkg --audit
CONTAINER

echo "Debian package upgrade smoke test passed: $OLD_NAME -> $NEW_NAME"
