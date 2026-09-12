# Product Vision and Principles

## Goal

Build a NAS OS that combines the ease of a consumer appliance with the transparency and flexibility of Debian.

The primary workload is:

- 4–12 HDD home/prosumer NAS;
- one separate system SSD;
- optionally a dedicated Apps/Cache SSD/NVMe;
- mixed disk sizes;
- Samba/NFS/SFTP/FTP/rsync;
- mergerfs + SnapRAID;
- Docker Engine + Docker Compose;
- local-first monitoring, alerting and recovery.

This is a **NAS + Docker host**, not a router, hypervisor cluster, Kubernetes distribution, or enterprise SAN.

## Core product principles

### 1. Offline-first

A release ISO must contain everything needed for installation and core operation:

- Debian base;
- kernel and firmware;
- MyNAS binaries/UI;
- Docker Engine and Compose;
- Samba/NFS/OpenSSH/rsync/FTP implementation;
- mergerfs/SnapRAID;
- smartmontools;
- NetworkManager;
- Avahi;
- NUT;
- filesystem tooling.

Internet-dependent features should degrade gracefully and display why they are unavailable.

### 2. Data remains readable without MyNAS

Data disks use ordinary filesystems such as XFS/ext4.

mergerfs combines mounted paths, not proprietary block layouts.

If MyNAS disappears, an experienced Linux user should still be able to mount the disks and read files.

### 3. Safe by default

- unknown disks are never initialized automatically;
- destructive actions are a separate security class;
- all relevant resource dependencies are checked;
- configuration changes are transactional where possible;
- suspicious storage state freezes automation;
- the system fails closed rather than guessing.

### 4. Realtime but calm

The UI receives live state, but does not continuously animate every number.

Use:

- REST for initial snapshots/actions;
- SSE for status/events/job progress;
- WebSockets only for bidirectional interactions such as shell/terminal.

### 5. Docker Compose first

Docker stacks are standard Compose files.

Simple App UI and raw Compose editing are two views of the same stack.

No proprietary app runtime is required.

### 6. Progressive disclosure

Normal users see concepts such as:

- Media Pool;
- Photos;
- Apps SSD;
- Read & Write;
- Healthy.

Advanced users can reveal:

- WWN/UUID;
- mount options;
- raw SMART;
- PUID/PGID;
- routes/MTU;
- complete Compose YAML.

### 7. Recovery is part of normal operation

Recovery readiness is continuously calculated.

Configuration backup should be automatic and verified.

A system-disk failure must be treated as an expected event.

### 8. No forced cloud

Core functionality works with the Internet unavailable.

Tailscale, Slack, Telegram, Docker registries, Backblaze, S3, Let's Encrypt, etc. are optional integrations.

## Disk roles

MyNAS should model disks using semantic roles:

- **System** — OS, MyNAS state;
- **Apps/Cache** — Docker engine/appdata/databases/cache;
- **Data** — persistent files;
- **Parity** — SnapRAID parity;
- **Backup** — local backup destination;
- **External** — removable/imported;
- **Unknown** — not yet adopted.

Role is stored by stable device identity, never Linux device letter.

## Expected hardware baseline

Initial target:

- x86-64;
- UEFI and legacy BIOS install support if practical;
- SATA/AHCI;
- common NVMe;
- common Intel/Realtek NICs;
- USB storage;
- smartmontools-compatible SATA/SAS/NVMe health;
- optional HBA support where Linux exposes disks normally.

ARM can be considered later.

## Explicit product boundaries

Not initial core responsibilities:

- Kubernetes;
- Ceph;
- distributed storage;
- routing/NAT gateway appliance;
- DHCP server;
- IDS/IPS;
- Active Directory server;
- full virtualization platform.

These can be served by Docker or other dedicated products.

## Success metrics

A successful product should achieve:

- first installation from USB without Internet;
- no command-line requirement for normal setup;
- sub-second-feeling navigation on LAN after initial load;
- reliable live state without page refresh;
- safe import of existing disks;
- recovery from system-disk failure through installer;
- Compose portability;
- clear explanation of parity versus backup;
- useful alerts rather than raw Linux errors.
