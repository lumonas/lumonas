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

The daemon also prunes operational SQLite history at startup and every fifteen
minutes. Notification deliveries, completed backup runs, expired storage
plans, and completed network checkpoints are bounded, while active backup
sessions, firing/acknowledged alerts, pending rollback checkpoints, and
unexpired plans are retained; expired sessions and resolved alert history are
removed. Committed configuration-generation history is bounded while pending
generations remain available for recovery diagnosis. The
release security-controls job runs `scripts/retention-smoke.sh` so an
unbounded operational table cannot silently ship.
That scheduled pass also rechecks events, audit rows, terminal jobs, and
capacity snapshots, covering rows created by migrations, recovery tooling, or
other administrative paths that bypass normal write-time pruning.
The dedicated generation-retention smoke test is also release-blocking.
The package post-install policy is release-blocking as well: live systemd
hosts fail the transaction when an enabled LumoNAS unit cannot start, while
chroot/image builds retain their explicit no-PID-1 compatibility path.
The matching `prerm` path fails a live upgrade if any ordered dependent or
privileged provider cannot stop, preventing mixed old/new service binaries;
the no-PID-1 fallback remains limited to image and chroot construction.

### Integration

The frontend also has a browser-level smoke suite under `web/e2e/`. It runs
against the production Vite application in explicit demo mode and covers first
boot onboarding, the authenticated shell, and the Files, Docker, and Storage
navigation paths. CI installs a pinned Chromium runtime and runs this suite as
the `frontend-e2e` job; tagged publication depends on that job as well as the
real-backend runtime smoke.

The companion `frontend-live-e2e` suite runs the production bundle against
`scripts/dev.sh full`, with MSW disabled. It completes onboarding against the
real daemon, queues a maintenance job, waits for its `job.state_changed` event
through the browser's native `EventSource`, and navigates the live Overview,
Storage, and Monitoring pages. This catches a frontend/API/SSE integration
regression that API-only smoke tests cannot detect and is required by the
tagged release job.

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
Generated NFS, SFTP, FTP/FTPS, and rsync files are also parsed by
protocol-specific validators before atomic activation; malformed or unsafe
paths fail closed without changing the active configuration. The dedicated
validator smoke runs in the release-blocking share integration job.
The FTPS renderer is also checked against the certificate paths provisioned by
the Debian package, so an enabled FTPS share cannot point at stale TLS names.
The dedicated share-integration CI gate runs these production-path tests
independently and is required by the tagged release job.

Firewall activation is fail-closed: the daemon writes a generated ruleset only
as a staged file and requires the typed privileged worker to validate and apply
it. A missing broker or failed `nft` validation restores the previous ruleset
and fails the request; the release-blocking safety job covers this behavior.
Share configuration activation follows the same rule: validated files are
staged first, but a share mutation is not successful until the typed broker
has applied every required service and Avahi change. A missing broker rolls
back the generated files and stored share state; release CI covers this
fail-closed path.
File-identity provisioning and ACL changes are also operation-scoped at the
privileged boundary. System-user, Samba-user, and ACL mutations carry a
non-empty operation ID before the worker can execute them; the safety gate
checks rejection of unscoped requests.
The same release-blocking check covers NetworkManager checkpoints, Wi-Fi
connection changes, Wake-on-LAN, service reloads, and power actions so no
mutating worker path can be invoked without an operation scope.

The real-daemon API smoke starts the broker and all four typed privileged
workers in an isolated temporary socket directory before starting `lumonasd`.
This keeps `/readyz` meaningful in CI and exercises the same Unix-socket
boundary used by the packaged appliance rather than replacing it with an
in-process test double.

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
API over HTTPS before the ISO artifact is considered valid. The recovery ISO
smoke uses the same HTTPS web path while keeping its internal recovery daemon
on a separate loopback-only HTTP port.

The installer gate also runs the post-install runtime contract against that
freshly installed disk: server, disks, metrics, jobs, component health,
service ownership, the compiled frontend shell, and the authenticated SSE
stream are validated after login. This catches an installation that boots but
ships an incomplete or misconfigured runtime.

The same typed service-status validator is used by the live ISO, appliance,
recovery-source, and recovered-disk smokes. These paths therefore validate the
complete `/api/v1/services` response shape, not only a few expected unit names.

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

