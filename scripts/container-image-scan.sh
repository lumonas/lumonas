#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SCANNER_IMAGE="${LUMONAS_TRIVY_IMAGE:-aquasec/trivy:0.58.1}"
REQUIRE_SCANNER="${LUMONAS_CONTAINER_SCAN_REQUIRED:-false}"

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
	docker run --rm "$SCANNER_IMAGE" image \
		--scanners vuln \
		--severity HIGH,CRITICAL \
		--ignore-unfixed \
		--exit-code 1 \
		--no-progress \
		"$image"
done

echo "LumoNAS container image checks passed"
