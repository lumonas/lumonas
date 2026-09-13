# LumoNAS runtime

The repository now contains the first appliance-runtime vertical slice alongside the React UI:

- `lumonasd` — SQLite-backed API, jobs, event stream, and read-only Linux collectors;
- `lumonas-privd` — Unix-socket allow-list broker that rejects unknown operations;
- `lumonas-web` — static frontend server with `/api` reverse proxy;
- `packaging/` — systemd units and Debian package builder;
- `.github/workflows/ci.yml` — Go, frontend, package, QEMU smoke-test, and ISO jobs.

## Local development

The quickest way to work on the UI — the frontend is backed by the Mock Service
Worker (no backend needed), with login `admin` + any 4+ character password:

```sh
make dev            # or: cd web && pnpm dev
```

To run everything for real — `lumonasd`, the `lumonas-privd` broker plus its
storage/network/power/general workers (custom socket dir, no root needed), and
the frontend proxying to the live API — use:

```sh
make dev-full       # or: scripts/dev.sh full
```

State (SQLite db, stacks, recovery dir, worker sockets) and logs live under
`build/dev/`; the frontend runs in the foreground and Ctrl-C stops the stack.
Note that privileged worker operations shell out to Linux tooling (samba,
snapraid, nft, …), so most mutating ops fail on a macOS host — the API and UI
flows still work. Set `LUMONAS_AUTH_REQUIRED=true` for an authenticated session
(`admin` / `dev-password-123` by default).

### Dev command reference

| Command | What it starts | URL |
|---|---|---|
| `make dev` | Vite dev server + MSW mock backend | http://localhost:5173 |
| `make dev-full` | `lumonasd` + privd broker + 4 workers + Vite (real API) | http://localhost:5173 (API: 8080) |
| `pnpm dev:api` | Vite only, proxies to an already-running backend | http://localhost:5173 |
| `make api-smoke` | Black-box backend smoke test (self-contained) | — |
| `make test` | Go tests + web lint/typecheck/tests + contract checks | — |

The dev launcher reads these overrides (all optional):

| Variable | Default | Purpose |
|---|---|---|
| `LUMONASD_LISTEN` | `127.0.0.1:8080` | Backend listen address (must stay `8080` for the Vite proxy) |
| `LUMONAS_DEV_DIR` | `build/dev` | State, binaries, worker sockets, and logs root |
| `LUMONAS_AUTH_REQUIRED` | `false` | Require login in full mode |
| `LUMONAS_ADMIN_PASSWORD` | `dev-password-123` | Admin password when auth is required |

If port 5173 or 8080 is already taken (e.g. a stray `lumonasd` from an earlier
session), stop that process first — `lsof -nP -iTCP:8080 -sTCP:LISTEN` finds it.

Manual setup, piece by piece:

Run the backend with a temporary database:

```sh
env LUMONAS_DB_PATH=/tmp/lumonas.db go run ./cmd/lumonasd
```

In another terminal, run the frontend against the real API. Vite proxies
`/api`, `/healthz`, and `/readyz` to the backend:

```sh
cd web
pnpm dev:api
```

`pnpm dev` uses the mock service worker by default (see
`web/.env.development`); `VITE_USE_MOCKS` is baked in at build time, so
production bundles always talk to the real backend.

For an appliance-style authenticated runtime, provide an environment file to `lumonasd`:

```sh
LUMONAS_AUTH_REQUIRED=true
LUMONAS_ADMIN_PASSWORD='use-a-long-unique-password'
LUMONAS_RECOVERY_KEY='store-this-independent-recovery-key-safely'
```

## Verification

```sh
make test
make build
```

Debian package creation requires `dpkg-deb` and is intended for Debian/Ubuntu CI:

```sh
make package
```

After the Debian package and QEMU smoke test are reliable, the offline installer can be built on Debian/Ubuntu with `live-build`:

```sh
make iso VERSION=0.1.0-dev
```

The `iso` target builds the matching Debian package first and passes that exact
artifact to the ISO builder. Use `LUMONAS_ISO_WORKDIR` to retain or relocate
the live-build work directory when debugging an image build.

Storage mutations now use immutable plans, planner-time state validation, stable disk identity revalidation, explicit safety unlock/reauthentication, and the typed `lumonas-privd` broker. Mount, unmount, format, and erase workers are allow-listed; SnapRAID jobs, NetworkManager checkpoints, ACL changes, scheduled backup/power policy, and explicit power actions are brokered with bounded confirmation. Automatic recovery remains an explicit staged operation.

The CI storage gate also exercises disposable loopback media: ext4 UUID stability, read-only import, mismatch rejection, format/erase behavior, and optional XFS import are verified before release artifacts can be promoted.

