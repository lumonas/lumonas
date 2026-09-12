# Testing, CI and Release Engineering

## Philosophy

LumoNAS touches real disks. Safety cannot rely only on code review.

Automate destructive failure scenarios in disposable virtual environments.

## Test layers

### Unit

- identity normalization;
- operation planning;
- config validation;
- YAML patch preservation;
- alert state machine;
- backup manifest;
- migrations.

Retention tests cover bounded terminal job history while ensuring active jobs
survive pruning.

### Integration

Run system tools in containers/VMs where appropriate:

- Samba config rendering/validation;
- NetworkManager profiles;
- Docker Engine interactions;
- SnapRAID test arrays;
- mergerfs mounts;
- filesystem identity.

Share integration tests also verify that generated Samba, rsync, and vsftpd
paths are consumed by systemd drop-ins, while NFS and SFTP activation remains
confined to typed privileged configuration targets.
The FTPS renderer is also checked against the certificate paths provisioned by
the Debian package, so an enabled FTPS share cannot point at stale TLS names.
The dedicated share-integration CI gate runs these production-path tests
independently and is required by the tagged release job.

### QEMU end-to-end

Virtual NAS:

- system disk;
- several data disks;
- parity disk;
- NIC.

Run:

- installer;
- onboarding;
- storage;
- share;
- Docker;
- backup/recovery.

The generated offline ISO is also booted under QEMU with a blank replacement
disk. The smoke test waits for the live image's health, readiness, and server
API before the ISO artifact is considered valid.

The release ISO job additionally attaches a disposable recovery media image
containing a verified fixture bundle and a blank replacement disk. A
systemd-managed recovery helper identifies both devices by stable virtio
serial, formats only the blank target, runs `lumonas-recover --apply`, and
shuts the guest down. CI mounts the resulting target image and verifies the
restored desired state, Compose data, encrypted payload, and completion marker.
The host harness first runs the production `lumonas-recover` planner binary
against the same bundle and requires verified checksums plus valid database,
desired state, and Compose payloads before the guest boots; CI builds and
passes that binary explicitly rather than relying on a development `go run`.
This is release-blocking and exercises the offline system-disk replacement path
end to end.

The separate `recovery-api` gate exercises the production export path before
the ISO job: it creates a management user, managed SMB/NFS share, Compose
stack, encrypted secret payload, and configuration generation through the real
API handlers, exports the encrypted bundle, verifies its checksums, applies it
to a blank filesystem, and reopens the restored SQLite database. This catches
export omissions that a prebuilt recovery fixture cannot detect.
The recovery helper has a bounded startup timeout and powers the guest off on
failure; the host harness also applies a deadline so a broken restore fails
closed instead of hanging the release job.

The Debian appliance smoke test also requires `lumonas-privd`, all four typed
privileged workers, `lumonasd`, and `lumonas-web` to report active/running
through the services API, and requires the web service identity to be `lumonas`
rather than root.

The package-permissions job installs the generated `.deb` in a disposable
Debian 13 container and verifies the resulting ownership and modes. It proves
that administrator-owned environment files are readable by, but not writable
to, the `lumonas` service account, while the runtime, recovery, disk, and pool
directories remain writable by that account. This check is release-blocking.

The log-retention smoke test verifies that the packaged journald drop-in keeps
system and runtime logs bounded and expires old files, preventing appliance
logs from consuming the data volume without an explicit operator choice.
It also validates the packaged Docker `json-file` baseline with 10 MiB files
and three retained files.

The disk API contract test populates every stable identity field and verifies
that `/api/v1/disks` serializes those fields, including the current device
path, WWN/GPT/partition/filesystem identifiers, mount state, and last-seen
timestamp. OpenAPI and frontend types are kept aligned with that response.

The API contract checker also extracts every typed frontend API call from
`web/src/api/queries.ts`, normalizes template parameters, and requires a
matching documented/backend route. A frontend query cannot silently drift to
an undocumented endpoint.

Disk collection requests `PTUUID` from `lsblk` and uses the partition-table
UUID as a stable fallback between serial and filesystem UUID. The collector
contract test verifies both the command field and the resulting `gptDiskGuid`.

`check-api-contract.py` runs on every backend and installer-scripts job. It
checks the stable Disk and LumoEvent field sets in OpenAPI and TypeScript, so
route parity alone cannot hide a response-shape regression.

