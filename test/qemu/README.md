# QEMU appliance test fixture

Build a reproducible Debian 13 raw image on a Linux host with root privileges:

```sh
make package
sudo LUMONAS_DEB="$PWD/lumonas_0.1.0-dev_amd64.deb" \
  LUMONAS_QEMU_IMAGE="$PWD/build/qemu/lumonas-debian13.raw" \
  scripts/qemu-build-image.sh
```

The runtime smoke-test entrypoint is `scripts/qemu-smoke.sh`. It boots the image with one system disk, three virtual data disks, a parity disk, and a virtual NIC. Set `LUMONAS_QEMU_ASSERT=true` to wait for and assert health/readiness, core API endpoints, service discovery, stable disk identity, live SSE metrics, and recovery export/status/restore-plan verification through the guest web service.

The GitHub Actions QEMU job builds this image from a clean checkout, runs the asserted smoke test, and publishes the raw appliance image plus the smoke log for release assembly.
