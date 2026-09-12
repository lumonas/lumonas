# Offline installer

`build-iso.sh` builds an amd64 Debian 13 live installer with the LumoNAS `.deb` and a local APT repository embedded in the image. The image includes the core NAS packages and enables the LumoNAS systemd services during the live-build hook.

Build prerequisites are Debian/Ubuntu `live-build`, `dpkg-deb`, and the package produced by `packaging/build-deb.sh`:

```sh
make package
bash installer/build-iso.sh 0.1.0-dev
```

The image build uses Debian package mirrors while constructing the ISO, but the resulting media carries the LumoNAS package repository and can install the appliance package without Internet access. Debian base package pinning, repository signing, and the recovery boot menu remain release-hardening work.