Frontend API queries are checked against the backend route contract and every
named response type is required to have a matching OpenAPI component schema.

Release CI also starts the real root-owned storage worker against a disposable loop device and verifies typed format, read-only mount, stale-plan, expiry, operation-ID, unmount, and erase behavior. No production device is used by this test.

Event, audit, capacity, and job history retention are bounded so long-running appliances do not grow SQLite state without limit.

Tagged releases run a Debian 13 previous-to-current package upgrade smoke test and verify that administrator configuration survives the upgrade.

The ISO pipeline boots the generated offline image under QEMU with a blank replacement disk and checks the real health, readiness, frontend document, and server endpoints before publishing the artifact. Release CI also performs a full offline system-disk recovery: it supplies a separate recovery medium, installs a bootable Debian runtime onto the blank disk, restores state, powers the guest off, and boots the recovered disk without the ISO to verify its real API.

The recovery gate uses a real migrated SQLite fixture containing users, groups, a share, protocol ACLs, and a committed configuration generation. It starts `lumonasd` against the recovered database before shutdown and verifies those records through the real principals/shares API, alongside Docker Compose, mergerfs, SnapRAID, stable disk identities, encrypted secrets, and the recovery result.

Release CI includes a security gate that checks tracked files for high-confidence credential formats and runs secret-redaction plus privileged-operation rejection tests.

Release CI also runs pinned Go/frontend dependency checks and scans the catalog
container images with Trivy; HIGH and CRITICAL unfixed findings block a tagged
release.

Packaged systemd units are verified with `systemd-analyze` in CI; local runs
skip that check when systemd tooling is unavailable.

QEMU smoke tests also verify all three LumoNAS services are running and that
the web service is owned by the unprivileged `lumonas` user.

Each Debian package includes a verified build manifest with source, toolchain,
lockfile, catalog, and dependency metadata.

Release verification also hashes SBOM, signature, and Cosign bundle sidecars;
tampering with release metadata fails verification, and publication rejects
sidecars that are not structured SPDX documents.

Generated ISO and QEMU appliances also include versioned Debian package
inventories with the source commit, reproducible epoch, and exact installed
versions.

Tagged publication is blocked by the package, QEMU, ISO, recovery, storage
safety, security, dependency, race/fuzz, schema-compatibility, and upgrade
gates.

CI also fills a disposable ext4 filesystem and verifies the production
collector reports a critical nearly-full state before release publication.

Host integration commands are executed through bounded contexts, including
privileged storage/network operations, disk and SMART discovery, Docker,
Samba validation, WireGuard, Tailscale, and NUT.

The API emits structured request logs with status, response size, duration, and
correlation IDs while keeping query strings and request bodies out of logs.

Offline recovery includes the plan-first `lumonas-recover` utility. Restoration requires explicit `--apply` plus an absolute target root and writes verified configuration, Compose state, the SQLite database, and encrypted secrets atomically.

Management sessions can be reviewed and revoked by token digest, while Time Machine shares render Samba fruit support only when explicitly enabled.

SMB and Time Machine share activation also updates the managed Avahi service announcement through the privileged broker; arbitrary Avahi content is rejected.

Read-only host integrations include `lsblk`/SMART disk identity, mergerfs mount discovery, SnapRAID configuration inspection, NUT UPS telemetry (`LUMONAS_UPS_NAMES` or `upsc -l`), systemd status, Docker Compose inspection, and `ethtool` Wake-on-LAN capability discovery. WOL changes use a typed privileged operation. Notifications can be tested through the authenticated `/api/v1/notifications/test` endpoint after setting a webhook or ntfy URL.

Pool capacity is sampled once per UTC day into SQLite, retained for 180 days, and exposed through the read-only `/api/v1/capacity/forecast` endpoint. A forecast is withheld until at least three samples span a full day.

Set both `LUMONAS_WEB_TLS_CERT` and `LUMONAS_WEB_TLS_KEY` in `/etc/lumonas/lumonas-web.env` to serve the web listener over local HTTPS. The package provisions an opt-in self-signed certificate at `/etc/lumonas/tls/` when OpenSSL is available; replace it with a certificate issued by your local CA for trusted clients and protect the private key.

## Security Features

LumoNAS includes several security features for production deployments:

- **HTTPS by default**: Self-signed certificate generated on first install
- **CSRF protection**: Tokens generated on login, required for all mutations
- **Rate limiting**: 5 login attempts per 5-minute window per IP
- **CORS configuration**: Configurable via `LUMONAS_CORS_ORIGINS` environment variable
- **Security headers**: X-Frame-Options, CSP, HSTS, Referrer-Policy
- **Request size limits**: 32 MB JSON, 2 GB multipart uploads
