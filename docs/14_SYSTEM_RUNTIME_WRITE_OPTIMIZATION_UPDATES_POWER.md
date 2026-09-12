# System Runtime, Write Optimization, Updates and Power

## Goal

Keep LumoNAS lightweight and reduce unnecessary system-SSD writes without risking important state.

## Memory/write optimization

### Balanced — default

- zram swap;
- tmpfs for normal transient directories;
- aggregate metrics in RAM before persistence;
- bounded journald retention;
- Docker log limits;
- batched noncritical telemetry writes.

### Normal

More persistent logs and normal disk behavior.

### Maximum write reduction

For flash/eMMC/USB-style boot media:

- more volatile logs;
- RAM-backed selected temporary/log paths;
- periodic persistence;
- explicit warning about recent noncritical log loss after sudden power failure.

Do not place databases/appdata in volatile storage by default.

## zram

Use compressed RAM swap rather than a large default SSD swap file.

Expose:

- enabled;
- current compressed usage;
- compression ratio;
- pressure state.

## RAM transcode/cache

Optional per Docker app.

Provide bounded tmpfs for Jellyfin/Plex transcode.

User specifies maximum size.

## Docker logging

Set default log rotation.

UI:

- log driver;
- size limit;
- retained files;
- per-stack override;
- top log disk consumers.

## SSD wear

Display SATA/NVMe endurance fields when available:

- data written;
- percentage used;
- media errors;
- unsafe shutdowns;
- temperature;
- available spare.

Alert on worsening wear/error state.

## System updates

Separate:

1. LumoNAS core release;
2. Debian security updates;
3. Docker image updates.

Do not silently dist-upgrade major Debian releases.

## Update workflow

Before significant LumoNAS update:

1. configuration generation snapshot;
2. recovery backup verification;
3. download/verify signed package/image;
4. apply;
5. restart/reboot if needed;
6. health-check;
7. expose rollback when architecture permits.

## Future A/B system

Long-term preferred design:

```text
EFI
Recovery
System A
System B
State
```

Write new release to inactive system slot.

Boot and health-check.

Automatically return to previous slot on boot failure.

This is a future architecture track, not required for first functional build.

## Power

UI support:

- shutdown;
- reboot;
- scheduled shutdown/reboot;
- Wake-on-LAN config;
- maintenance mode.

Before clean shutdown:

- flush LumoNAS state;
- persist volatile critical queues;
- stop Docker cleanly;
- stop file services;
- sync filesystems;
- unmount pools where appropriate.

## UPS via NUT

Wizard:

- detect/configure UPS;
- battery level/runtime;
- power state;
- self-test where supported;
- shutdown policy.

Recommended sequence:

```text
utility power lost
→ alert
→ battery threshold/runtime threshold
→ stop heavy jobs
→ flush volatile state
→ stop apps/services
→ sync/unmount
→ shutdown
```

## Acceptance criteria

- Balanced mode does not make important config dependent on RAM.
- A clean shutdown flushes pending config/important events.
- Noisy Docker logs cannot grow without a configured bound.
