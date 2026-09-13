# Offline USB Installer and OS Image

## Requirement

A LumoNAS USB image must install a fully usable NAS without Internet access.

Core installation cannot depend on:

- Debian mirrors;
- Docker repositories;
- GitHub;
- LumoNAS servers;
- external DNS;
- cloud services.

## Release artifact

Primary artifact:

```text
lumonas-<version>-amd64.iso
```

Also publish:

```text
SHA256SUMS
signature
release notes
```

`SHA256SUMS` covers the package, ISO, machine image, SBOM/signature
sidecars, and `RELEASE-MANIFEST.json` itself.

Future artifacts may include qcow2/raw images.

## Baseline

For the September 2026 plan, use Debian 13 Stable.

Pin package versions per LumoNAS release so:

> same ISO = same installed core system.

The builder uses a dated Debian snapshot over HTTPS and validates its workdir
and optional live-build cache source before cleanup. Cache data may not live
inside the disposable ISO workdir.

The ISO and recovery QEMU smoke tests use restricted user networking: host port
forwarding remains available for assertions, but the guest cannot reach an
external network while proving the offline installer path. This applies to the
source appliance, recovery environment, and final boot from the restored disk.

Online security updates may be applied after installation.

The live ISO web console uses the same provisioned local TLS certificate as the
installed appliance. Its health and QEMU smoke probes use HTTPS with explicit
certificate pinning disabled only for the generated self-signed certificate;
the recovery helper's internal loopback API remains separate and local.

Release builds set `SOURCE_DATE_EPOCH` from the source commit. The package and
ISO manifests record this value, and the embedded APT `Release` metadata uses
it for its date, so release metadata is reproducible from the checked-out
source rather than the build machine clock.

The installed ISO and QEMU appliance also carry versioned Debian package
inventories at `/usr/share/doc/lumonas/iso-package-manifest.txt` and
`/usr/share/doc/lumonas/qemu-package-manifest.txt`. Each inventory records the
source commit, reproducible epoch, and exact package versions present in the
image.

## Image build approach

Recommended evolution:

### Initial engineering

Use Debian installer/live tooling plus an embedded offline APT repository.

### Mature release

Use reproducible build configuration, likely `live-build` for installer media and evaluate `mkosi` for machine images/A-B system experiments.

The installer should rely on Debian for:

- kernel;
- drivers;
- partitioning primitives;
- bootloader;
- EFI/BIOS support;
- filesystem package installation.

Do not write a partitioner from scratch.

## Embedded packages

The image should include:

- Debian base;
- common firmware packages;
- NetworkManager;
- Avahi;
- nftables;
- OpenSSH;
- Samba;
- NFS server/client;
- rsync;
- FTP server if enabled;
- mergerfs;
- SnapRAID;
- smartmontools;
- lm-sensors;
- NUT;
- Docker Engine;
- Docker Compose plugin;
- LumoNAS packages;
- recovery tools.

## Offline repository

Embed a signed local repository:

```text
/opt/lumonas-repo/
├── dists/
├── pool/
├── Release
├── Release.gpg
└── lumonas-keyring...
```

Installation points APT to the media/local repo first.

Network availability may add official mirrors, but cannot be mandatory.

Release builds must set `LUMONAS_REQUIRE_REPO_SIGNATURE=true` and provide the
release signing key and private key to an ephemeral GnuPG home. The installer
then fails closed unless `Release.gpg`, `InRelease`, and the embedded archive
keyring are present; `[trusted=yes]` is reserved for local development images.
The live-build installation hook also fails closed if signed repository
metadata refresh or package installation fails; direct package fallback is
limited to unsigned local development media.

## Boot menu

```text
Install LumoNAS
Recover existing LumoNAS
Hardware diagnostics
Advanced options
```

Advanced:

- safe graphics;
- serial console;
- shell;
- memory test if available;
- raw recovery environment.

## Installer screens

1. Welcome/hardware detection
2. Language/keyboard/timezone
3. New install vs restore
4. Select system disk
5. Network (DHCP/static, Internet optional)
6. Admin + SSH key
7. Basic system choices
8. Destructive review
9. Installation progress
10. Final verification/reboot

Storage pool/parity configuration happens after boot in the browser.

## System disk selection

Show:

- model;
- capacity;
- serial suffix/full on detail;
- current filesystem;
- used data estimate;
- whether existing LumoNAS metadata exists.

Do not emphasize `/dev/sdX`.

Selecting a disk containing data requires explicit advanced override.

## Offline networking behavior

If DHCP succeeds but Internet does not:

```text
LAN: Connected
Internet: Unavailable
Installation can continue offline.
```

mDNS should make the installed system discoverable as `lumonas.local` after boot.

## Browser-based installer future

Long-term, the boot environment may display:

```text
Open http://192.168.1.123 to install LumoNAS
```

and use the same React design language remotely.

For the first production installer, reliability is more important than building a custom network installer.

## Recovery mode

Recovery media should support:

- detect existing LumoNAS data disks;
- validate recovery backup;
- install blank replacement system SSD;
- restore desired state;
- reset admin access;
- network rescue;
- bootloader repair;
- export diagnostics;
- read-only data inspection.

## Data-disk protection

During installer/recovery discovery:

- mount existing data disks read-only;
- identify by LumoNAS metadata + filesystem UUID + disk serial;
- do not run repair/format automatically;
- do not alter parity.

## Verification before success

Installer must verify:

- bootloader installed;
- root filesystem mountable;
- LumoNAS services enabled;
- local database created;
- Docker starts;
- network configuration valid;
- UI responds locally;
- protected data disks were not modified.

## CI requirements

For each release image:

- UEFI QEMU install;
- BIOS install if supported;
- no-network install;
- first boot;
- UI availability;
- Docker availability;
- SMB service;
- install with unrelated data disks attached;
- install with existing LumoNAS disks attached;
- recovery install after deleting system disk.

Offline-install failure blocks release.
