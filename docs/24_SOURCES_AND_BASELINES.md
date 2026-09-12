# Technical Sources and Baselines

This file records the main external technologies assumed by the September 2026 plan. Always re-check versions before implementation/release.

## Debian

Baseline: Debian 13 Stable (Trixie). Debian announced 13.6 on 2026-07-11.

- https://www.debian.org/News/2026/20260711
- https://wiki.debian.org/DebianStable

## Docker

MyNAS should use Docker Engine API/Go SDK rather than parsing `docker ps` output for normal management.

- https://docs.docker.com/reference/api/engine/
- https://docs.docker.com/compose/

## NetworkManager

Use D-Bus API and checkpoint/rollback behavior for safe network changes.

- https://networkmanager.dev/docs/api/latest/
- https://networkmanager.dev/docs/api/latest/spec.html

## SnapRAID

The manual documents sync/scrub/content/parity behavior. MyNAS should keep SnapRAID terminology and safety semantics accurate.

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

Technology versions should be pinned per MyNAS release.

The product must not assume that “latest” package behavior remains unchanged; CI and release manifests are authoritative for each shipped image.