The QEMU and ISO smoke jobs upload their appliance logs with `if: always()`, so
a failed boot remains diagnosable. These diagnostic uploads do not satisfy the
release artifact gates; publication still requires the corresponding jobs to
pass.

The separate `recovery-api` gate exercises the production export path before
the ISO job: it creates a management user, network connection, managed SMB/NFS
share, Compose stack, encrypted secret payload, and configuration generation
through the real API handlers, exports the encrypted bundle, verifies its
checksums, applies it to a blank filesystem, and reopens the restored SQLite
database. It also requires explicit network, firewall, binding, and mount
metadata in the bundle. This catches export omissions that a prebuilt
recovery fixture cannot detect.
The recovery helper has a bounded startup timeout and powers the guest off on
failure; the host harness also applies a deadline so a broken restore fails
closed instead of hanging the release job.

The Debian appliance smoke test also requires `lumonas-privd`, all four typed
privileged workers, `lumonasd`, and `lumonas-web` to report active/running
through the services API, and requires the web service identity to be `lumonas`
rather than root. It also fetches the live runtime settings contract and
requires `lumonas-runtime.service` to be active, failing if the appliance does
not expose both the runtime and tmpfs state through a live systemd unit.
The image builder validates the runtime provisioning unit with the same
in-guest `systemd-analyze verify` pass as the API services.

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
The companion log-identity smoke test requires explicit journald output and a
stable `SyslogIdentifier` for the web service, daemon, broker, each typed
privileged worker, and runtime provisioner. This keeps structured operational
records searchable after an appliance reboot or upgrade.

The disk API contract test populates every stable identity field and verifies
that `/api/v1/disks` serializes those fields, including the current device
path, WWN/GPT/partition/filesystem identifiers, mount state, and last-seen
timestamp. OpenAPI and frontend types are kept aligned with that response.

The API contract checker also extracts every typed frontend API call from
`web/src/api/queries.ts`, normalizes template parameters, and requires a
matching documented/backend route. It also requires every named frontend
response type to exist in both the shared TypeScript declarations and
`docs/openapi.yaml`; inline response objects remain local to their query. A
frontend query cannot silently drift to an undocumented endpoint or schema.

Disk collection requests `PTUUID` from `lsblk` and uses the partition-table
UUID as a stable fallback between serial and filesystem UUID. The collector
contract test verifies both the command field and the resulting `gptDiskGuid`.

When `lsblk` leaves identity fields empty, the read-only collector enriches the
record from `udevadm info --query=property --name <device>`. The disk identity
smoke covers WWN, serial, model, filesystem UUID, partition-table UUID, bus,
and preservation of authoritative `lsblk` values. It also requests the lsblk
device tree and promotes mounted partition filesystem UUID/type metadata to the
physical-disk record, so a mounted `/dev/sda1` cannot be mistaken for an
unmounted `/dev/sda` during a safety check.

`check-api-contract.py` runs on every backend and installer-scripts job. It
checks the stable Disk and LumoEvent field sets in OpenAPI and TypeScript,
requires all 56 named frontend response types to have OpenAPI components, and
checks route parity so a response-shape or endpoint regression cannot hide.

The contract gate also rejects duplicate OpenAPI path or method keys. The
normalizer in `scripts/normalize-openapi.py` merges legacy split path blocks
before review so strict YAML parsers cannot silently discard an operation.

The black-box API smoke test additionally validates live JSON from the server,
disk, metrics, and jobs endpoints, plus the JSON envelope of streamed SSE
events. This catches runtime serialization regressions that static route and
type checks cannot see.

It performs a Unix-socket capability preflight: the normal local target reports
an explicit skip when the host sandbox forbids AF_UNIX binds, while CI and
`make api-smoke-strict` fail closed instead of weakening the privileged-worker
assertion.

Linux broker tests also exercise the kernel peer-credential boundary directly:
the accepted Unix peer must be root or the configured `lumonas` service group,
and a peer presented with a different configured group is rejected. Socket
filesystem mode remains a first gate, while `SO_PEERCRED` is the runtime
identity check used by `lumonas-privd` for defense in depth.

The Debian 13 QEMU smoke reuses those same validators against the appliance's
live responses and event stream, so release gating checks runtime shape and
reachability together rather than relying on string probes alone.

