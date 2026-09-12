# Offline installer

`build-iso.sh` builds an amd64 Debian 13 live installer with the LumoNAS `.deb` and a local APT repository embedded in the image. The image includes the core NAS packages and enables the LumoNAS systemd services during the live-build hook.

Build prerequisites are Debian/Ubuntu `live-build`, `dpkg-deb`, and the package produced by `packaging/build-deb.sh`:

```sh
make package
bash installer/build-iso.sh 0.1.0-dev
```

The image build uses Debian package mirrors while constructing the ISO, but the resulting media carries the LumoNAS package repository and can install the appliance package without Internet access. CI publishes the ISO with checksums, SBOM, and release signatures. With `LUMONAS_ENABLE_RECOVERY_SMOKE=true`, the image also boots an offline recovery fixture against a blank replacement disk, starts `lumonasd` against the restored filesystem, verifies the restored SQLite state through the real principals/shares API, and then checks users, shares, ACLs, Compose stack, mergerfs/SnapRAID configuration, encrypted secret payload, and recovery result before powering off.

The recovery fixture is deliberately generated from the production SQLite migrations and store APIs, rather than a fake database header. Run `make recovery-fixture` to build it locally, or let `make qemu-recovery-smoke` build it automatically.
