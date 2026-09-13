#!/usr/bin/env bash
# FTP and FTPS interoperability via curl against the LumoNAS FTP(S) service.
set -u
# shellcheck source=interop-common.sh
. "$(dirname "$0")/interop-common.sh"

NAME_FTP="ftp roundtrip"
NAME_FTPS="ftps roundtrip"
require_tool curl "$NAME_FTP"
require_port "$LUMONAS_INTEROP_HOST" "${LUMONAS_INTEROP_FTP_PORT:-21}" "$NAME_FTP"
require_credentials "$NAME_FTP"

run_ftp() {
	local tls="$1"
	local name="$2"
	local url="ftp://$LUMONAS_INTEROP_HOST:${LUMONAS_INTEROP_FTP_PORT:-21}/$LUMONAS_INTEROP_SHARE/"
	local extra=()
	if [ "$tls" = "ftps" ]; then
		extra+=(--ssl-reqd --insecure)
		name="ftps roundtrip"
	fi
	local payload="lumonas-interop-$(date +%s).txt"
	printf 'lumonas ftp interop payload\n' > "$LUMONAS_INTEROP_WORKDIR/$payload"

	if ! curl -sS --connect-timeout 10 -u "$LUMONAS_INTEROP_USER:$LUMONAS_INTEROP_PASSWORD" \
		"${extra[@]}" -T "$LUMONAS_INTEROP_WORKDIR/$payload" "$url" >/dev/null 2>&1; then
		interop_result fail "$name" "upload failed"
		return
	fi
	if curl -sS --connect-timeout 10 -u "$LUMONAS_INTEROP_USER:$LUMONAS_INTEROP_PASSWORD" \
		"${extra[@]}" "$url$payload" 2>/dev/null | grep -q 'lumonas ftp interop payload'; then
		interop_result pass "$name"
	else
		interop_result fail "$name" "download roundtrip mismatch"
	fi
	curl -sS --connect-timeout 10 -u "$LUMONAS_INTEROP_USER:$LUMONAS_INTEROP_PASSWORD" \
		"${extra[@]}" -Q "-DELE $payload" "$url" >/dev/null 2>&1
}

require_port "$LUMONAS_INTEROP_HOST" "${LUMONAS_INTEROP_FTP_PORT:-21}" "$NAME_FTPS"
run_ftp ftp "$NAME_FTP"
run_ftp ftps "$NAME_FTPS"

interop_summary
[ "$INTEROP_FAIL" -eq 0 ]
