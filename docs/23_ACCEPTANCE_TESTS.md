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

The QEMU release smoke compares stable identity metadata independently of
`currentPath` and requires at least one changed transient path; a stable
sorted list of IDs alone is insufficient.

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

## K1. Offline ISO HTTPS

Boot the generated offline ISO under QEMU and query the live web console.

Pass if the live ISO web service exposes health, readiness, and server routes
over HTTPS with the generated certificate, while the recovery daemon remains
bound to its separate loopback-only API port.

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
blank replacement disk. Tagged CI first configures a disposable Debian source
appliance through the real API, including ext4 mounts, a mergerfs pool, a LAN
profile, appdata, users, shares, Compose, and SnapRAID. It then verifies the
encrypted export and restores it through the offline ISO. Its fixture fallback
is created through the production SQLite migrations and verifies restored
users/groups, share protocol ACLs, committed configuration generation, Compose
YAML, mergerfs/SnapRAID configuration, disk identity metadata, encrypted
secrets, and the recovery result after shutdown.
The hook also starts the backend against the restored filesystem and checks the
principals and shares API before powering off, so API readiness is not supplied
only by the live ISO runtime. It also queries the restored storage-mounts API and
network-connections API, and storage-mounts API; these checks cover the LAN
mapping, mergerfs pool, and stable disk branch identities.
Before booting the ISO, the host harness must independently verify the bundle
with the production `lumonas-recover` binary and confirm its checksums and
payload validity.

The production recovery API gate also creates a user, managed SMB/NFS share,
Compose stack, encrypted secret, and generation through the live handlers,
then exports, verifies, applies, and reopens the restored database. It must
pass independently of the QEMU fixture so recovery export regressions cannot
be hidden by fixture-only coverage.

Tagged CI additionally boots the generated Debian appliance as the configured
source, persists its exported bundle, shuts it down cleanly, and performs the
offline restore against a blank replacement disk. The fixture-only path is
retained for local development but is not the release source of truth.

## M. Incomplete Docker backup

One app has no appdata protection.

Pass if Disaster Recovery UI explicitly reports it as not fully recoverable.

The API also exposes the matched catalog recovery contract for known images and
uses a conservative default for imported stacks; this metadata must not be
interpreted as appdata content backup.

The recovery bundle format additionally accepts verified appdata archives and
restores them only to approved data roots after traversal, symlink, special-file,
and size-limit checks.

For a configured stack, the exporter must resolve every declared appdata path,
stop and restart the stack for the default contract, and mark the recovery
status incomplete if any source or lifecycle step fails.
It must also reject traversal stack names and duplicate service mounts rather
than selecting a source nondeterministically.

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

Pass if none of the canary strings appear, including canaries in recovery-key
files and PEM private-key blocks.

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

The same release controls must execute bounded Go fuzzing for path validation,
backup manifests, Compose transformations, network payloads, recovery bundles,
and storage configuration, rather than only running their seed cases.

Run `scripts/request-limits-smoke.sh`.

Pass if oversized JSON and multipart requests are rejected before the handler
runs, while an accepted multipart request is not parsed until endpoint
authentication and routing have completed.

## V. systemd unit validation

Run packaging checks on the Debian/Ubuntu build runner.

Pass if every packaged service unit passes `systemd-analyze verify` and the
release job fails when the verifier is unavailable.

## W. QEMU service identity

Boot the Debian appliance and query `/api/v1/services`.

Pass if the three LumoNAS services and the runtime provisioning unit are
active/running, and `lumonas-web.service` reports `user: lumonas`.

## X. Package manifest

Inspect the generated `.deb` with `verify-deb.sh`.

Pass if the embedded build manifest matches the package version, architecture,
and exact Debian dependency fields, and contains non-empty source/toolchain and
input hashes.

## Y. Daemon restart and SSE replay

Run the black-box API smoke and `lumonasd` restart/event-stream tests.

Pass if interrupted queued/preparing/running jobs become failed with a
completion timestamp, and an SSE client resuming from `Last-Event-ID` receives
only events after that cursor before live delivery begins. The smoke must do
this against the same persisted SQLite database after restarting the daemon.

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
cleanly, `snapraid status`, `sync`, and `scrub` complete against the
disposable configuration, and a missing SnapRAID configuration fails closed.

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

## AI. Upgrade service restart ordering

Run the upgrade service ordering smoke test and Debian package verification.

Pass if the upgrade stops `lumonas-web`, `lumonasd`, the privileged workers,
and broker before unpacking, then starts the broker, workers, daemon, and web
service in dependency order.

The same upgrade must show that `lumonas-migrate` is packaged, runs as the
`lumonas` service user before startup, creates or upgrades the configured
SQLite database, and returns a failing package transaction when migrations
cannot be applied.

## AJ. Systemd sandbox policy

Run `LUMONAS_REQUIRE_SYSTEMD_SECURITY=true bash scripts/systemd-security-smoke.sh`
and `systemd-analyze verify` against the packaged units.

