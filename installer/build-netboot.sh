#!/bin/sh
# Build a PXE/netboot bundle from a LumoNAS offline ISO: kernel, initrd, and
# squashfs plus ready-to-use GRUB and iPXE configuration snippets. The bundle
# is a plain tarball that can be dropped into any TFTP/HTTP root.
#
# Inputs (environment):
#   LUMONAS_ISO         path to the amd64 offline ISO (required)
#   LUMONAS_NETBOOT_DIR output directory (default build/netboot)
set -eu

VERSION="${1:-0.1.0-dev}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
ISO="${LUMONAS_ISO:-$ROOT/build/releases/lumonas-$VERSION-amd64.iso}"
OUT="${LUMONAS_NETBOOT_DIR:-$ROOT/build/netboot}"

[ -f "$ISO" ] || { echo "offline ISO not found: $ISO — run 'make iso' first" >&2; exit 1; }
command -v xorriso >/dev/null 2>&1 || { echo "xorriso is required to extract the ISO" >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo "tar is required" >&2; exit 1; }

EXTRACT="$OUT/extract"
rm -rf "$OUT"
mkdir -p "$EXTRACT" "$OUT/pxelinux.cfg" "$OUT/grub"
xorriso -osirrox on -indev "$ISO" -extract /live "$EXTRACT/live" >/dev/null 2>&1

KERNEL="$EXTRACT/live/vmlinuz"
INITRD="$EXTRACT/live/initrd.img"
SQUASHFS="$EXTRACT/live/filesystem.squashfs"
for artifact in "$KERNEL" "$INITRD" "$SQUASHFS"; do
	[ -f "$artifact" ] || { echo "ISO is missing $(basename "$artifact") — not a LumoNAS live image?" >&2; exit 1; }
done

cp "$KERNEL" "$INITRD" "$SQUASHFS" "$OUT/"
cp "$EXTRACT/live/filesystem.squashfs" "$OUT/lumonas.squashfs" 2>/dev/null || mv "$OUT/filesystem.squashfs" "$OUT/lumonas.squashfs"

cat > "$OUT/grub/grub.cfg" <<-'GRUB'
	set timeout=5
	menuentry "Install LumoNAS" {
		linux /vmlinuz boot=live squashfs_path=/lumonas.squashfs netboot=nfs
		initrd /initrd.img
	}
GRUB

cat > "$OUT/ipxe.script" <<-'IPXE'
	#!ipxe
	kernel http://${next-server}/lumonas/vmlinuz boot=live squashfs_path=/lumonas.squashfs netboot=http
	initrd http://${next-server}/lumonas/initrd.img
	boot
IPXE

cat > "$OUT/README.txt" <<-'DOCS'
	LumoNAS PXE bundle
	==================
	Serve this directory over TFTP (kernel/initrd) and HTTP (squashfs).

	GRUB:   copy grub.cfg next to the kernel and point your DHCP
	        option 17 / TFTP server at this directory.
	iPXE:   chainload ipxe.script from your DHCP/iPXE setup.

	The squashfs is loaded over HTTP; keep the file names stable.
DOCS

tar -czf "$OUT/lumonas-netboot-$VERSION.tar.gz" -C "$OUT" \
	vmlinuz initrd.img lumonas.squashfs grub/grub.cfg ipxe.script README.txt
rm -rf "$EXTRACT"
echo "Created PXE bundle: $OUT/lumonas-netboot-$VERSION.tar.gz"
