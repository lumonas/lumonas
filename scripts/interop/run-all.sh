#!/usr/bin/env bash
# Runs every LumoNAS interoperability check and prints one summary. Missing
# tooling or unreachable services skip cleanly; only real protocol failures
# make the run exit non-zero.
set -u

here="$(dirname "$0")"
status=0

for script in "$here"/interop-smb.sh "$here"/interop-nfs.sh "$here"/interop-sftp.sh "$here"/interop-ftp.sh "$here"/interop-rsync.sh "$here"/interop-timemachine.sh; do
	printf '\n== %s ==\n' "$(basename "$script")"
	bash "$script" || status=1
done

exit "$status"