Local recovery uploads use a temporary mode-0600 file, sync the file before
promotion, atomically rename it into place, and sync the containing directory.
Downloads are synced before close as well, so a successful local backup copy
has an explicit durability boundary in addition to checksum verification.

The A/B update manager applies the same boundary to staged packages, manifests,
and slot state: package bytes are synced before close, atomic renames are
followed by a parent-directory sync, and activation is reported only after the
durable state write succeeds.

The SSE envelope test round-trips an event containing correlation, operation,
plan, actor, generation, resource, and payload data. This keeps the fields
needed to trace a destructive operation from the initiating request through
its persisted event and frontend delivery.

The daemon restart tests also verify that queued, preparing, and running jobs
are failed closed when `lumonasd` starts again, and that an SSE client can
resume from `Last-Event-ID` without receiving its cursor event twice, while
preserving correlation, operation, and plan tracing fields on replayed events.
Host integration commands use bounded contexts and process groups so a missing or
wedged utility, including descendants that inherit its pipes, cannot keep a
privileged request, job, or release smoke test alive indefinitely. The
interactive NetworkManager checkpoint path is covered separately because it
keeps a confirmation pipe open while the checkpoint is pending.

Job admission tests also verify resource-level serialization: SnapRAID sync and
scrub share one protection lock, while SMART operations lock by stable disk
identity. Conflicting work is rejected before persistence, closing the race
between concurrent API requests and scheduled maintenance.

The network checkpoint safety test also forces persistence of the pending
checkpoint record to fail and requires the daemon to invoke the typed rollback
operation before returning an error. A checkpoint is never considered safely
created unless both the privileged state and its durable rollback record exist.
The corresponding commit/rollback test also fails the completion write and
requires the API to report that the privileged action completed with
persistence incomplete.
The storage confirmation safety test applies the same rule after a destructive
operation: if the executed plan cannot be persisted, the API reports the
operation as completed but state persistence incomplete and emits no success
event.
WireGuard mutation is covered by the privileged-worker gate as well: the API
must send a typed, operation-scoped request to the network worker, and the
private key must cross the final command boundary on stdin only.
Tailscale connect, disconnect, and exit-node changes use the same worker
boundary, operation IDs, expiries, authorization, and audit path; their
command runners are injected in tests so missing binaries cannot turn into a
false-positive mutation.
The Tailscale connect test also verifies that an auth key is staged in a
mode-0600 temporary file and is never included in command arguments. The file
must be gone when the bounded command returns.
Power actions and scheduled/UPS shutdowns use the same daemon broker seam, so
the API cannot silently bypass the typed privileged boundary in production or
tests.
All other `lumonasd` privileged calls use that same execution helper as well,
including storage mounts/pools, SnapRAID, ACL jobs, runtime provisioning, and
live runtime status.
The framed privileged client also rejects oversized JSON requests before
writing to the Unix socket; broker and worker scanners enforce the same limit
on inbound frames.
The release race/fuzz gate is centralized in `scripts/race-fuzz-smoke.sh`,
which is used by both CI and the local Make target so the package set and fuzz
harness cannot diverge. It runs the race detector across every Go package
before the bounded fuzz suite, including collectors, event delivery, stores,
and all three runtime services.
Debian packaging also normalizes package-entry mtimes and builds the artifact
twice with the same source provenance. CI and `make package` fail if the two
`.deb` files differ byte-for-byte.

The command-boundary policy smoke scans production Go code for raw command
construction. Ordinary integrations must use the shared bounded runner; the
only permitted interactive exception is the allow-listed NetworkManager
checkpoint, which is required to use a process group and a finite confirmation
deadline. The same check rejects shell entrypoints in service code.

The shared runner also caps captured stdout and stderr at 1 MiB and fails the
operation closed when a utility exceeds that limit. Runner tests exercise both
stdout-only and combined-output paths so command diagnostics cannot become an
unbounded memory sink.

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
assets fails the release gate. Because the Debian package enables local HTTPS
by default, the QEMU image installs `openssl` before `postinst` runs and the
smoke uses `curl -k` only for the generated self-signed certificate. The web
boundary applies the browser security headers to both static and proxied
responses and overwrites the forwarded scheme before reaching the loopback
daemon, so HTTPS behavior is covered outside the API process as well.
`openssl` is a hard Debian dependency, so `--no-install-recommends` installs
cannot leave the active TLS configuration without its certificate generator.
Maintainer-script certificate failures are fatal before service startup, so a
package transaction cannot leave a partially configured HTTPS appliance.

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

