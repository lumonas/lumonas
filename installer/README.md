# Offline installer

`build-iso.sh` builds an amd64 Debian 13 live installer with the LumoNAS `.deb` embedded in the image. The image includes the core NAS packages and enables the three LumoNAS systemd services during the live-build hook.

Build prerequisites are Debian/Ubuntu `live-build`, `dpkg-deb`, and the package produced by `packaging/build-deb.sh`:

```sh
make package
bash installer/build-iso.sh 0.1.0-dev
```

The initial image build uses Debian package mirrors while constructing the ISO. The resulting image contains the LumoNAS package and core package list for offline installation; a pinned local APT repository and recovery boot menu are subsequent release-hardening steps.
