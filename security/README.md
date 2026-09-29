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

## Regenerating the file

`catalog-image-baseline.json` is generated, but commit it deliberately rather
than automatically: a regenerated file hides the fact that a pin changed and
its findings changed with it.
