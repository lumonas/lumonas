#!/usr/bin/env bash
# SFTP interoperability: authenticated batch upload, listing, and download via
# the OpenSSH client against LumoNAS.
set -u
# shellcheck source=interop-common.sh
. "$(dirname "$0")/interop-common.sh"

NAME="sftp roundtrip"
require_tool sftp "$NAME"
require_port "$LUMONAS_INTEROP_HOST" 22 "$NAME"
require_credentials "$NAME"

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

if sftp -oBatchMode=no -oStrictHostKeyChecking=no -oUserKnownHostsFile=/dev/null \
	-P "${LUMONAS_INTEROP_SFTP_PORT:-22}" \
	"$LUMONAS_INTEROP_USER@$LUMONAS_INTEROP_HOST" -b "$batch" -- "$LUMONAS_INTEROP_PASSWORD" >/dev/null 2>&1 ||
	sftp -oBatchMode=no -oStrictHostKeyChecking=no -oUserKnownHostsFile=/dev/null \
		-P "${LUMONAS_INTEROP_SFTP_PORT:-22}" \
		"$LUMONAS_INTEROP_USER@$LUMONAS_INTEROP_HOST" -b "$batch" >/dev/null 2>&1; then
	if grep -q 'lumonas sftp interop payload' "$LUMONAS_INTEROP_WORKDIR/downloaded-$payload" 2>/dev/null; then
		interop_result pass "$NAME"
	else
		interop_result fail "$NAME" "downloaded content mismatch"
	fi
else
	interop_result fail "$NAME" "sftp batch failed (interactive password prompts are unsupported; configure an SSH key)"
fi

interop_summary
[ "$INTEROP_FAIL" -eq 0 ]
