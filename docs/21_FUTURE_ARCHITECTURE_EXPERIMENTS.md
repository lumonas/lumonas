# Future Architecture Experiments

These are promising directions but should be validated experimentally before becoming permanent dependencies.

## A/B OS system slots

Goal: make OS upgrades atomic.

Possible layout:

```text
EFI
Recovery
System A
System B
State
```

Upgrade writes inactive slot.

Boot health-check decides commit/rollback.

Challenges:

- Docker/package integration;
- kernel modules;
- state schema compatibility;
- disk space;
- Debian update model.

## Immutable/semi-immutable root

Consider a read-only or image-based base OS with mutable state in dedicated partitions.

Benefits:

- known system state;
- simpler rollback;
- less config drift.

Costs:

- debugging/admin expectations;
- package customization;
- hardware driver additions.

## mkosi-generated machine images

Evaluate mkosi for:

- reproducible system disk images;
- qcow2 test artifacts;
- A/B images;
- UKI/secure-boot workflows.

Keep USB installer needs separate if required.

## Secure Boot

Future:

- signed kernel/UKI;
- MyNAS signing chain;
- recovery compatibility.

Do not make initial releases unusable on common self-built hardware because of premature Secure Boot complexity.

## TPM

Use as optional convenience:

- secret unseal on same machine;
- update integrity.

Never make recovery bundle decryptable only by original TPM.

## systemd-sysext

Potential modular delivery of selected host components.

Investigate only if it simplifies immutable base architecture.

## Filesystem snapshots

If future storage modules support snapshots:

- app-consistent backup;
- SMB previous versions;
- rollback.

Do not pretend SnapRAID/mergerfs provide snapshots.

## eBPF telemetry

Could improve per-process/network/disk insight, but may add kernel/version complexity.

Only adopt if simpler `/proc`, cgroup and Docker metrics are insufficient.

## Rootless/containerized management components

Explore reducing privilege further, but storage/network core still needs controlled host access.

## Rust privileged helper

Go is recommended for core implementation consistency.

A tiny Rust `mynas-privd` could be considered later if memory-safety/threat review justifies a second language. Avoid complexity unless there is a concrete gain.
