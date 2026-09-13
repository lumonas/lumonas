#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
CONTROL="$ROOT/packaging/debian/control"
QEMU="$ROOT/scripts/qemu-build-image.sh"
ISO="$ROOT/installer/build-iso.sh"

# These packages are part of the appliance integration baseline. The QEMU
# image deliberately uses --no-install-recommends, so it must list every
# integration explicitly instead of relying on Debian metadata.
for package in \
	network-manager systemd-resolved smartmontools lm-sensors nut-client mergerfs snapraid grub-efi-amd64 dosfstools efibootmgr \
	e2fsprogs xfsprogs docker.io docker-compose samba samba-common-bin \
	nfs-kernel-server rsync vsftpd avahi-daemon nftables; do
	if ! grep -Eq "(^|[ ,])${package}([, ]|$)" "$CONTROL"; then
		echo "package $package is missing from Debian control metadata" >&2
		exit 1
	fi
	if ! grep -Eq "(^|[[:space:]])${package}([[:space:]\\\\]|$)" "$QEMU"; then
		echo "package $package is missing from the QEMU appliance install set" >&2
		exit 1
	fi
	if ! grep -Eq "^${package}$" "$ISO"; then
		echo "package $package is missing from the offline ISO install set" >&2
		exit 1
	fi
done

echo "LumoNAS package dependency parity verified"
