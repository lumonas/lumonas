# Storage, Disks, Pools and SnapRAID

## Initial supported filesystems

Recommended:

- XFS;
- ext4.

Keep the first storage model intentionally simple.

Btrfs/ZFS can be future modules.

## Physical disk model

Store:

- stable LumoNAS disk ID;
- WWN;
- serial;
- model;
- size;
- bus/controller;
- current kernel path;
- rotational flag;
- SMART capability;
- filesystem/partition identities;
- semantic role;
- LumoNAS metadata generation.

## Disk UI

Default list:

- Name/model;
- Role;
- Capacity;
- Used;
- Temperature;
- Health.

Advanced columns:

- device path;
- serial;
- WWN;
- UUID;
- SMART;
- power-on hours;
- interface;
- standby.

## SMART

Normal UI translates SMART into understandable signals:

- overall status;
- temperature;
- reallocated sectors;
- pending sectors;
- uncorrectable sectors;
- CRC errors;
- SSD/NVMe wear;
- test history.

Raw SMART remains available.

Schedule:

- short test weekly by default;
- extended test monthly or user-customized;
- avoid colliding with heavy backup/scrub windows.

## Disk power management

Per rotational disk:

- disabled;
- 15/30/60/120 minute standby presets;
- advanced custom value;
- wake/sleep history;
- avoid overly aggressive defaults.

Warn that frequent spin cycling may be undesirable for some workloads.

## mergerfs

First-class pool type for mixed-size HDDs.

LumoNAS owns semantic pool configuration and renders mount options.

Default policy should favor predictable directory placement and sensible free-space behavior; expose advanced mergerfs policy selection later.

Pool UI shows backing disks individually so the user understands files physically remain on normal filesystems.

## Pool paths

Canonical user-facing resource:

```text
Pool: Media
```

Effective mount:

```text
/srv/pools/media
```

Disk branch mounts:

```text
/srv/disks/<disk-id>
```

Docker templates should refer to storage-resource variables rather than hardcoded branch paths.

## SnapRAID

Model separately from mergerfs.

Concepts:

- parity disk(s);
- protected data disks;
- content files;
- sync;
- scrub;
- status;
- fix/recovery.

### Content file replication

Automatically create multiple content copies across distinct devices, including system and selected data disks when safe.

Configuration backup records their locations.

### Protection status

Never show only “Healthy” when unsynced changes are large.

Display:

```text
Parity: Healthy
Last sync: 5h ago
Changes since sync: 8.3 GB
```

Severity should increase if:

- last successful sync exceeds policy;
- changed data exceeds threshold;
- a protected disk disappears;
- parity/content files are unavailable.

### Scheduling

Support:

- fixed daily time;
- custom cron-like UI;
- optional change threshold;
- minimum interval guard.

Scrub:

- weekly default;
- percentage strategy aligned with SnapRAID behavior;
- show last verified data age.

## Failed disk workflow

When disk disappears:

1. freeze modifying automation;
2. identify missing stable disk;
3. raise critical alert;
4. show dependent resources;
5. guide physical replacement;
6. recognize candidate replacement disk;
7. verify size and identity;
8. generate recovery plan;
9. run SnapRAID recovery;
10. verify recovered filesystem/files;
11. re-add to mergerfs;
12. unfreeze automation.

Do not silently run recovery.

## Import existing disks

Scan read-only first.

Show:

- filesystem;
- used space;
- LumoNAS metadata if any;
- SnapRAID content files if detected;
- possible mergerfs relationship.

Options:

- import as standalone;
- add existing path as pool branch;
- reconstruct existing LumoNAS pool;
- erase and initialize (destructive).

## Filesystem check

Provide wizard:

- stop dependent services;
- enter maintenance state;
- unmount;
- run read-only check first where supported;
- present proposed repair;
- require explicit confirmation for modifications;
- remount;
- health-check dependencies.

## Capacity forecasting

Collect daily used-space snapshots.

Forecast:

- linear recent growth;
- confidence/volatility;
- estimated full date.

Display only when enough history exists; do not show false precision.

## Acceptance criteria

- Reboot/device-letter reorder does not change disk roles.
- Pulling a protected disk blocks automated SnapRAID sync.
- Existing filesystem import does not write to disk before approval.
- Removing a pool disk with dependencies is blocked.
- Data files remain directly readable by mounting individual data disks.
