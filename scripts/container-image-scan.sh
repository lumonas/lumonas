#!/bin/sh
# Scan repository and catalog container images.
#
# The repository filesystem and anything LumoNAS builds is held to zero
# unfixed HIGH/CRITICAL findings. Catalog entries are third-party images the
# project does not build, so they are compared against a reviewed baseline
# instead: a finding that is not listed fails the build and must be triaged.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SCANNER_IMAGE="${LUMONAS_TRIVY_IMAGE:-aquasec/trivy:0.58.1}"
REQUIRE_SCANNER="${LUMONAS_CONTAINER_SCAN_REQUIRED:-false}"
BASELINE="${LUMONAS_CATALOG_BASELINE:-$ROOT/security/catalog-image-baseline.json}"

if ! command -v docker >/dev/null 2>&1; then
	if [ "$REQUIRE_SCANNER" = "true" ]; then
		echo "Docker is required for the container image gate" >&2
		exit 1
	fi
	echo "Docker is not installed; container image scan skipped" >&2
	exit 0
fi

if ! docker image inspect "$SCANNER_IMAGE" >/dev/null 2>&1; then
	if [ "$REQUIRE_SCANNER" = "true" ]; then
		echo "Trivy scanner image is not available: $SCANNER_IMAGE" >&2
		exit 1
	fi
	echo "Trivy scanner image is not available; container image scan skipped" >&2
	exit 0
fi

# Everything LumoNAS produces must be clean.
docker run --rm \
	-v "$ROOT:/workspace:ro" \
	"$SCANNER_IMAGE" fs \
	--scanners vuln \
	--severity HIGH,CRITICAL \
	--ignore-unfixed \
	--exit-code 1 \
	--no-progress \
	--skip-dirs /workspace/.git \
	--skip-dirs /workspace/web/node_modules \
	--skip-dirs /workspace/web/dist \
	--skip-dirs /workspace/build \
	/workspace

# Catalog images are compared against the reviewed baseline, and an image
# missing from the baseline is compared against nothing, so it must be clean.
for image in $(python3 - "$ROOT/catalog/apps.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    catalog = json.load(handle)

for app in catalog:
    image = app.get("image")
    if image:
        print(image)
PY
); do
	[ -n "$image" ] || continue
	echo "Scanning container image: $image"
	report="$(mktemp "${TMPDIR:-/tmp}/lumonas-image-scan.XXXXXX")"
	errors="$(mktemp "${TMPDIR:-/tmp}/lumonas-image-scan-err.XXXXXX")"
	trap 'rm -f "$report" "$errors"' EXIT INT TERM
	# Fail closed. A scan that cannot pull the image, or that errors, must not
	# be read as a clean scan: a missing upstream tag otherwise looks exactly
	# like an image with no findings.
	#
	# Transient registry and database failures are retried, because a rate limit
	# is indistinguishable from a real scan failure and the fix for one is not
	# the fix for the other. --timeout 20m because a large image under a loaded
	# runner can otherwise die with "semaphore acquire: context deadline
	# exceeded", which is a timeout, not a finding.
	scan_ok=false
	for attempt in 1 2 3; do
		if docker run --rm "$SCANNER_IMAGE" image \
			--timeout 20m \
			--scanners vuln \
			--severity HIGH,CRITICAL \
			--ignore-unfixed \
			--exit-code 0 \
			--no-progress \
			--format json \
			"$image" >"$report" 2>"$errors" && [ -s "$report" ]; then
			scan_ok=true
			break
		fi
		echo "scan attempt $attempt for $image failed; retrying" >&2
		sleep $((attempt * 15))
	done
	if [ "$scan_ok" != true ]; then
		echo "scanning $image failed:" >&2
		sed -n '1,20p' "$errors" >&2
		echo "" >&2
		echo "This usually means the pinned tag no longer exists upstream." >&2
		echo "Update the pin in catalog/apps.json, then re-sign the catalog." >&2
		exit 1
	fi

	# An image with no baseline entry is held to the strict zero-finding
	# standard; check-catalog-baseline.py enforces that by default.
	#
	# Keep scanning after a mismatch instead of stopping at the first one. The
	# vulnerability database is live, so one advisory can appear against a dozen
	# images that all ship the same package version, and stopping at the first
	# turns a single refresh into a dozen CI runs that each report one image.
	# The gate still fails, and it now fails with the whole list.
	if ! python3 "$ROOT/scripts/check-catalog-baseline.py" \
		--baseline "$BASELINE" --results "$report" --image "$image"; then
		drift=1
	fi
	rm -f "$report" "$errors"
	trap - EXIT INT TERM
done

if [ "${drift:-0}" -ne 0 ]; then
	echo "" >&2
	echo "One or more images have findings outside the accepted baseline." >&2
	echo "Triage each one, then refresh security/catalog-image-baseline.json in a" >&2
	echo "reviewed commit. See security/README.md; prefer updating the image pin." >&2
	exit 1
fi

echo "LumoNAS container image checks passed"
