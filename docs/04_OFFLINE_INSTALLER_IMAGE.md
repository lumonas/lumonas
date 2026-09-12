# Offline USB Installer and OS Image

## Requirement

A MyNAS USB image must install a fully usable NAS without Internet access.

Core installation cannot depend on:

- Debian mirrors;
- Docker repositories;
- GitHub;
- MyNAS servers;
- external DNS;
- cloud services.

## Release artifact

Primary artifact:

```text
mynas-<version>-amd64.iso
```

Also publish:

```text
SHA256SUMS
signature
release notes
```

Future artifacts may include qcow2/raw images.

## Baseline

For the September 2026 plan, use Debian 13 Stable.

Pin package versions per MyNAS release so:

> same ISO = same installed core system.

Online security updates may be applied after installation.

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
- MyNAS packages;
- recovery tools.

## Offline repository

Embed a signed local repository:

```text
/opt/mynas-repo/
├── dists/
├── pool/
├── Release
├── Release.gpg
└── mynas-keyring...
```

Installation points APT to the media/local repo first.

Network availability may add official mirrors, but cannot be mandatory.

## Boot menu

```text
Install MyNAS
Recover existing MyNAS
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
- whether existing MyNAS metadata exists.

Do not emphasize `/dev/sdX`.

Selecting a disk containing data requires explicit advanced override.

## Offline networking behavior

If DHCP succeeds but Internet does not:

```text
LAN: Connected
Internet: Unavailable
Installation can continue offline.
```

mDNS should make the installed system discoverable as `mynas.local` after boot.

## Browser-based installer future

Long-term, the boot environment may display:

```text
Open http://192.168.1.123 to install MyNAS
```

and use the same React design language remotely.

For the first production installer, reliability is more important than building a custom network installer.

## Recovery mode

Recovery media should support:

- detect existing MyNAS data disks;
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
- identify by MyNAS metadata + filesystem UUID + disk serial;
- do not run repair/format automatically;
- do not alter parity.

## Verification before success

Installer must verify:

- bootloader installed;
- root filesystem mountable;
- MyNAS services enabled;
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
- install with existing MyNAS disks attached;
- recovery install after deleting system disk.

Offline-install failure blocks release.
