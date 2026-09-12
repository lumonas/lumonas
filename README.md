# LumoNAS runtime

The repository now contains the first appliance-runtime vertical slice alongside the React UI:

- `mynasd` — SQLite-backed API, jobs, event stream, and read-only Linux collectors;
- `mynas-privd` — Unix-socket allow-list broker that rejects unknown operations;
- `mynas-web` — static frontend server with `/api` reverse proxy;
- `packaging/` — systemd units and Debian package builder;
- `.github/workflows/ci.yml` — Go, frontend, package, QEMU smoke-test, and ISO jobs.

## Local development

Run the backend with a temporary database:

```sh
env MYNAS_DB_PATH=/tmp/lumonas.db go run ./cmd/mynasd
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

For an appliance-style authenticated runtime, provide an environment file to `mynasd`:

```sh
MYNAS_AUTH_REQUIRED=true
MYNAS_ADMIN_PASSWORD='use-a-long-unique-password'
MYNAS_RECOVERY_KEY='store-this-independent-recovery-key-safely'
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
sudo MYNAS_DEB="$PWD/lumonas_0.1.0-dev_amd64.deb" bash installer/build-iso.sh 0.1.0-dev
```

Storage mutations now use immutable plans, stable disk identity revalidation, explicit safety unlock/reauthentication, and the typed `mynas-privd` broker. Mount, unmount, format, and erase workers are allow-listed; SnapRAID jobs, NetworkManager checkpoints, and explicit power actions are also brokered with bounded confirmation. ACL mutation, scheduled power policy, and automatic recovery execution remain intentionally separate follow-up workers.

Read-only host integrations include `lsblk`/SMART disk identity, mergerfs mount discovery, SnapRAID configuration inspection, NUT UPS telemetry (`MYNAS_UPS_NAMES` or `upsc -l`), systemd status, and Docker Compose inspection. Notifications can be tested through the authenticated `/api/v1/notifications/test` endpoint after setting a webhook or ntfy URL.

Pool capacity is sampled once per UTC day into SQLite, retained for 180 days, and exposed through the read-only `/api/v1/capacity/forecast` endpoint. A forecast is withheld until at least three samples span a full day.

Set both `MYNAS_WEB_TLS_CERT` and `MYNAS_WEB_TLS_KEY` in `/etc/mynas/mynas-web.env` to serve the web listener over local HTTPS. The package deliberately does not generate a certificate; the operator or installer must provision a local CA/certificate and protect its private key.