The SSE envelope test round-trips an event containing correlation, operation,
plan, actor, generation, resource, and payload data. This keeps the fields
needed to trace a destructive operation from the initiating request through
its persisted event and frontend delivery.

The daemon restart tests also verify that queued, preparing, and running jobs
are failed closed when `lumonasd` starts again, and that an SSE client can
resume from `Last-Event-ID` without receiving its cursor event twice. Host
integration commands use bounded contexts so a missing or wedged utility
cannot keep a job or release smoke test alive indefinitely.

## Destructive safety tests

Explicit cases:

- `/dev/sdb` becomes `/dev/sdc`;
- serial mismatch before format;
- filesystem UUID changes after plan;
- disk becomes mounted;
- pool dependency appears after plan;
- stale config generation;
- expired operation token;
- disk disconnect during SnapRAID job;
- simulated partial command failure.

The privileged safety contract also asserts that a missing target, an already
mounted target, and a target reported by `findmnt` all fail before any
destructive command is invoked.

The release safety job additionally executes the SnapRAID sync and scrub
failure contract, ensuring a non-zero command result cannot advance protection
state.

The network diagnostic contract verifies exact allow-listed arguments for ping
and traceroute, bounded output, context cancellation, and failure propagation.
Unsupported diagnostic kinds are rejected before any external command runs.

Storage plan tests also mutate GPT disk GUID and partition UUID independently
of the device path and require both single-disk and pool plans to fail closed.

Network and backup integration tests replace the WireGuard and SFTP command
runners and assert exact argv/stdin, proving these operations retain bounded
execution without invoking a shell.

The privileged safety suite also submits a confirmed filesystem mutation
without an operation ID and requires rejection before any command is run.

The broker logging test verifies that operation metadata is retained while
requested state and expected identity payloads are excluded from structured
logs.

The QEMU smoke fetches `/` through `lumonas-web` and checks the compiled title
and React root markers, so a package with a healthy API but missing frontend
assets fails the release gate.

The frontend job and Debian builder also run `frontend-runtime-smoke.sh`. It
requires the production title/root markers and rejects MSW bootstrap code in
the emitted JavaScript, while leaving the mock service-worker asset available
for the explicitly selected local demo mode.

The black-box API smoke restarts `lumonasd` against the same SQLite database,
verifies an interrupted SMART job is failed closed after startup, and resumes
an authenticated SSE stream from a retained metrics event to replay the job
state event. The restart probe is assertion-required in Ubuntu CI; macOS
development runs without Linux block-device discovery and reports that
disk-dependent probe as skipped.

The QEMU reorder assertion compares each disk's stable identity tuple (ID,
serial, WWN/UUID fields, and capacity) independently of `currentPath`, then
requires at least one transient device path to change after the virtual disk
order is rearranged.

Expected result: fail closed.

Onboarding also fails closed: an initial SnapRAID sync is not queued until
the privileged configuration and stable-disk validation both succeed.

## Recovery test

Release-blocking scenario:

1. Install old/current LumoNAS.
2. Configure realistic NAS.
3. Create test files.
4. Configure SnapRAID and Compose.
5. Create config + app backup.
6. Remove/delete system disk.
7. Attach blank disk.
8. Boot ISO offline.
9. Recover.
10. Verify checksums/config/services.

The API smoke is the source-state half of this scenario; the QEMU ISO smoke is
the offline replacement-disk half. Both are release-blocking and intentionally
kept as separate gates so a fixture cannot make an export regression invisible.

## Upgrade matrix

At minimum:

- previous stable → current;
- older supported release → current;
- backup generated by supported old schema → current restore.

Tagged CI downloads the previous release `.deb`, installs it in Debian 13,
adds an administrator-owned configuration marker, upgrades to the current
package, and verifies that the marker and runtime layout survive.
The upgrade-compatibility gate also opens a single legacy SQLite fixture that
contains the old users, jobs, events, audit, and network table layouts in one
database, then verifies that all records remain readable after the complete
migration chain. This catches ordering problems that isolated migration tests
can miss.

The Debian maintainer scripts stop services in dependent-to-provider order
(web, daemon, workers, broker) before an upgrade and start them in the reverse
dependency order afterward. A packaging smoke test checks both the script
ordering and that `prerm` is included in the generated `.deb`.

## Installer matrix

- UEFI;
- legacy BIOS if supported;
- network available;
- no network;
- data disks attached;
- existing LumoNAS disks attached;
- small/large system disk.

## Docker tests

