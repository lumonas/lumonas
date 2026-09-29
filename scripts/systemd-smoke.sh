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

# systemd-analyze needs the boot targets and the distro units the LumoNAS units
# depend on, otherwise it cannot construct a start transaction at all. Stage
# the distro units first, because each one may also own a drop-in directory.
for target in sysinit.target basic.target shutdown.target rescue.target emergency.target \
	local-fs.target swap.target umount.target network-online.target remote-fs.target \
	network.target multi-user.target graphical.target timers.target; do
	printf '[Unit]\nDescription=Stub %s for unit verification\n' "$target" >"$STAGE/etc/systemd/system/$target"
done
# Ordering units the packaged units reference by name.
for unit in systemd-modules-load systemd-journald systemd-udevd systemd-tmpfiles-setup \
	sysinit early-boot; do
	{
		printf '[Unit]\nDescription=Stub %s for unit verification\n' "$unit"
		printf '[Service]\nType=oneshot\nExecStart=/bin/true\nRemainAfterExit=yes\n'
		printf '[Install]\nWantedBy=sysinit.target\n'
	} >"$STAGE/etc/systemd/system/$unit.service"
done
for unit in avahi-daemon docker nfs-server rsync smbd ssh vsftpd; do
	{
		printf '[Unit]\nDescription=Stub %s for unit verification\n' "$unit"
		printf '[Service]\nType=oneshot\nExecStart=/bin/true\nRemainAfterExit=yes\n'
		printf '[Install]\nWantedBy=multi-user.target\n'
	} >"$STAGE/etc/systemd/system/$unit.service"
done
for dropin in "$ROOT"/packaging/systemd/*.d; do
	[ -d "$dropin" ] || continue
	mkdir -p "$STAGE/etc/systemd/system/$(basename "$dropin")"
	cp "$dropin"/* "$STAGE/etc/systemd/system/$(basename "$dropin")/" 2>/dev/null || true
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
	/usr/bin/rsync \
	/usr/bin/rsyncd \
	/usr/bin/vsftpd \
	/usr/bin/nmbd \
	/usr/sbin/smbd \
	/usr/sbin/nmbd \
	/usr/sbin/rsyncd \
	/usr/sbin/vsftpd \
	/usr/bin/systemctl \
	/usr/bin/apt-get \
	/bin/true \
	/usr/lib/systemd/systemd \
	/usr/sbin/snapraid \
	/usr/bin/mergerfs \
	/usr/bin/dpkg; do
	target="$STAGE$executable"
	mkdir -p "$(dirname "$target")"
	printf '#!/bin/sh\nexit 0\n' >"$target"
	chmod 0755 "$target"
done

# Pass unit names rather than absolute paths inside the root: a path handed to
# verify is loaded from the host filesystem, so its absolute ExecStart targets
# would be resolved against the host and never found. Naming the units lets
# --root's unit search path load them from the staged tree, which is what makes
# the stub executables resolve.
# shellcheck disable=SC2034
LUMONAS_VERIFY_UNITS=""
for unit in "$STAGE"/etc/systemd/system/*.service "$STAGE"/etc/systemd/system/*.target; do
	[ -f "$unit" ] || continue
	name=$(basename "$unit")
	# Only the LumoNAS units are under test; the rest exist to satisfy the
	# dependency graph and are pulled in transitively.
	case "$name" in
		lumonas*|lumonasd.*) LUMONAS_VERIFY_UNITS="$LUMONAS_VERIFY_UNITS $name" ;;
	esac
done
# shellcheck disable=SC2086
[ -n "$LUMONAS_VERIFY_UNITS" ] || { echo "no LumoNAS units were staged" >&2; exit 1; }
# --root already sets the unit search path, so do not also point
# SYSTEMD_UNIT_PATH at the staged directory: systemd would then look for units
# under <root><path> and fail to resolve the staged .target files.
# shellcheck disable=SC2086
systemd-analyze --root="$STAGE" verify $LUMONAS_VERIFY_UNITS
echo "LumoNAS systemd units verified"
