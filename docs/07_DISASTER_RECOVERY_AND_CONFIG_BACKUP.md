# Disaster Recovery and Configuration Backup

## Recovery objective

Primary scenario:

```text
system SSD fails
→ replace SSD
→ boot MyNAS USB
→ Recover existing MyNAS
→ unlock recovery backup
→ validate disks/hardware
→ restore configuration/app state
→ verify
```

User data disks remain untouched throughout discovery and planning.

## Recovery layers

### R0 — OS

Reinstall from release image. Do not rely on block-level OS backup.

### R1 — MyNAS desired state

Must be strongly recoverable:

- hostname/timezone;
- users/groups;
- ACL definitions;
- shares;
- network;
- firewall;
- Docker Compose;
- global variables;
- encrypted secrets;
- mergerfs;
- SnapRAID;
- schedules;
- notifications;
- backups;
- certificates/keys where selected.

### R2 — Docker application state

Recoverability depends on configured app backup strategy.

### R3 — user data

Lives on data disks and/or backup targets. SnapRAID parity is not a substitute for backup.

## Backup format

Use a versioned MyNAS Recovery Bundle, e.g.:

```text
*.mrb
```

Contents:

```text
manifest.json
desired-state.json
mynas.db export/safe snapshot
docker/stacks/
acl/
certificates/
encrypted-secrets/
checksums/
```

Manifest includes:

- backup format version;
- config schema;
- MyNAS version;
- NAS UUID;
- generation;
- timestamp;
- disk identities;
- pool identities;
- checksums.

## Encryption

Sensitive bundle sections must use authenticated encryption.

Generate an independent recovery key.

TPM may be an optional convenience but never the sole recovery mechanism.

## Configuration generations

Each committed desired-state change increments a generation:

```text
1840
1841
1842
```

Backup metadata on disks records the newest known generation.

During recovery, compare available copies and choose newest valid consistent generation.

## Backup destinations

Default:

- system SSD local copy;
- multiple adopted data disks.

Recommended optional:

- USB backup;
- second NAS;
- SFTP;
- S3-compatible;
- Backblaze.

Config backups are small enough to retain many generations.

## On-disk MyNAS metadata

Each adopted data disk may contain:

```text
/.mynas/
├── disk.json
└── recovery/
```

`disk.json` contains no secrets.

Store:

- NAS UUID;
- disk stable identity;
- role;
- pool ID;
- filesystem UUID;
- SnapRAID role;
- last config generation.

## Automatic schedule

Create recovery snapshot:

- after meaningful desired-state commits;
- before disruptive updates;
- daily;
- before large migrations.

Retention example:

- last 20 config generations;
- 30 daily;
- 12 monthly if remote space is cheap.

## Verification

A backup is not “healthy” merely because the file exists.

Verify:

- checksum;
- decryptability;
- manifest schema;
- SQLite consistency;
- Compose syntax;
- referenced secret objects;
- disk manifest parse;
- expected files.

Expose:

```text
Latest backup: Verified
Recovery readiness: 96%
```

## Restore planning

Restore is staged:

1. inspect backup;
2. discover disks read-only;
3. match by stable identity;
4. compare old/new network hardware;
5. inspect app backups;
6. generate restore plan;
7. user approves;
8. install/restore;
9. health-check;
10. commit restored generation.

## Hardware remapping

If motherboard/NIC changes:

```text
Old: Intel I219-V
New: Realtek RTL8125
```

Offer explicit mapping of previous profile to new interface.

Never blindly apply interface-name-specific config.

## Docker recovery contract

Each stack has recovery metadata:

- Compose;
- environment;
- secrets;
- bind mounts;
- named volumes;
- databases;
- pre-backup action;
- post-backup action;
- restore action.

Catalog apps can ship known recovery contracts.

Unknown Compose default:

**stop stack → backup configured appdata → restart**

Alternative advanced modes:

- crash-consistent;
- custom hooks;
- external snapshot.

## Recovery readiness UI

Show per layer:

```text
Configuration: current/verified
Storage metadata: all disks
Docker stacks: 18/18
Docker appdata: 17/18
Remote backup: current
Recovery key: configured
```

Never claim an app is fully recoverable if its mutable state is not protected.

## Partial restore

Design format to allow future selective restore:

- network;
- shares;
- Docker;
- notifications;
- users.

Initial implementation may restore all R1 state, but schema should keep modules separable.

## Recovery CI

For every release:

1. provision QEMU NAS with multiple virtual disks;
2. configure users, shares, static network, SnapRAID, Docker;
3. create known test files/checksums;
4. create and verify recovery backup;
5. delete system virtual disk;
6. attach blank replacement;
7. boot installer offline;
8. restore;
9. compare configuration;
10. compare data hashes;
11. verify services.

Failure blocks release.

## Product wording

Use:

> A failed system disk should be an inconvenience, not a disaster.

Do not promise that parity or an unconfigured Docker app backup can recover data that was never backed up.
