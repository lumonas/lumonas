#!/usr/bin/env bash
# NFS interoperability: export discovery plus a root-permission mount roundtrip.
set -u
# shellcheck source=interop-common.sh
. "$(dirname "$0")/interop-common.sh"

NAME="nfs mount roundtrip"
require_tool showmount "$NAME"
[ "$(uname -s)" = "Linux" ] || { interop_result skip "$NAME" "NFS mount requires Linux"; exit 0; }
[ "$(id -u)" = "0" ] || { interop_result skip "$NAME" "requires root"; exit 0; }
require_port "$LUMONAS_INTEROP_HOST" 2049 "$NAME"

if ! showmount -e "$LUMONAS_INTEROP_HOST" >/dev/null 2>&1; then
	interop_result fail "$NAME" "export listing failed"
	interop_summary
	exit 1
fi

mountpoint="$LUMONAS_INTEROP_WORKDIR/nfs-mount"
mkdir -p "$mountpoint"
payload="lumonas-interop-nfs-$(date +%s).txt"

if ! mount -t nfs4 -o vers=4.2 "$LUMONAS_INTEROP_HOST:/$LUMONAS_INTEROP_SHARE" "$mountpoint" >/dev/null 2>&1; then
	interop_result fail "$NAME" "nfs4 mount failed"
	interop_summary
	exit 1
fi

printf 'lumonas nfs interop payload\n' > "$mountpoint/$payload"
sync
if grep -q 'lumonas nfs interop payload' "$mountpoint/$payload" 2>/dev/null; then
	interop_result pass "$NAME"
else
	interop_result fail "$NAME" "write/read roundtrip mismatch"
fi
rm -f "$mountpoint/$payload"
umount "$mountpoint" 2>/dev/null

interop_summary
[ "$INTEROP_FAIL" -eq 0 ]
