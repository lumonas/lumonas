# Product Acceptance Tests

These are user-visible end-to-end scenarios.

## A. Offline installation

Given:

- blank system SSD;
- five HDDs;
- no Internet.

User boots USB and installs.

Pass if:

- installation completes;
- data HDDs are untouched;
- system boots;
- `lumonas.local` is advertised on LAN;
- UI, Docker, SMB packages and storage tools are available.

## B. Existing data import

Given an XFS HDD containing files.

User selects Import.

Pass if:

- LumoNAS mounts/imports without formatting;
- sample file hashes remain unchanged;
- destructive erase is not default.

## C. Mixed-disk pool

Create mergerfs pool from mixed sizes.

Pass if:

- total capacity is correctly represented;
- individual files remain on normal backing disks;
- pool persists across reboot/device-letter reorder.

## D. SnapRAID protection

If the privileged configuration step fails or a disk identity is stale,
onboarding must not queue the initial sync. It must report protection as
unconfigured and leave the data disks untouched.

Configure parity, sync, create new files.

Pass if:

- UI clearly shows changes since sync;
- state is not misleadingly “fully protected” before next sync.

## E. Disk disappears

Remove a protected virtual disk.

Pass if:

- critical alert;
- Storage Safety Mode;
- automated sync blocked;
- no force action occurs.

## F. Wrong destructive target race

Plan format for disk A, then swap device mapping.

Pass if operation aborts.

## G. Share creation

Create Documents SMB share with two users.

Pass if Windows/macOS can access with expected read/write policy without manual Samba edit.

## H. Simple Docker

Install Jellyfin from form.

Pass if:

- app starts;
- storage selected in UI maps correctly;
- Compose file is standard.

## I. Advanced Compose preservation

Add custom unsupported YAML manually.

Then change a simple field in UI.

Pass if unsupported YAML remains.

## J. Common env variable

Change `MEDIA` storage reference.

Pass if:

- affected stacks listed;
- user previews;
- redeploy uses new resolved path.

## K. Network rollback

Apply unreachable static IP remotely.

Pass if old config returns after checkpoint timeout.

## L. Config recovery

Create backup, destroy system SSD, restore.

Pass if:

- users;
- shares;
- storage identities;
- SnapRAID config;
- Docker stacks;
- secrets (after recovery key);
- network mapping;
- all return correctly.

## M. Incomplete Docker backup

One app has no appdata protection.

Pass if Disaster Recovery UI explicitly reports it as not fully recoverable.

## N. UPS shutdown

Simulate UPS on-battery threshold.

Pass if:

- alert;
- heavy jobs stop;
- state flushes;
- containers stop;
- clean shutdown occurs.

## O. Notification routing

Trigger disk temperature test alert.

Pass if only configured routes receive firing and recovery notifications.

## P. Write optimization

Balanced mode over extended test.

Pass if:

- metrics/log storage remains bounded;
- configuration survives forced restart;
- Docker logs remain capped.

## Q. Security redaction

Create secrets with unique canary strings.

Generate diagnostics.

Pass if none of the canary strings appear.

## R. Mobile/PWA basics

Pass if phone can:

- view health;
- inspect disk alert;
- restart app;
- acknowledge alert;
- view active jobs.

## S. Config drift

Modify supported service config externally.

Pass if UI detects drift rather than silently claiming desired state equals actual state.

## T. Upgrade

Upgrade supported previous release.

Pass if:

- pre-update recovery verification runs;
- data/share/Docker configuration remains valid;
- recovery bundle from old version can migrate.
