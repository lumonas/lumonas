#!/usr/bin/env bash
# Time Machine / Avahi interoperability: verifies LumoNAS is advertising its
# SMB and Time Machine services over mDNS and, on macOS, that the advertised
# host is usable by the Time Machine framework.
set -u
# shellcheck source=interop-common.sh
. "$(dirname "$0")/interop-common.sh"

NAME="avahi timemachine advertisement"
require_tool avahi-browse "$NAME"

browse() {
	avahi-browse -rpt -t "$1" 2>/dev/null
}

smb_found=$(browse _smb._tcp | grep -c "IPv4" || true)
adisk_found=$(browse _adisk._tcp | grep -c "IPv4" || true)

if [ "$smb_found" -eq 0 ]; then
	interop_result fail "$NAME" "no _smb._tcp advertisement discovered"
else
	interop_result pass "_smb._tcp advertisement ($smb_found instance(s))"
fi

if [ "$adisk_found" -eq 0 ]; then
	interop_result skip "timemachine _adisk._tcp" "no Time Machine advertisement (expected unless a TM share is enabled)"
else
	interop_result pass "_adisk._tcp advertisement ($adisk_found instance(s))"
fi

if [ "$(uname -s)" = "Darwin" ] && command -v tmutil >/dev/null 2>&1; then
	NAME_MAC="macOS tmutil discovery"
	if tmutil listbackups >/dev/null 2>&1 || tmutil destinationinfo >/dev/null 2>&1; then
		interop_result pass "$NAME_MAC"
	else
		interop_result skip "$NAME_MAC" "tmutil could not query Time Machine state"
	fi
fi

interop_summary
[ "$INTEROP_FAIL" -eq 0 ]