It also waits for a persisted system-metrics sample, validates the timestamped
history response, and requires that sample to remain available after the
daemon restart. The Debian QEMU smoke repeats the history check after rebooting
with reordered data disks.

The same runtime checks validate `/health/components`; Docker Engine
availability is reported as an explicit attention component instead of being
silently represented as zero containers. The dashboard and Docker page surface
the same state so an unavailable Engine cannot be mistaken for an empty
inventory.

The QEMU reorder assertion compares each disk's stable identity tuple (ID,
serial, WWN/UUID fields, and capacity) independently of `currentPath`, then
requires at least one transient device path to change after the virtual disk
order is rearranged.

The Docker integration gate uses a deterministic Engine API double to verify
container, image, volume, and multiplexed log-frame decoding. This protects
the production Unix-socket path from regressing to human-oriented CLI output
parsing while keeping Compose command tests independently injectable.
The API, QEMU, and recovered-disk smoke tests validate the summary plus the
container, image, and volume inventory response shapes. The QEMU and
recovered-disk smoke tests also require the `/docker/summary` contract to
report `available: true`, so an empty zero-valued response cannot hide a
missing Docker daemon. Container inventory enrichment uses read-only Engine
inspect and one-shot stats requests for restart count, start time, memory, and
CPU percentage; image inventory also correlates container image IDs and
references to mark `inUse` conservatively. These mappings are covered by the
Docker integration gate. Volume usage is collected from the Engine's bounded
`/system/df` response and fails soft when the daemon does not support it.

The QEMU smoke exercises both documented SSE routes (`/api/v1/events/stream`
and the compatibility alias `/api/v1/events`) and validates each captured
envelope. The blank-disk ISO and recovered-disk boots additionally assert that
the daemon, unprivileged web service, and privileged broker service are active.

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

Release CI now feeds the generated Debian appliance image into the recovery
harness as the live source. The harness formats and mounts disposable virtual
disks, creates a real mergerfs pool, persists a LAN profile, creates a
management user, share, Compose stack, and appdata, exports and persists the
bundle, then boots the ISO against a blank replacement disk. The fixture
remains a local fallback, while tagged CI asserts the live-source path.
The source shutdown request is release-blocking: a failed API request or a
guest that does not exit cleanly fails the harness instead of being
force-killed and treated as a valid recovery source.
The recovery VM exit status is authoritative as well; a non-zero QEMU exit
cannot be hidden by a successful file-restoration check.

## Upgrade matrix

At minimum:

- previous stable → current;
- older supported release → current;
- backup generated by supported old schema → current restore.

Tagged CI downloads the previous release `.deb`, installs it in Debian 13,
adds an administrator-owned configuration marker, upgrades to the current
package, and verifies that the marker and runtime layout survive.
For the first tagged release, the upgrade job records that no previous
baseline exists and passes; every later tagged release keeps the upgrade test
release-blocking.
The upgrade-compatibility gate also opens a single legacy SQLite fixture that
contains the old users, jobs, events, audit, and network table layouts in one
database, then verifies that all records remain readable after the complete
migration chain. This catches ordering problems that isolated migration tests
can miss.
The same gate verifies that the storage snapshot migration adds the scheduled
snapshot origin field without losing legacy rows. The safety-recovery gate
also fires an enabled snapshot schedule and checks that its privileged
operation and persisted origin are present before release.

The Debian maintainer scripts stop services in dependent-to-provider order
(web, daemon, workers, broker) before an upgrade and start them in the reverse
dependency order afterward. A packaging smoke test checks both the script
ordering and that `prerm` is included in the generated `.deb`.

Before that dependency graph is started, `postinst` runs the packaged
`lumonas-migrate` binary as the unprivileged `lumonas` user. It reads only the
validated `LUMONAS_DB_PATH` setting, applies the production SQLite migration
chain, and fails the package transaction if migration cannot complete. This
keeps schema upgrades explicit and makes a broken migration release-blocking.

The release artifact smoke test invokes the same relative `build/releases`
layout used by CI, creates Debian and disk-image fixtures, verifies that both
are present in `SHA256SUMS`, and runs the release verifier against those
checksums. This prevents a working-directory bug from producing a seemingly
successful release with missing checksum entries.

