#!/bin/sh
# Shared helper for the systemd verification gates.
#
# `systemd-analyze verify` reports whether each ExecStart target is executable
# on the host, and the binaries a package installs do not exist outside a
# package context. Callers therefore need to separate that one class of finding,
# which is not verifiable here, from every other diagnostic, which is.
#
# verify_units <systemd-analyze args...>
#   Runs systemd-analyze verify and classifies its output. Exits non-zero for
#   any finding other than an unresolvable /usr/lib/lumonas ExecStart, printing
#   those findings. Returns 0 when only that class was reported, noting how many
#   were deferred so the coverage gap stays visible.
#
# The authoritative check, against the installed package and its real binaries,
# runs in scripts/permission-smoke.sh.

verify_units() {
	verify_output="$(systemd-analyze "$@" 2>&1)" || true

	deferred=0
	unexpected=""
	old_ifs=$IFS
	IFS='
'
	# Iterate without a subshell so the counters survive.
	for line in $verify_output; do
		[ -n "$line" ] || continue
		case "$line" in
			*"Command /usr/lib/lumonas/"*"is not executable"*)
				deferred=$((deferred + 1))
				;;
			*)
				unexpected="$unexpected$line
"
				;;
		esac
	done
	IFS=$old_ifs

	if [ -n "$unexpected" ]; then
		printf '%s' "$unexpected" >&2
		echo "systemd unit verification failed" >&2
		return 1
	fi
	if [ "$deferred" -gt 0 ]; then
		echo "NOTE: $deferred ExecStart target(s) under /usr/lib/lumonas are not" >&2
		echo "      installed here; they are verified against the installed" >&2
		echo "      package by scripts/permission-smoke.sh" >&2
	fi
	return 0
}
