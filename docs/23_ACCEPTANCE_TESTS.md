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

The release-blocking Debian 13 QEMU recovery test exercises this path against a
blank replacement disk. Its fixture is created through the production SQLite
migrations and verifies restored users/groups, share protocol ACLs, committed
configuration generation, Compose YAML, mergerfs/SnapRAID configuration, disk
identity metadata, encrypted secrets, and the recovery result after shutdown.
The hook also starts the backend against the restored filesystem and checks the
principals and shares API before powering off, so API readiness is not supplied
only by the live ISO runtime.

## M. Incomplete Docker backup

One app has no appdata protection.

Pass if Disaster Recovery UI explicitly reports it as not fully recoverable.

The API also exposes the matched catalog recovery contract for known images and
uses a conservative default for imported stacks; this metadata must not be
interpreted as appdata content backup.

The recovery bundle format additionally accepts verified appdata archives and
restores them only to approved data roots after traversal, symlink, special-file,
and size-limit checks.

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

## U. Dependency and image security

Run the release dependency controls on a clean checkout.

Pass if:

- Go vulnerability checks pass;
- production frontend dependencies have no HIGH or CRITICAL advisories;
- repository dependency manifests pass the Trivy scan;
- every catalog container image passes the Trivy scan;
- the tagged release is blocked when any of these checks fails.

## V. systemd unit validation

Run packaging checks on the Debian/Ubuntu build runner.

Pass if every packaged service unit passes `systemd-analyze verify` and the
release job fails when the verifier is unavailable.

## W. QEMU service identity

Boot the Debian appliance and query `/api/v1/services`.

Pass if the three LumoNAS services are active/running and
`lumonas-web.service` reports `user: lumonas`.

## X. Package manifest

Inspect the generated `.deb` with `verify-deb.sh`.

Pass if the embedded build manifest matches the package version, architecture,
and exact Debian dependency fields, and contains non-empty source/toolchain and
input hashes.

## Y. Daemon restart and SSE replay

Run the `lumonasd` restart and event-stream tests.

Pass if interrupted queued/preparing/running jobs become failed with a
completion timestamp, and an SSE client resuming from `Last-Event-ID` receives
only events after that cursor before live delivery begins.

## Z. Read-only protection discovery

Query the protection configuration endpoint with a generated SnapRAID config.

Pass if it reports stable parity/data disk identities and performs exactly one
bounded `snapraid status` probe without issuing a mutating storage command.

## AA. Filesystem deployment coverage

Run the loopback storage smoke test from the packaged/QEMU dependency set.

Pass if both ext4 and XFS images are created, reattached by stable filesystem
identity, mounted read-only, and checked with `findmnt`; missing `xfsprogs`
must fail the release gate instead of silently skipping XFS coverage.

## AC. Real pool and protection integration

Run the privileged loopback smoke test with mergerfs and SnapRAID installed.

Pass if two disposable filesystem branches form a real mergerfs pool, a file
created through the pool is found on a backing branch, the pool unmounts
cleanly, and `snapraid status` completes against the disposable configuration.

## AB. Privileged IPC timeout

Run the privileged client cancellation test with a worker that reads a request
but never responds.

Pass if the client returns on the caller deadline and does not leave the API
job blocked on a connected Unix socket.

## AD. Package filesystem permissions

Install the generated `.deb` in a disposable Debian 13 container and inspect
the provisioned service account, configuration files, and runtime directories.

Pass if `/etc/lumonas` and its environment files are root-owned with the
documented group-readable modes, the `lumonas` account is non-root and cannot
modify those files, and it can write only to the provisioned runtime,
recovery, disk, pool, and appliance data directories. The package-permissions
job is release-blocking.

## AH. Journald retention policy

Run the packaging and log-retention smoke tests.

Pass if the packaged journald drop-in enforces bounded system/runtime usage,
30-day retention, and seven-day log-file rotation.

## AE. Disk API identity contract

Run the backend disk contract test and the OpenAPI parity check.