Tagged releases enable the strict release-set check. It requires exactly one
amd64 Debian package, at least one installer ISO, and at least one QEMU machine
image in addition to the checksum, SBOM, and signature checks. Pull requests
continue to use the lighter artifact smoke test because they do not publish a
release set.

The ISO job also requires the embedded APT repository to be signed on tagged
builds. It imports the release-only armored private key into an ephemeral
GnuPG home, selects it by the configured fingerprint, and checks for
`Release.gpg`, `InRelease`, and the exported keyring; unsigned `[trusted=yes]`
media is limited to non-release development builds. The installer hook also
refuses its direct `dpkg` fallback whenever the repository signature policy is
required, so tagged builds cannot silently install around failed APT
signature verification.

Package and ISO builders derive `SOURCE_DATE_EPOCH` from the source commit when
the caller does not provide it. The Debian build records that epoch in
`build-manifest.json`, while the ISO records it in its release manifest and
uses it for embedded APT metadata dates. This gives release verification a
stable provenance value and prevents wall-clock time from changing those
metadata files.

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
loop-device reattachment, runs the production `collector.Disks` read-only
against the disposable device, rejects writes after read-only import, rejects
an independent disk identity, and exercises XFS with the required `mkfs.xfs`.
It also formats and erases a separate disposable loopback image, proving the
real filesystem lifecycle tools work without ever targeting protected media.

The privileged storage loopback job additionally starts the actual
`lumonas-privd` storage worker as root and sends typed Unix-socket requests
against temporary loop devices. The primary fixture receives a GPT disk GUID
because an unpartitioned loop device has no stable hardware identity. A
separate unpartitioned fixture proves that a path-only identity is rejected
before formatting. The job verifies format, read-only mount, mounted disk
rejection, stale identity rejection, missing operation ID rejection,
expired-plan rejection, unmount, and erase. This job is release-blocking and
never uses a production device path.

## Fuzz/property tests

Good candidates:

- YAML transformations;
- API payload validation;
- path sanitization;
- generated share protocol configuration validation;
- network connection and storage plan validation;
- backup manifests;
- destructive plan invariants.

The `security-controls` job is also release-blocking. A dedicated scanner
checks tracked files for high-confidence private-key, cloud-token, package
token, and credential-URL formats; its smoke test proves both detection and
safe-fixture behavior. The job then runs the diagnostics redaction and
privileged-operation rejection tests with a clean checkout. It also runs the
Linux `SO_PEERCRED` peer-boundary tests; wrong-group Unix peers and non-Unix
connections must be rejected before request parsing.
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
The package dependency parity smoke test also compares the Debian control
metadata with the explicit QEMU and offline ISO install sets. This matters
because the QEMU image uses `--no-install-recommends`: a newly declared
integration must be added to both appliance builders or the release is
rejected before image construction.
The same rule covers boot-critical packages such as `systemd-resolved`; the
QEMU image enables its resolver unit and must carry the package explicitly so
the resolver symlink cannot point at a component omitted by `--no-install-recommends`.
The same release gate statically verifies the sandbox policy: all services
must use `NoNewPrivileges`, private temporary storage, protected home/system
paths, bounded resources, and non-shell entrypoints; the web/daemon services
must run as `lumonas`, while privileged workers are root-owned, Unix-socket
only, and capability-bounded. CI fails if this policy check is unavailable or
any unit regresses.
The management daemon also requires every typed privileged worker, so a
partially started broker cannot present a falsely mutation-capable appliance.

Every Debian package embeds `usr/share/lumonas/build-manifest.json` with the
source commit, Go toolchain, frontend lockfile hash, catalog hash, and exact
`Depends`/`Recommends` values. `verify-deb.sh` validates the manifest against
the package control metadata.

`release-artifacts.sh` also writes `RELEASE-MANIFEST.json`, listing every
package, ISO, and QEMU image with its SHA-256 digest and byte size, together
with the source commit and `SOURCE_DATE_EPOCH`. Strict tagged-release
verification requires this manifest, compares its source commit with
`github.sha` and its source epoch with that commit's timestamp. It also
rejects any artifact added or removed without regenerating the manifest.
The manifest also records SHA-256 digests and sizes for each generated SBOM,
signature, and Cosign bundle, while `SHA256SUMS` covers those sidecars and the
manifest after they are generated. Release verification therefore fails if
provenance or verification metadata is tampered with independently of the
main artifact.
Artifact names are also constrained to direct, non-symlink files in the
release directory, so a tampered manifest cannot redirect verification to a
path outside the published artifact set.

