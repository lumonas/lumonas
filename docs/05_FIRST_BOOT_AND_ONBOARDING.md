# First Boot and Onboarding

## Principle

The OS installer should install only the system. The first browser boot configures storage and recovery using the full modern UI.

Target wizard:

```text
Server → Storage → Protection → Recovery → Finish
```

Shares, Docker apps, notifications and cloud backups are suggested after the NAS becomes operational.

## Physical console after installation

Show a lightweight local status screen:

```text
LumoNAS — HomeNAS

Healthy
IP: 192.168.1.74
Web: http://lumonas.local

5 unconfigured disks detected

F2 Network
F3 Diagnostics
F4 Console
```

This is useful when browser access fails.

## Browser welcome

First visit should:

- verify administrator session;
- explain that setup can be changed later;
- not require Internet;
- show detected hardware summary.

## Step 1 — Server

Show/edit:

- friendly server name;
- hostname;
- timezone;
- LAN address;
- SSH state;
- update state.

Advanced network configuration is not required in onboarding.

## Step 2 — Storage discovery

Classify each physical disk:

- System;
- blank;
- existing filesystem/data;
- existing LumoNAS disk;
- suspected parity;
- removable/external.

Show cards/rows with:

- model;
- serial suffix;
- size;
- data state;
- current filesystem;
- recommended role.

### Existing data

Default action:

**Import without modifying data.**

Do not make erase the visually primary action.

### Blank mixed-size layout recommendation

For mergerfs + SnapRAID:

- choose largest suitable disk as parity;
- data disks may be different sizes;
- parity size must accommodate the largest protected data disk;
- show usable capacity;
- show that SnapRAID is scheduled parity, not realtime RAID.

The first sync is queued only after the generated SnapRAID configuration has
been accepted by the privileged broker. If configuration or disk identity
validation fails, onboarding remains completed but reports protection as
unconfigured and does not start a background sync against an unverified
layout.

Example:

```text
Data:
8 TB
8 TB
12 TB
12 TB

Parity:
12 TB

Usable data: 40 TB
Tolerance: one protected disk failure after successful sync
```

## Step 3 — Protection

Offer:

- SnapRAID parity selection;
- protected data disks;
- daily sync schedule;
- weekly scrub schedule;
- content-file replication.

Show clear wording:

> Changes made after the last successful sync are not yet fully represented in parity.

Initial sync runs as background job.

## Step 4 — Disaster Recovery

Strongly encourage automatic config protection.

Defaults:

- automatic config backup after meaningful config changes;
- daily snapshot;
- encrypted;
- copy to more than one data disk;
- generate recovery key;
- verify archive immediately.

Optional destinations:

- USB;
- another NAS;
- SFTP;
- S3/Backblaze.

Do not force cloud use.

## Recovery key UX

Provide:

- download;
- printable text;
- explicit acknowledgement;
- later rotation procedure.

Do not bind recovery solely to TPM because motherboard failure must remain recoverable.

## Finish

Show summary:

- data capacity;
- protection state;
- recovery readiness;
- jobs still running.

Suggested next tasks:

- Create share;
- Install Docker app;
- Configure notifications;
- Configure UPS;
- Go to Dashboard.

## Re-entry

If onboarding is interrupted:

- persist completed safe steps;
- resume from correct point;
- never rerun destructive disk initialization merely because browser state is lost.

## Accessibility

Wizard must support:

- keyboard navigation;
- screen-reader labels;
- high contrast;
- reduced motion;
- no color-only warning meaning.

## Acceptance criteria

A first-time user should be able to configure a common 4-data + 1-parity setup without seeing:

- `/dev/sdX`;
- raw mergerfs policies;
- SnapRAID CLI;
- UID/GID;
- Samba config syntax.

An expert can inspect these after setup.