Pass if a disk response preserves stable identity fields (`wwn`, GPT disk
GUID, partition UUID, filesystem UUID), the current path, mount state, and
last-seen timestamp, and those fields are represented in the OpenAPI and
frontend contracts.

The collector test also verifies that `lsblk` requests and returns the GPT
partition-table UUID, and that it is preferred over filesystem UUID/path for
stable identity fallback.

The same gate checks the complete SSE envelope field set in OpenAPI and the
frontend types, including correlation and operation metadata.

## AF. SSE observability envelope

Run the event encoding test and resume an SSE client from `Last-Event-ID`.

Pass if the JSON event preserves `correlationId`, `operationId`, `planHash`,
`actor`, `generation`, resource identity, schema version, and payload data,
and the frontend event type exposes the same optional metadata.

## AG. Destructive storage fail-closed gate

Run the privileged storage safety tests and the Linux loopback smoke test.

Pass if missing disks, mounted disks, and disks reported as mounted by
`findmnt` are rejected before `mkfs` or `wipefs`, and the real loopback test
can create both mergerfs branches and complete SnapRAID status cleanup.

The same gate injects failed SnapRAID sync and scrub commands and requires
both operations to finish failed without updating protection success metadata.

## AI. Network diagnostic command boundary

Run the network diagnostic command-module tests.

Pass if ping/traceroute use direct allow-listed arguments, bounded output and
context cancellation, and unsupported or failed probes are reported as
failures without shell execution.

## AJ. Extended disk identity revalidation

Create a destructive or pool plan, then change only the GPT disk GUID or
partition UUID while retaining the same device path and other metadata.

Pass if confirmation and privileged execution reject the stale plan before any
filesystem, pool, or protection command runs.

## AK. Bounded integration command execution

Run the WireGuard and SFTP command-boundary tests.

Pass if exact command arguments and stdin are captured through injectable
bounded runners, and no integration path constructs a shell command.

## AL. Mutation operation identity

Submit a confirmed filesystem or SnapRAID mutation without an operation ID.

Pass if `lumonas-privd` rejects it before disk discovery or command execution;
normal API confirmations and scheduled jobs must include the operation ID.

## AM. Privileged journal observability

Inspect a broker request/result pair in the service journal using a test
operation containing sensitive requested-state and identity values.

Pass if structured records include operation/correlation/plan/result metadata,
while sensitive payload values are absent and the packaged units identify their
logs as `lumonas-privd`, `lumonasd`, and `lumonas-web`.

## AN. Real frontend appliance smoke

Boot the Debian appliance through the QEMU release smoke and fetch `/` from
the packaged web service.

Pass if the response contains the compiled LumoNAS title and React root
element, in addition to the API, SSE, service identity, and disk assertions.

## AO. Wake-on-LAN command boundary

Run the network WOL tests and settings API test.

Pass if `ethtool` capability discovery distinguishes supported and enabled
states, settings changes use the typed `network.wol.set` broker operation, and
unsafe interface names are rejected before command execution.

## AP. Scheduled power safety

Run the power schedule and settings API tests.

Pass if invalid actions, clock formats, and day selections are rejected, a due
schedule is matched only once per local minute, and execution uses the typed
`power.shutdown` broker request with an operation ID.

## AQ. Docker log retention

Run the log-retention and packaging smoke tests.

Pass if the package ships a Docker `json-file` baseline capped at 10 MiB per
file and three files, applies it only when `/etc/docker/daemon.json` is absent,
and never overwrites an administrator-owned Docker configuration on upgrade.

## AR. Docker mutation authorization

Attempt Docker stack, container, image, and offline-import mutations without a
management session.

Pass if every mutation is rejected before the Docker command runs, while a
successful mutation produces a correlated audit record.

## AS. Release signature verification

Run tagged-release artifact verification with Cosign available.

Pass if every `.deb`, ISO, and QEMU artifact has a signature and verification
bundle, and `cosign verify-blob` validates the artifact against the configured
GitHub Actions OIDC issuer and tag workflow identity.