- simple-mode edit;
- raw Compose custom nodes;
- simple edit after raw edit;
- comments/unknown keys preservation as targeted;
- unresolved variables;
- secrets;
- offline imported image;
- rollback after unhealthy deploy.

## Network rollback

Automate:

- apply unreachable static IP;
- verify checkpoint rollback;
- verify old UI address returns.

## Filesystem tests

Use loopback/virtual block devices to test:

- ext4/XFS;
- read-only import;
- format plan;
- mount;
- identity reorder.

CI runs `scripts/storage-loopback-smoke.sh` as a release-blocking root-gated
check. It uses only a temporary directory, verifies ext4 UUID stability after
loop-device reattachment, rejects writes after read-only import, rejects an
independent disk identity, and exercises XFS when the runner provides
`mkfs.xfs`. It also formats and erases a separate disposable loopback image,
proving the real filesystem lifecycle tools work without ever targeting
protected media.

## Fuzz/property tests

Good candidates:

- YAML transformations;
- API payload validation;
- path sanitization;
- backup manifests;
- destructive plan invariants.

The `security-controls` job is also release-blocking. It scans tracked files
for high-confidence private-key and token formats, then runs the diagnostics
redaction and privileged-operation rejection tests with a clean checkout.
Support bundle redaction also covers structured recovery-key fields, raw PEM
private-key blocks, and entries whose filenames identify recovery keys or
private keys.

The `dependency-controls` job is release-blocking as well. It runs the pinned
Go vulnerability scanner and the production frontend dependency audit, then
uses the pinned Trivy container to scan the repository dependency manifests and
every application image declared in `catalog/apps.json`. Unfixed HIGH and
CRITICAL findings fail the job. The scanner is required in CI; local execution
may skip it when Docker or the scanner tool is unavailable.

Packaging validation also runs `systemd-analyze verify` against every packaged
service unit. The check is optional for macOS/local development, but CI fails
if `systemd-analyze` is unavailable or any unit is invalid.
The management daemon also requires every typed privileged worker, so a
partially started broker cannot present a falsely mutation-capable appliance.

Every Debian package embeds `usr/share/lumonas/build-manifest.json` with the
source commit, Go toolchain, frontend lockfile hash, catalog hash, and exact
`Depends`/`Recommends` values. `verify-deb.sh` validates the manifest against
the package control metadata.

HTTP requests receive a generated `X-Request-ID` and carry the same
correlation ID in context. API-created jobs persist it, and privileged calls
inherit it; daemon-created jobs use their stable job ID as the fallback
correlation key. Events and audit rows persist first-class correlation,
operation, plan-hash, actor, resource, and generation fields while preserving
the original metadata payload for compatibility.

Host integration commands use a shared bounded runner. Privileged commands,
disk/SMART discovery, Docker, Samba validation, NetworkManager/WireGuard,
Tailscale, NUT, systemd status, mergerfs/SnapRAID discovery, and SFTP backup
transfers inherit a finite deadline; NetworkManager checkpoints additionally
remain bounded by their requested confirmation timeout.
The privileged Unix-socket client applies the same fail-closed deadline to
connected request/response streams.

The protection-config API test uses a generated configuration and stable disk
identities, so the endpoint’s read-only discovery path is exercised separately
from destructive operation tests.

The release-blocking loopback job requires `mergerfs` and `snapraid` and
exercises their real mount, status, sync, and scrub commands against
disposable filesystems, including a missing-configuration failure check; it
does not silently fall back to mocks when either integration is unavailable.

## Release artifacts

CI should produce:

- signed packages;
- ISO;
- hashes/signature;
- SBOM;
- release notes.

Tagged release verification must cryptographically verify each artifact’s
Cosign bundle, including the GitHub Actions OIDC issuer and tag workflow
identity. A signature file existing on disk is not sufficient.

Dependency and container-image vulnerability scans are required before tagged
release publication, in addition to the generated SBOM and signatures.

## Reproducibility

Pin external package inputs per release as much as practical.

Record exact package set in release manifest.

## Release channels

Possible:

- Stable;
- Beta;
- Nightly/Developer.

Stable should receive only fully passed installer/recovery matrix.

## Telemetry

Do not require cloud telemetry.

Optional anonymous crash/diagnostic submission can be considered later with explicit consent.

## Acceptance criterion

A build with failed system-disk recovery, secret-leak checks, or destructive
safety tests cannot be promoted to Stable.
