#!/usr/bin/env bash
# SFTP interoperability: authenticated batch upload, listing, and download via
# the OpenSSH client against LumoNAS.
set -u
# shellcheck source=interop-common.sh
. "$(dirname "$0")/interop-common.sh"

NAME="sftp roundtrip"
require_tool sftp "$NAME"
require_port "$LUMONAS_INTEROP_HOST" 22 "$NAME"
require_user "$NAME"

if [ -n "${LUMONAS_INTEROP_SSH_KEY:-}" ]; then
	[ -f "$LUMONAS_INTEROP_SSH_KEY" ] || {
		interop_result skip "$NAME" "LUMONAS_INTEROP_SSH_KEY does not point to a file"
		exit 0
	}
	ssh_options=(-i "$LUMONAS_INTEROP_SSH_KEY")
else
	# OpenSSH sftp has no safe password argument. BatchMode keeps a hardware
	# acceptance run from hanging on an interactive prompt; use an ssh-agent or
	# set LUMONAS_INTEROP_SSH_KEY for non-interactive authentication.
	ssh_options=(-oBatchMode=yes)
fi

payload="lumonas-interop-$(date +%s).txt"
printf 'lumonas sftp interop payload\n' > "$LUMONAS_INTEROP_WORKDIR/$payload"

batch="$LUMONAS_INTEROP_WORKDIR/batch.sftp"
{
	printf 'cd /%s\n' "$LUMONAS_INTEROP_SHARE"
	printf 'put %s\n' "$payload"
	printf 'ls -l %s\n' "$payload"
	printf 'get %s downloaded-%s\n' "$payload" "$payload"
	printf 'rm %s\n' "$payload"
} > "$batch"

if sftp "${ssh_options[@]}" -oStrictHostKeyChecking=no -oUserKnownHostsFile=/dev/null \
	-P "${LUMONAS_INTEROP_SFTP_PORT:-22}" \
	"$LUMONAS_INTEROP_USER@$LUMONAS_INTEROP_HOST" -b "$batch" >/dev/null 2>&1; then
	if grep -q 'lumonas sftp interop payload' "$LUMONAS_INTEROP_WORKDIR/downloaded-$payload" 2>/dev/null; then
		interop_result pass "$NAME"
	else
		interop_result fail "$NAME" "downloaded content mismatch"
	fi
else
	interop_result fail "$NAME" "sftp batch failed; configure an SSH key or ssh-agent"
fi

interop_summary
[ "$INTEROP_FAIL" -eq 0 ]
