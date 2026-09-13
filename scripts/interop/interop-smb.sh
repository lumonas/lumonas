#!/usr/bin/env bash
# SMB interoperability: authenticated list, upload, download, and delete via
# smbclient against the configured LumoNAS share.
set -u
# shellcheck source=interop-common.sh
. "$(dirname "$0")/interop-common.sh"

NAME="smb roundtrip"
require_tool smbclient "$NAME"
require_port "$LUMONAS_INTEROP_HOST" 445 "$NAME"
require_credentials "$NAME"

payload="lumonas-interop-$(date +%s).txt"
printf 'lumonas smb interop payload\n' > "$LUMONAS_INTEROP_WORKDIR/$payload"

if ! smbclient "//$LUMONAS_INTEROP_HOST/$LUMONAS_INTEROP_SHARE" \
	-U "$LUMONAS_INTEROP_USER%$LUMONAS_INTEROP_PASSWORD" \
	-m SMB3 -c "ls" >/dev/null 2>&1; then
	interop_result fail "$NAME" "share listing failed"
	interop_summary
	exit 1
fi

if ! smbclient "//$LUMONAS_INTEROP_HOST/$LUMONAS_INTEROP_SHARE" \
	-U "$LUMONAS_INTEROP_USER%$LUMONAS_INTEROP_PASSWORD" \
	-m SMB3 -c "put \"$LUMONAS_INTEROP_WORKDIR/$payload\" \"$payload\"" >/dev/null 2>&1; then
	interop_result fail "$NAME" "upload failed"
	interop_summary
	exit 1
fi

rm -f "$LUMONAS_INTEROP_WORKDIR/$payload"
if smbclient "//$LUMONAS_INTEROP_HOST/$LUMONAS_INTEROP_SHARE" \
	-U "$LUMONAS_INTEROP_USER%$LUMONAS_INTEROP_PASSWORD" \
	-m SMB3 -c "get \"$payload\" \"$LUMONAS_INTEROP_WORKDIR/$payload\"" >/dev/null 2>&1 &&
	grep -q 'lumonas smb interop payload' "$LUMONAS_INTEROP_WORKDIR/$payload"; then
	interop_result pass "$NAME"
else
	interop_result fail "$NAME" "download roundtrip mismatch"
fi

smbclient "//$LUMONAS_INTEROP_HOST/$LUMONAS_INTEROP_SHARE" \
	-U "$LUMONAS_INTEROP_USER%$LUMONAS_INTEROP_PASSWORD" \
	-m SMB3 -c "rm \"$payload\"" >/dev/null 2>&1

interop_summary
[ "$INTEROP_FAIL" -eq 0 ]
