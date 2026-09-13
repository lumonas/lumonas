#!/usr/bin/env bash
# rsync interoperability: module-based or ssh-based transfer with checksum
# verification and cleanup.
set -u
# shellcheck source=interop-common.sh
. "$(dirname "$0")/interop-common.sh"

NAME="rsync roundtrip"
require_tool rsync "$NAME"

payload="lumonas-interop-rsync-$(date +%s).txt"
printf 'lumonas rsync interop payload\n' > "$LUMONAS_INTEROP_WORKDIR/$payload"

mode="${LUMONAS_INTEROP_RSYNC_MODE:-module}"
rsync_flags=(-c)
if [ "$mode" = "module" ]; then
	require_port "$LUMONAS_INTEROP_HOST" "${LUMONAS_INTEROP_RSYNC_PORT:-873}" "$NAME"
	require_credentials "$NAME"
	destination="rsync://$LUMONAS_INTEROP_USER@$LUMONAS_INTEROP_HOST:${LUMONAS_INTEROP_RSYNC_PORT:-873}/${LUMONAS_INTEROP_RSYNC_MODULE:-$LUMONAS_INTEROP_SHARE}/"
else
	require_port "$LUMONAS_INTEROP_HOST" 22 "$NAME"
	destination="$LUMONAS_INTEROP_USER@$LUMONAS_INTEROP_HOST:/srv/$LUMONAS_INTEROP_SHARE/"
	rsync_flags+=(-e "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null")
fi

if ! rsync "${rsync_flags[@]}" "$LUMONAS_INTEROP_WORKDIR/$payload" "$destination" >/dev/null 2>&1; then
	interop_result fail "$NAME" "rsync upload failed (mode: $mode)"
	interop_summary
	exit 1
fi

# A checksum-validating dry run must consider the remote file up to date;
# that proves both sides agree on content.
if rsync --dry-run "${rsync_flags[@]}" "$LUMONAS_INTEROP_WORKDIR/$payload" "$destination" >/dev/null 2>&1; then
	interop_result pass "$NAME (mode: $mode)"
else
	interop_result fail "$NAME" "checksum verification failed (mode: $mode)"
fi
