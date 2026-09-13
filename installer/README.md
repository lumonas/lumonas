# Offline installer

`build-iso.sh` builds an amd64 Debian 13 live installer with the LumoNAS `.deb` and a local APT repository embedded in the image. The image includes the core NAS packages and enables the LumoNAS systemd services during the live-build hook.

Build prerequisites are Debian/Ubuntu `live-build`, `dpkg-deb`, and the package produced by `packaging/build-deb.sh`:

```sh
make iso VERSION=0.1.0-dev
```

The Make target builds the matching `.deb` before invoking the ISO builder, so
the package and media cannot accidentally come from different versions. The
underlying script remains available for CI and for explicit artifact paths via
`LUMONAS_DEB`.

Tagged CI builds set `LUMONAS_REQUIRE_REPO_SIGNATURE=true` and provide the
release-only `LUMONAS_REPO_SIGN_KEY` fingerprint and
`LUMONAS_REPO_PRIVATE_KEY` armored-key secrets. CI imports the private key
into an ephemeral GnuPG home before building. Such builds fail closed unless
the embedded repository contains `Release.gpg`, `InRelease`, and the exported
archive keyring. Local development builds may omit the key and use the
explicitly marked unsigned repository path; `LUMONAS_ENFORCE_SIGNING=true`
applies the tagged-CI strictness to local builds on demand, failing the build
when `LUMONAS_REPO_SIGN_KEY` is absent. During the live-build hook, a
required signature also makes APT metadata refresh and package installation
fail closed; direct `dpkg` fallback is available only for unsigned local
development images.

The image build uses Debian package mirrors while constructing the ISO, but the resulting media carries the LumoNAS package repository and can install the appliance package without Internet access. CI publishes the ISO with checksums, SBOM, and release signatures. With `LUMONAS_ENABLE_RECOVERY_SMOKE=true`, the image installs a bootable Debian runtime and bootloader onto a blank replacement disk, restores a verified recovery fixture, starts `lumonasd` against the restored filesystem, verifies the restored SQLite state through the real principals/shares API, and then boots the recovered disk without the ISO before the gate passes.

## Browser-based installation

The live medium runs `lumonasd` with `LUMONAS_INSTALLER_MODE=true`, which
enables the `/install/*` endpoints and the `/install` page:

1. `GET /install/targets` reviews every disk. The running system disk,
   mounted disks, parity members, disks carrying filesystems, undersized
   disks, and disks in critical health are listed with explicit protection
   reasons — they stay visible so the review step explains the exclusion.
2. `POST /install/plan` validates the operator's choices and produces an
   immutable, hash-pinned plan that expires after ten minutes.
3. `POST /install/apply` requires the exact plan hash, explicit
   confirmation, and a 12+ character administrator password. The plan hash
   and disk identity are revalidated at the privileged boundary; the broker
   invokes the packaged `install-disk` provisioner (partition, filesystem,
   Debian runtime copy, bootloader) with the password delivered over stdin and
   written only to the account-owned first-boot environment file. Apply is
   single-flight: while a disk is being provisioned, a second plan or apply
   request is rejected.
4. `GET /install/status` reports the current stage.

Outside installer mode all `/install/*` endpoints return 404, so a running
appliance can never offer to install over itself.

The release storage gate separately installs `e2fsprogs`, `xfsprogs`, mergerfs,
and SnapRAID and exercises disposable ext4/XFS branches, a real mergerfs pool,
and a read-only SnapRAID status probe before an appliance release is accepted.

The packaging gate also verifies every systemd unit with `systemd-analyze` and
checks the required sandbox policy, including unprivileged web/daemon users,
Unix-socket-only privileged workers, capability bounds, and finite resource
limits.

The recovery fixture is deliberately generated from the production SQLite migrations and store APIs, rather than a fake database header. Run `make recovery-fixture` to build it locally, or let `make qemu-recovery-smoke` build it automatically.

Run `make recovery-api-smoke` to exercise the production API export path with a
configured user, network connection, share, Compose stack, encrypted secret,
explicit network/firewall/mount metadata, and a restored SQLite database
before booting the ISO.

When a Debian appliance image is available, `make qemu-recovery-live
LUMONAS_ISO=... LUMONAS_QEMU_IMAGE=...` runs the stronger live-source recovery
path: it formats disposable data/parity disks, mounts them, creates a real
mergerfs pool, persists network state and appdata through the source appliance
API, exports the bundle, and restores it onto a blank replacement disk through
the offline ISO.
