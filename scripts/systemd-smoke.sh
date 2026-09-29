#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
REQUIRE="${LUMONAS_REQUIRE_SYSTEMD_VERIFY:-false}"

if ! command -v systemd-analyze >/dev/null 2>&1; then
	if [ "$REQUIRE" = "true" ]; then
		echo "systemd-analyze is required for the systemd unit gate" >&2
		exit 1
	fi
	echo "systemd-analyze is not installed; unit verification skipped" >&2
	exit 0
fi

# systemd-analyze verify also resolves every ExecStart target, so verifying the
# units in a bare checkout fails with "not executable: No such file or
# directory" for the binaries the package would install. Verify against a
# throwaway root that contains the units plus stub executables, so this gate
# checks the unit contracts rather than the presence of a build.
STAGE="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-systemd-verify.XXXXXX")"
cleanup() { rm -rf "$STAGE"; }
trap cleanup EXIT INT TERM
mkdir -p "$STAGE/etc/systemd/system" "$STAGE/usr/lib/lumonas" "$STAGE/usr/local/bin" "$STAGE/sbin" "$STAGE/bin"
for unit in "$ROOT"/packaging/systemd/*.service "$ROOT"/packaging/systemd/*.target; do
	[ -f "$unit" ] || continue
	cp "$unit" "$STAGE/etc/systemd/system/"
done
for dropin in "$ROOT"/packaging/systemd/*.d; do
	[ -d "$dropin" ] || continue
	name="$(basename "$dropin" .d)"
	mkdir -p "$STAGE/etc/systemd/system/$name"
	cp "$dropin"/* "$STAGE/etc/systemd/system/$name/" 2>/dev/null || true
done
# Executables the units and their drop-ins reference by absolute path.
for executable in \
	/usr/lib/lumonas/lumonasd \
	/usr/lib/lumonas/lumonas-web \
	/usr/lib/lumonas/lumonas-privd \
	/usr/lib/lumonas/lumonas-migrate \
	/usr/lib/lumonas/lumonas-recover \
	/usr/lib/lumonas/install-disk \
	/usr/local/bin/fwupdmgr \
	/usr/bin/docker \
	/usr/bin/smbd \
	/usr/bin/rsyncd \
	/usr/bin/vsftpd \
	/usr/bin/nmbd \
	/usr/sbin/smbd \
	/usr/bin/systemctl \
	/usr/bin/apt-get \
	/usr/sbin/snapraid \
	/usr/bin/mergerfs \
	/usr/bin/dpkg; do
	target="$STAGE$executable"
	mkdir -p "$(dirname "$target")"
	printf '#!/bin/sh\nexit 0\n' >"$target"
	chmod 0755 "$target"
done

systemd-analyze --root="$STAGE" verify \
	"$STAGE"/etc/systemd/system/*.service "$STAGE"/etc/systemd/system/*.target
echo "LumoNAS systemd units verified"
