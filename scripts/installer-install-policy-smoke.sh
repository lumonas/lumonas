#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SCRIPT="$ROOT/packaging/scripts/install-disk.sh"

[ -x "$SCRIPT" ] || { echo "offline disk installer must be executable" >&2; exit 1; }
sh -n "$SCRIPT"

for marker in \
	'ADMIN_PASSWORD="$(sed -n' \
	'rsync -aHAX --numeric-ids --one-file-system' \
	'--exclude=/proc/***' \
	'--exclude=/sys/***' \
	'findmnt -rn -S' \
	'lsblk -nrpo MOUNTPOINT' \
	'sfdisk --wipe always' \
	'partx --update' \
	'first-boot.env' \
	'grub-install --target=x86_64-efi' \
	'grub-install --target=i386-pc'; do
	grep -F -- "$marker" "$SCRIPT" >/dev/null || {
		echo "offline disk installer is missing required marker: $marker" >&2
		exit 1
	}
done

if grep -E 'debootstrap|apt-get|LUMONAS_INSTALL_MIRROR' "$SCRIPT" >/dev/null; then
	echo "offline disk installer must not require package downloads" >&2
	exit 1
fi

echo "LumoNAS offline disk installer policy passed"
