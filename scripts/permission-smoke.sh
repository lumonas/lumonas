#!/bin/sh
set -eu

PACKAGE=${1:-}
if [ -z "$PACKAGE" ]; then
	echo "usage: $0 PACKAGE.deb" >&2
	exit 2
fi
[ -f "$PACKAGE" ] || { echo "package not found: $PACKAGE" >&2; exit 1; }

if command -v docker >/dev/null 2>&1; then
	RUNTIME=docker
elif command -v podman >/dev/null 2>&1; then
	RUNTIME=podman
else
	echo "docker or podman is required for package permission smoke testing" >&2
	exit 1
fi

# Mount the directory that actually contains the package rather than the
# repository root. CI downloads the artifact into build/package, so mounting
# the root would leave the .deb outside the container's view.
PACKAGE_DIR="$(CDPATH= cd -- "$(dirname -- "$PACKAGE")" && pwd)"
PACKAGE_NAME=$(basename "$PACKAGE")

# Satisfy the package's own Depends from its control metadata rather than
# hardcoding a list here, so a newly declared dependency cannot leave this
# gate installing an incomplete set.
command -v dpkg-deb >/dev/null 2>&1 || {
	echo "dpkg-deb is required to read the package dependencies" >&2
	exit 1
}
PACKAGE_DEPENDS="$(dpkg-deb -f "$PACKAGE" Depends 2>/dev/null \
	| tr ',' ' ' \
	| tr -s '[:space:]' ' ' \
	| sed 's/^ *//; s/ *$//' || true)"

"$RUNTIME" run --rm \
	-v "$PACKAGE_DIR:/packages:ro" \
	-e PACKAGE_NAME="$PACKAGE_NAME" \
	-e PACKAGE_DEPENDS="$PACKAGE_DEPENDS" \
	debian:trixie-slim \
	/bin/sh -euxc '
apt-get update
# passwd/util-linux provide the runuser and coreutils used by the assertions.
apt-get install -y --no-install-recommends ca-certificates $PACKAGE_DEPENDS passwd systemd util-linux
dpkg -i "/packages/$PACKAGE_NAME"

[ "$(stat -c "%U:%G:%a" /etc/lumonas)" = "root:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /etc/lumonas/lumonasd.env)" = "root:lumonas:640" ]
[ "$(stat -c "%U:%G:%a" /etc/lumonas/lumonas-web.env)" = "root:lumonas:640" ]
[ "$(stat -c "%U:%G:%a" /etc/lumonas/runtime.env)" = "root:lumonas:640" ]
[ "$(stat -c "%U:%G:%a" /var/lib/lumonas)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /var/lib/lumonas/secrets)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /var/lib/lumonas/recovery)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /srv/lumonas)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /srv/disks)" = "lumonas:lumonas:750" ]
[ "$(stat -c "%U:%G:%a" /srv/pools)" = "lumonas:lumonas:750" ]

runuser -u lumonas -- sh -eu -c "
	test -r /etc/lumonas/lumonasd.env
	test -r /etc/lumonas/lumonas-web.env
	test -r /etc/lumonas/runtime.env
	test ! -w /etc/lumonas/lumonasd.env
	test ! -w /etc/lumonas/lumonas-web.env
	test ! -w /etc/lumonas/runtime.env
	test ! -w /etc/lumonas
	touch /var/lib/lumonas/.permission-smoke
	touch /var/lib/lumonas/recovery/.permission-smoke
	touch /srv/lumonas/.permission-smoke
	touch /srv/disks/.permission-smoke
	touch /srv/pools/.permission-smoke
	rm -f /var/lib/lumonas/.permission-smoke /var/lib/lumonas/recovery/.permission-smoke /srv/lumonas/.permission-smoke /srv/disks/.permission-smoke /srv/pools/.permission-smoke
"

test "$(id -u lumonas)" -ne 0
test "$(id -g lumonas)" -ne 0
if getent group docker >/dev/null 2>&1; then
	! id -Gn lumonas | tr " " "\n" | grep -qx docker
fi
systemctl list-unit-files lumonas-web.service lumonasd.service >/dev/null

# Verify the units that were actually installed, against the binaries the
# package actually installed. This is the authoritative systemd gate: unlike a
# verification staged outside a package context, every ExecStart target here
# really exists, so a wrong path or a missing binary is caught. The optional
# distro services are Recommends and are not installed in this container, so
# provide stubs for them to satisfy the dependency graph.
for optional in avahi-daemon docker nfs-server rsync smbd ssh vsftpd; do
	cat > "/etc/systemd/system/$optional.service" <<STUB
[Unit]
Description=Stub $optional for unit verification
[Service]
Type=oneshot
ExecStart=/bin/true
RemainAfterExit=yes
[Install]
WantedBy=multi-user.target
STUB
done
systemd-analyze verify /lib/systemd/system/lumonas*.service /lib/systemd/system/lumonas*.target

dpkg --audit
'

echo "LumoNAS package permission smoke test passed: $PACKAGE_NAME"
