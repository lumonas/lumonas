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

### Current state: image-based slot pipeline (implemented)

The core of this design is implemented behind the privileged broker:

- `POST /updates/slot/stage` verifies a signed root-filesystem image
  (ed25519 manifest, identical trust model to package updates) and stages it
  under the inactive slot directory of `LUMONAS_UPDATE_ROOT`;
- `POST /updates/slot/activate` re-verifies the staged image (drift between
  staging and activation fails closed), writes it to the inactive slot device
  via the `system.slot.write` broker operation (digest-checked `dd`, target
  must be a stable persistent whole-disk alias for an unmounted block device),
  and arms the bootloader through
  `system.slot.bootnext` (`efibootmgr --bootnext`);
- boot health and rollback reuse the existing `updates.Manager`
  boot-attempt machinery; `POST /updates/slot/confirm` commits the pending
  slot after the running version proves healthy;
- slot devices and EFI entries are configured per appliance with
  `LUMONAS_SLOT_DEVICES=a=<device>:<entry>,b=<device>:<entry>`.

`scripts/qemu-ab-smoke.sh` (`LUMONAS_AB_ASSERT=true`) exercises stage,
privileged write, tamper rejection, and BIOS fail-closed BootNext in QEMU.
The x86 QEMU image is GPT-partitioned with both a BIOS boot partition and an
EFI system partition. `scripts/qemu-uefi-ab-smoke.sh`
(`LUMONAS_UEFI_AB_ASSERT=true`) boots it through OVMF, registers both slot
entries against distinct GPT identities, rewrites the inactive whole disk
through the broker, arms `BootNext`, tolerates the expected SSH disconnect,
and verifies after a real reboot that the root filesystem came from slot B. Its
SSH and HTTPS health probes use separate QEMU host forwards, preventing a
working SSH control channel from hiding a failed web listener.

### Remaining for full immutable A/B

- secure boot / UKI integration;
- atomic deduplicated images;
- background automatic updates.

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
- LumoNAS signing chain;
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

A tiny Rust `lumonas-privd` could be considered later if memory-safety/threat review justifies a second language. Avoid complexity unless there is a concrete gain.