The generated ISO and QEMU images embed versioned Debian package inventories
under `/usr/share/doc/lumonas/`, including the source commit, source epoch, and
every installed package version, so an artifact can be audited after boot or
offline inspection.

Local artifact construction follows the same dependency chain as CI: `make
iso` builds the matching Debian package before invoking `live-build`, and
`make qemu-image` likewise depends on the package target. This prevents a
stale or differently versioned `.deb` from being embedded in a test appliance.

HTTP requests receive a generated `X-Request-ID` and carry the same
correlation ID in context. API-created jobs persist it and the authenticated
actor, and privileged calls inherit it; daemon-created jobs use their stable
job ID as the fallback correlation key and `system` as the actor. Events and
audit rows persist first-class correlation, operation, plan-hash, actor,
resource, and generation fields. Request-originated mutation events are
checked at the envelope level so actor attribution does not leak into payload
data, while scheduled and daemon-originated events remain `system`.
The original metadata payload is preserved
the original metadata payload for compatibility.

Host integration commands use a shared bounded runner. Privileged commands,
disk/SMART discovery, Docker, Samba validation, NetworkManager/WireGuard,
Tailscale, NUT, systemd status, mergerfs/SnapRAID discovery, and SFTP backup
transfers inherit a finite deadline; NetworkManager checkpoints additionally
remain bounded by their requested confirmation timeout.
The privileged Unix-socket client applies the same fail-closed deadline to
connected request/response streams.
Privileged IPC frames are also bounded to one MiB on both the client response
reader and broker/worker scanners, with oversized response tests running in the
release-blocking Go safety suite.
Recovery export now fails closed when stable disk identity collection, NAS
identity, Docker stack discovery, bundle verification, or versioned-copy
persistence fails; the API cannot report a verified bundle for partial state.
Recovery ZIP parsing is bounded by entry count, entry-name length, per-entry
expanded bytes, and aggregate expanded bytes. Both bundle creation and
verification enforce these limits, and the release safety gate exercises the
malformed-entry and oversized-shape paths before recovery artifacts can ship.
Recovery export verifies the complete bundle before publication, stages both
latest and generation-addressed copies with mode 0600, fsyncs file and
directory metadata, and publishes them with collision-safe atomic renames.
Persistence tests cover repeated exports in the same timestamp and tampered
bundles that must not create a recovery directory.
Notification provider responses are bounded to 64 KiB before status handling,
and the security gate exercises the oversized-response rejection path.
Support bundles retain their downloadable archive even when a collector fails,
but record redacted collection-error status in `server.json` instead of making
missing disks, events, audit rows, or appliance identity appear healthy.
Configured notification credential failures now persist a sanitized failed
delivery record and enter the existing suppression counter; persistence errors
are structured warnings rather than silent drops.
The suppression counter retains first and second failures until the third
consecutive failure activates the bounded cooldown; it no longer resets the
window while the cooldown timestamp is unset.

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

The race/fuzz job runs the Go fuzz engine with a bounded duration for path,
backup-manifest, Compose, network, recovery, and storage validators. It is
not satisfied by merely selecting `Fuzz` functions as ordinary unit tests.

Request middleware applies a 32 MiB cap to JSON writes and a 2 GiB streaming
cap to multipart writes, rejecting an oversized declared body before routing.
Multipart parsing is left to the authenticated endpoint so unauthenticated
requests cannot force large temporary-file work.

The release job has an explicit gate-policy smoke test that checks its `needs`
set includes package, QEMU, ISO, recovery, storage safety, integration,
security, dependency, race/fuzz, schema-compatibility, and upgrade jobs. A
successful individual job cannot be bypassed by accidentally omitting it from
tagged publication.

The deployment-hardening gates also mount and fill a disposable ext4 image,
then run the production filesystem collector against it. The test requires a
critical nearly-full result and is included in tagged release dependencies, so
disk exhaustion handling cannot regress silently.

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