Pass if every unit has bounded resources and the required filesystem/process
sandboxing, the web and management services run as the unprivileged `lumonas`
user, and privileged workers are restricted to Unix sockets with an explicit
capability bounding set. This check is release-blocking.

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

It also checks that all frontend query and mutation paths resolve to documented
backend routes after normalizing dynamic path parameters.

Run `scripts/disk-identity-smoke.sh`.

Pass if missing identity fields are enriched from read-only udev properties,
existing `lsblk` identity wins over fallback values, and the command arguments
remain fixed to the device path rather than a shell expression.

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
The same loopback gate formats and erases only a disposable test image,
verifying filesystem UUID creation and signature removal.

The same gate injects failed SnapRAID sync and scrub commands and requires
both operations to finish failed without updating protection success metadata.

The privileged storage loopback harness must also exercise the real
`lumonas-privd` storage worker against a disposable loop device, including
read-only mounting and rejection of mounted, stale, expired, and operation-ID
missing requests before erase.

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

## AL. FTPS certificate integration

Run the share configuration and packaging smoke tests.

Pass if generated FTPS configuration uses the provisioned
`/etc/lumonas/tls/server.crt` and `server.key` paths and rejects obsolete
certificate locations.

## AM. Bounded integration command policy

Run `scripts/command-boundary-smoke.sh`.

Pass if ordinary production integrations construct external commands only
through the shared bounded runner, the interactive NetworkManager checkpoint
remains process-group bounded with a finite deadline, and service code has no
shell command escape hatch.

The same gate must reject command output larger than the shared 1 MiB capture
limit instead of returning success with unbounded memory use.

## AN. Mutation operation identity

Submit a confirmed filesystem or SnapRAID mutation without an operation ID.

Pass if `lumonas-privd` rejects it before disk discovery or command execution;
normal API confirmations and scheduled jobs must include the operation ID.

## AN. Privileged journal observability

Inspect a broker request/result pair in the service journal using a test
operation containing sensitive requested-state and identity values.

Pass if structured records include operation/correlation/plan/result metadata,
while sensitive payload values are absent and the packaged units identify their
logs as `lumonas-privd`, `lumonasd`, and `lumonas-web`.

## AO. Real frontend appliance smoke

Boot the Debian appliance through the QEMU release smoke and fetch `/` from
the packaged web service.

Pass if the response contains the compiled LumoNAS title and React root
element, in addition to the API, SSE, service identity, and disk assertions.
The production bundle smoke must also pass without MSW bootstrap code; mock
handlers remain limited to the explicit local demo mode.

## AP. Wake-on-LAN command boundary

Run the network WOL tests and settings API test.

Pass if `ethtool` capability discovery distinguishes supported and enabled
states, settings changes use the typed `network.wol.set` broker operation, and
unsafe interface names are rejected before command execution.

## AQ. Scheduled power safety

Run the power schedule and settings API tests.

Pass if invalid actions, clock formats, and day selections are rejected, a due
schedule is matched only once per local minute, and execution uses the typed
`power.shutdown` broker request with an operation ID.

## AR. Docker log retention

Run the log-retention and packaging smoke tests.

Pass if the package ships a Docker `json-file` baseline capped at 10 MiB per
file and three files, applies it only when `/etc/docker/daemon.json` is absent,
and never overwrites an administrator-owned Docker configuration on upgrade.

## AS. Docker mutation authorization

Attempt Docker stack, container, image, and offline-import mutations without a
management session.

Pass if every mutation is rejected before the Docker command runs, while a
successful mutation produces a correlated audit record.

## AT. Release signature verification

Run tagged-release artifact verification with Cosign available.

Pass if every `.deb`, ISO, and QEMU artifact has a signature and verification
bundle, and `cosign verify-blob` validates the artifact against the configured
GitHub Actions OIDC issuer and tag workflow identity.

The ISO’s embedded APT repository must additionally contain signed `Release`
metadata and its archive keyring on tagged builds; a missing repository
signing fingerprint or private-key secret fails the build.

## AU. Release artifact manifest

Run `scripts/release-artifacts.sh` and strict `scripts/verify-release.sh` on a
complete release directory.

Pass if `RELEASE-MANIFEST.json` records the source commit, reproducible source
epoch, SHA-256 digest, and byte size for every Debian package, ISO, and QEMU
image, strict verification matches the source commit and reproducible epoch to
the tagged GitHub revision, and an artifact added without regenerating the
manifest is rejected.

The generated ISO and QEMU images must also expose their versioned Debian
package inventories under `/usr/share/doc/lumonas/`, including the source
commit, source epoch, and exact installed package versions.

## AV. Release gate policy

Run `scripts/release-gate-policy-smoke.sh`.

Pass if tagged publication depends on the package, QEMU, ISO, recovery,
storage-safety, integration, security, dependency, race/fuzz,
schema-compatibility, and upgrade jobs, so a failed required gate cannot still
publish release artifacts.

## AW. Disk-full behavior

Run `scripts/disk-full-smoke.sh` on the Linux CI runner.

