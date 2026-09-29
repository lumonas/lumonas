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
	trap 'rm -f "$report"' EXIT INT TERM
	docker run --rm "$SCANNER_IMAGE" image \
		--scanners vuln \
		--severity HIGH,CRITICAL \
		--ignore-unfixed \
		--exit-code 0 \
		--no-progress \
		--format json \
		"$image" >"$report" 2>/dev/null || true

	# An image with no baseline entry is held to the strict zero-finding
	# standard; check-catalog-baseline.py enforces that by default.
	python3 "$ROOT/scripts/check-catalog-baseline.py" \
		--baseline "$BASELINE" --results "$report" --image "$image"
	rm -f "$report"
	trap - EXIT INT TERM
done

echo "LumoNAS container image checks passed"
