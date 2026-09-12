# LumoNAS runtime

The repository now contains the first appliance-runtime vertical slice alongside the React UI:

- `lumonasd` — SQLite-backed API, jobs, event stream, and read-only Linux collectors;
- `lumonas-privd` — Unix-socket allow-list broker that rejects unknown operations;
- `lumonas-web` — static frontend server with `/api` reverse proxy;
- `packaging/` — systemd units and Debian package builder;
- `.github/workflows/ci.yml` — Go, frontend, package, QEMU smoke-test, and ISO jobs.

## Local development

Run the backend with a temporary database:

```sh
env LUMONAS_DB_PATH=/tmp/lumonas.db go run ./cmd/lumonasd
```

In another terminal, run the frontend. Vite proxies `/api`, `/healthz`, and `/readyz` to the backend:

```sh
cd web
pnpm dev
```

The mock service worker is opt-in:

```sh
VITE_USE_MOCKS=true pnpm dev
```

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
sudo LUMONAS_DEB="$PWD/lumonas_0.1.0-dev_amd64.deb" bash installer/build-iso.sh 0.1.0-dev
```

Storage mutations now use immutable plans, stable disk identity revalidation, explicit safety unlock/reauthentication, and the typed `lumonas-privd` broker. Mount, unmount, format, and erase workers are allow-listed; SnapRAID jobs, NetworkManager checkpoints, ACL changes, scheduled backup/power policy, and explicit power actions are brokered with bounded confirmation. Automatic recovery remains an explicit staged operation.

The CI storage gate also exercises disposable loopback media: ext4 UUID stability, read-only import, mismatch rejection, and optional XFS import are verified before release artifacts can be promoted.

Event, audit, capacity, and job history retention are bounded so long-running appliances do not grow SQLite state without limit.

Tagged releases run a Debian 13 previous-to-current package upgrade smoke test and verify that administrator configuration survives the upgrade.

The ISO pipeline boots the generated offline image under QEMU with a blank replacement disk and checks the real health, readiness, and server endpoints before publishing the artifact. Release CI also performs a full offline system-disk recovery: it supplies a separate recovery medium, restores into the blank disk, powers the guest off, and verifies the recovered state directly from the image.

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

Host integration commands are executed through bounded contexts, including
privileged storage/network operations, disk and SMART discovery, Docker,
Samba validation, WireGuard, Tailscale, and NUT.

Offline recovery includes the plan-first `lumonas-recover` utility. Restoration requires explicit `--apply` plus an absolute target root and writes verified configuration, Compose state, the SQLite database, and encrypted secrets atomically.

Management sessions can be reviewed and revoked by token digest, while Time Machine shares render Samba fruit support only when explicitly enabled.

SMB and Time Machine share activation also updates the managed Avahi service announcement through the privileged broker; arbitrary Avahi content is rejected.

Read-only host integrations include `lsblk`/SMART disk identity, mergerfs mount discovery, SnapRAID configuration inspection, NUT UPS telemetry (`LUMONAS_UPS_NAMES` or `upsc -l`), systemd status, and Docker Compose inspection. Notifications can be tested through the authenticated `/api/v1/notifications/test` endpoint after setting a webhook or ntfy URL.

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