Pass if a disposable ext4 filesystem filled beyond 95% is reported by the
production collector as `critical`, and the gate blocks tagged publication.

## AX. Operational SQLite retention

Run `scripts/retention-smoke.sh`.

Pass if notification deliveries, completed backup runs, expired storage
plans, and completed network checkpoints are bounded to the configured
history window, while active sessions, active backup work, pending network
rollback state, firing/acknowledged alerts, and unexpired storage plans remain
available. Expired sessions and resolved alert history must be removed. The
daemon must bound committed configuration generations while preserving
pending generations, and run the same policy at startup and periodically
while serving requests.

## AY. Generated share protocol validation

Run `scripts/share-protocol-smoke.sh` and `scripts/share-config-smoke.sh`.

Pass if generated NFS, SFTP, FTP/FTPS, and rsync configuration is parsed by
protocol-specific validators before activation, unsafe paths and malformed
directives fail closed, and the active configuration remains unchanged after a
validation failure. The validator smoke must run in the release-blocking share
integration job.

## AZ. Secret scanning

Run `scripts/secret-scan-smoke.sh`.

Pass if tracked files are scanned for high-confidence private-key, cloud-token,
package-token, and credential-URL formats, synthetic leaks are rejected, safe
fixtures pass, and the scan runs in the release-blocking security-controls job.

## BA. Release-blocking fuzz coverage

Run `LUMONAS_FUZZ_TIME=5s bash scripts/fuzz-smoke.sh`.

Pass if the release-blocking fuzz job exercises Compose/YAML transformations,
backup and recovery manifests, path and storage-plan validation, generated
share protocol configuration, network connection payloads, and API-adjacent
credential/path validation without panics.

## BB. Firewall activation safety

Run the release-blocking safety-recovery tests.

Pass if firewall rules are staged and activated only through the typed
privileged worker, every firewall mutation carries an operation ID and expiry,
and a missing broker or failed validation leaves the previous generated
ruleset unchanged.

## BC. Privileged identity mutation scope

Pass if system-user, Samba-user, and ACL mutations are rejected without an
operation ID and production provisioning requests include operation IDs.
The same invariant applies to network checkpoint/Wi-Fi/WOL changes, service
reloads, and power actions.

## BD. Package service-start failure policy

Pass if `postinst` fails on a live systemd host when a required LumoNAS unit
cannot start, while package installation remains usable in chroots without
`/run/systemd/system`.
Upgrade removal must likewise fail on a live systemd host when an ordered
service cannot stop, while retaining the chroot compatibility path.

## BE. Privileged IPC frame bounds

Run the privileged client and worker forwarding tests with an oversized JSON
response.

Pass if both the management client and root-owned broker reject responses above
the one-MiB frame limit without returning success or retaining an unbounded
buffer.

## BF. Recovery export completeness

Run the recovery API smoke tests with disk discovery and NAS identity failures.

Pass if recovery export returns an error instead of creating a verified bundle
when required stable-disk or appliance identity metadata is unavailable.
The successful export path must also leave a separately named, checksum-verified
generation copy that can be opened independently of `latest.mrb`.

## BG. Notification provider response bounds

Run the security smoke tests against a provider returning more than 64 KiB.

Pass if delivery rejects the response within the bounded read and records a
failure eligible for the existing retry/suppression path.

## BH. Support-bundle collection completeness

Run the support-bundle test with disk discovery unavailable.

Pass if the archive remains downloadable, its disk section is empty rather than
fabricated, and `server.json` records the redacted `disks: unavailable` status.

## BI. Notification failure reporting

Run notification delivery with unreadable encrypted channel credentials.

Pass if a sanitized `failed` delivery row is persisted and the channel enters
the existing bounded suppression policy without exposing credential details.
The first two consecutive failures must remain eligible for delivery, while
the third activates the cooldown and later success clears it.

## BJ. Network checkpoint persistence rollback

Run the network checkpoint persistence safety test.

Pass if a failure to persist a pending NetworkManager checkpoint invokes the
typed privileged rollback operation, restores the prior connection state, and
returns an error. A checkpoint must not remain active without a durable
rollback record.
The commit and rollback endpoints must likewise report an incomplete result if
their durable completion state cannot be written after the privileged action.

## BK. Storage execution state persistence

Run the storage confirmation persistence safety test.

Pass if a destructive storage command is never reported as fully successful
when its executed-plan state cannot be persisted. The API must identify the
operation as completed with incomplete persistence and must not emit a normal
success event.

## BL. Privileged WireGuard mutation

Run the WireGuard worker and API broker tests.

Pass if the unprivileged daemon never invokes `wg set` directly, the request
requires an operation ID and expiry, the network worker validates the typed
configuration, and the private key is supplied through bounded stdin rather
than command arguments or logs.

## BM. Privileged Tailscale mutation

Run the Tailscale worker and API broker tests.

Pass if connect, disconnect, and exit-node changes are routed through the
network worker, require typed allow-listed operations with an operation ID and
expiry, validate host/IP inputs, and produce sanitized audit/event metadata.
