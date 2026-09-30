# Accepted catalog image vulnerabilities

The container image gate holds two different standards, on purpose.

**Everything LumoNAS builds must be clean.** The repository filesystem, the
Debian package, the ISO and every other artifact are scanned with
`--exit-code 1` against zero unfixed HIGH/CRITICAL findings. A regression there
fails immediately.

**Third-party catalog images are compared against this baseline.** LumoNAS
does not build the images it offers in the app catalog, so it cannot rebuild
them when a base-image package is patched. Requiring zero findings for them is
not achievable: for example `nextcloud` currently reports several hundred, all
in Debian base packages whose fixed versions are newer than the published
image. `--ignore-unfixed` does not help, because a fix *does* exist upstream,
just not in that image.

What the gate still guarantees is the property that matters: **no new known
vulnerability is introduced silently.** Every HIGH/CRITICAL finding in a
catalog image must appear in `catalog-image-baseline.json`. Anything new fails
the build and must be triaged before the baseline changes. An image that is
absent from the baseline entirely is held to the strict zero-finding standard,
so adding a new catalog entry cannot quietly admit findings.

## Catalog image availability

`scripts/catalog-image-availability.sh` verifies that every pinned image still
resolves upstream. A deleted tag or a removed repository otherwise breaks
silently, because scanning a missing image returns no findings rather than an
error, so a broken pin reads exactly like a clean image.

Eleven catalog entries pointed at images that no longer exist:

| Entry | Old pin | Outcome |
|---|---|---|
| radarr | `linuxserver/radarr:5.17` | repointed to `6.4.4` |
| sonarr | `linuxserver/sonarr:4.0` | repointed to `4.0.20` |
| tautulli | `linuxserver/tautulli:2.15` | repointed to `2.18.2` |
| bazarr | `linuxserver/bazarr:1.5` | repointed to `1.6.2` |
| uptime-kuma | `louislam/uptime-kuma:1.21` | repointed to `2.5.5` |
| watchtower | `containrrr/watchtower:1.7` | repointed to `1.7.1` |
| organizr | `linuxserver/organizr:2.1` | repointed to `f6d984d2-ls56` |
| homarr | `ghcr.io/homarr-labs/homarr:1.0` | repointed to `v1.59.3` |
| pihole | `pihole/pihole:2024.11.0` | repointed to `2026.09.0` |
| minio | `minio/minio:RELEASE.2025-02-28T09-55-16Z` | **removed**, repository no longer exists on Docker Hub |
| vaultwarden-backup | `tigattack/vaultwarden-backup:1.8` | **removed**, repository no longer exists |

MinIO and Vaultwarden Backup were removed rather than repointed because no
replacement image could be verified as existing. Re-adding either entry needs a
source that actually resolves.

## Pinning

Every catalog image is pinned to an immutable tag, and the availability gate
rejects `latest`, `stable`, `main`, `edge`, `nightly` and `develop`. A floating
tag is a problem in two ways: an install is not reproducible, and the image
behind the tag can change without the catalog changing, which silently
invalidates the accepted vulnerability baseline for that entry.

Use a version tag, or a build tag such as linuxserver's `f6d984d2-ls56` where
upstream publishes no semver. Upgrading an app then becomes a deliberate change
that shows up in review, which is also when the baseline should be refreshed.

## Changing the baseline

Prefer fixing the image over accepting the finding. The catalog pins have
repeatedly turned out to be years behind, and updating them has removed
hundreds of findings at a time, so re-run the scan before reaching for the
baseline.

1. Update the pin in `catalog/apps.json` and measure the result:

   ```sh
   docker run --rm aquasec/trivy:0.58.1 image --scanners vuln \
     --severity HIGH,CRITICAL --ignore-unfixed --no-progress --format json IMAGE:TAG
   ```

2. If a finding genuinely has to be accepted, add its identifier to the
   relevant image in `catalog-image-baseline.json` in a commit that records
   why. Review the diff: this file is a record of accepted risk, not a
   scratchpad.

3. Re-sign the catalog, because editing `catalog/apps.json` invalidates the
   committed signature:

   ```sh
   # Actions -> Sign catalog -> Run workflow, then open the branch it pushes.
   ```

## Reproduce a failure on the same architecture as the gate

These images are multi-arch, and Trivy scans the variant that matches the host
it runs on. CI runs this gate on amd64, so a scan run on Apple Silicon reads
the **arm64** layers of the same tag and reports a different finding set for the
same image. The two CVEs that failed this gate for `jellyfin/jellyfin:12.1` are
in openssl, which is built per architecture: an arm64 scan reported 13
findings and looked clean, the amd64 scan reported 15 and named them.

Force the architecture when reproducing:

```sh
docker run --rm --platform linux/amd64 aquasec/trivy:0.58.1 image \
  --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed \
  --no-progress --format json IMAGE:TAG
```

A finding that will not reproduce locally is usually this, not a stale database.

## The 2026-09-30 openssl refresh

Seven identifiers were added across three images, and none were removed:

| Image | Added |
|---|---|
| `nextcloud:35.0.1` | CVE-2026-75804, CVE-2026-80864, CVE-2026-84782 |
| `ghcr.io/paperless-ngx/paperless-ngx:3.2.1` | CVE-2026-75804, CVE-2026-84782 |
| `netdata/netdata:v2.11.1` | CVE-2026-75804, CVE-2026-84782 |

All of them are advisories against packages the images already carry, published
after the entries were written. Two are openssl, fixed in `3.5.7-1~deb13u3`; the
images are on `u2`. The third is `linux-libc-dev`, fixed in `6.12.111-1`. None
can be removed by moving a pin, because no newer build of any of these three
images has picked up the fixed package yet. The same two openssl identifiers
were already accepted for `jellyfin/jellyfin:12.1` in the previous commit, and
they appear here on three more images for the same reason.

This entry exists because the "only ever grows" warning above is worth reading
against a concrete case: the diff is additive because the database learned
about these, not because nobody read the previous entries. Check the same way
when the next refresh comes.

## The 2026-10-01 kernel refresh

Four identifiers were added across two images, and none were removed:

| Image | Added | Already accepted for |
|---|---|---|
| `ghcr.io/home-assistant/home-assistant:2025.9.3` | CVE-2026-75804, CVE-2026-84782 | jellyfin, nextcloud, paperless, netdata |
| `nextcloud:35.0.1` | CVE-2026-97496, CVE-2026-97991 | nothing |

The home-assistant pair is the same two identifiers the previous entry
records, now on a fourth and fifth image carrying the same packages. That
they recur is the point of listing where else they are accepted.

The nextcloud pair is genuinely new to this project. Both are Linux kernel
advisories: `drm/amdkfd: Fix OOB memory exposure in get_wave_state()` and
`vdpa_sim_blk: reject out-of-range sector starts`, both fixed in
`6.12.111` / `6.18.53`. They are reported against the kernel version the
image was built against, not against a kernel LumoNAS ships — the host runs
its own, and the guest kernel is Debian's. Neither is reachable from inside
a container. `nextcloud:35.0.1` is also the newest tag Docker Hub lists for
that image, so there is no pin to move: the fixed kernel is not available
in any published build of it. This image already accepts a long tail of
sibling advisories against the same reported version (CVE-2026-93817,
CVE-2026-89846, CVE-2026-98039 and others).

Both entries stay until the underlying images are rebuilt on a newer base.
For nextcloud that is a matter of waiting for upstream; there is nothing to
choose here.

## Baseline drift is expected, not an incident

The scanner's vulnerability database is live. New advisories are published for
packages that are already in these images all the time, so **this gate going red
does not by itself mean a catalog change went wrong.** A finding appearing that
is not in `catalog-image-baseline.json` means the database learned about it
after the baseline was written, not that the catalog pulled something new.

That is the gate doing its job in the sense that matters: it is telling you the
accepted set no longer matches reality and needs a decision. The response is
usually to review and refresh, not to be alarmed.

What is *not* acceptable is refreshing without reading the diff. A refresh that
silently accepts a hundred new findings has quietly undone the control, so the
procedure below asks for the added and removed identifiers explicitly.

## Refreshing after drift

1. Reproduce the failure and see which findings are new:

   ```sh
   bash scripts/container-image-scan.sh
   ```

   It names the image and lists every finding missing from the baseline.

2. For each newly reported finding, decide whether to accept it or fix the
   underlying image. Fixing means updating the pin in `catalog/apps.json` and
   re-signing; accepting means recording it in the baseline with a reason.

3. Re-measure that one image and confirm the count moves in the right direction:

   ```sh
   docker run --rm aquasec/trivy:0.58.1 image --scanners vuln \
     --severity HIGH,CRITICAL --ignore-unfixed --no-progress --format json IMAGE:TAG
   ```

4. Update the entry in `catalog-image-baseline.json`. A refresh that only *adds*
   identifiers is suspicious: the database also retires findings, so an entry
   that only ever grows is a sign the set is being rubber-stamped rather than
   reviewed. Check that resolved findings come off the list.

5. Commit the baseline on its own, and state in the message which findings were
   added, which were removed, and why the new ones are acceptable. Re-signing is
   not needed unless `catalog/apps.json` also changed.

## Regenerating the file

`catalog-image-baseline.json` is generated, but commit it deliberately rather
than automatically: a regenerated file hides the fact that a pin changed and
its findings changed with it.
