# Technical Sources and Baselines

This file records the main external technologies assumed by the September 2026 plan. Always re-check versions before implementation/release.

## Debian

Baseline: Debian 13 Stable (Trixie). Debian announced 13.6 on 2026-07-11.

- https://www.debian.org/News/2026/20260711
- https://wiki.debian.org/DebianStable

## Docker

LumoNAS uses the Docker Engine API over the Unix socket for normal read-only
management and status collection rather than parsing `docker ps` output. The
socket is bounded by `LUMONAS_DOCKER_SOCKET` and defaults to
`/var/run/docker.sock`; Compose remains a separately validated command
boundary because the Engine API does not replace Compose project operations.
Container resource status uses the read-only `/containers/{id}/json` inspect
and `/containers/{id}/stats?stream=false` endpoints; failures in optional
enrichment do not hide the base inventory.
The appliance daemon does not join the broad `docker` group: the root-owned
`lumonas-privd` broker exposes only the typed Engine reads and Docker commands
needed by the management API.

- https://docs.docker.com/reference/api/engine/
- https://docs.docker.com/compose/

## NetworkManager

Use D-Bus API and checkpoint/rollback behavior for safe network changes.

- https://networkmanager.dev/docs/api/latest/
- https://networkmanager.dev/docs/api/latest/spec.html

## SnapRAID

The manual documents sync/scrub/content/parity behavior. LumoNAS should keep SnapRAID terminology and safety semantics accurate.

- https://www.snapraid.it/manual

## mergerfs

mergerfs combines paths and uses policies to choose branches for operations. Use opinionated defaults and keep advanced policies available.

- https://github.com/trapexit/mergerfs
- https://github.com/trapexit/mergerfs/blob/master/mkdocs/docs/quickstart.md

## mkosi

Candidate future image/A-B/reproducible image tooling. Supports Debian image creation.

- https://github.com/systemd/mkosi
- https://github.com/systemd/mkosi/blob/main/mkosi/resources/man/mkosi.1.md

## Notes

Technology versions should be pinned per LumoNAS release.

The product must not assume that “latest” package behavior remains unchanged; CI and release manifests are authoritative for each shipped image.
