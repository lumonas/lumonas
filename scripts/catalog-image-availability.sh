#!/bin/sh
# Verify every catalog image reference still resolves upstream.
#
# A catalog entry is a promise that the app can be installed. A tag that has
# been deleted, or a repository that has been removed, breaks that promise
# silently: nothing in the build notices, because scanning a missing image
# returns no findings rather than an error. This gate turns that into a hard
# failure.
#
# It is deliberately separate from the vulnerability scan so that a broken pin
# is reported as a broken pin.
#
# Registry rate limiting makes some lookups inconclusive, so only a definitive
# "not found" fails the build. A throttled lookup is reported and skipped
# rather than being mistaken for a broken pin.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
REQUIRE="${LUMONAS_CATALOG_IMAGES_REQUIRED:-false}"

if ! command -v docker >/dev/null 2>&1; then
	if [ "$REQUIRE" = "true" ]; then
		echo "Docker is required for the catalog image availability gate" >&2
		exit 1
	fi
	echo "Docker is not installed; catalog image availability check skipped" >&2
	exit 0
fi

# resolve_image echoes one of: found, missing, or unknown.
resolve_image() {
	image=$1
	if output=$(docker manifest inspect "$image" 2>&1 >/dev/null); then
		echo found
		return 0
	fi
	case "$output" in
		*MANIFEST_UNKNOWN* | *"manifest unknown"* | *"not found"* | *"404"* | *"does not exist"* | *"no such manifest"*)
			echo missing
			;;
		*toomanyrequests* | *"Too Many Requests"* | *"rate limit"* | *429*)
			echo unknown
			;;
		*)
			# Anything else, including an empty message, is not evidence that
			# the image is gone, so do not fail on it.
			echo unknown
			;;
	esac
}

failed=0
throttled=0
checked=0
floating=0
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
	checked=$((checked + 1))
	# A floating tag makes an install irreproducible and makes the accepted
	# vulnerability baseline unstable, because the image behind the tag changes
	# without the catalog changing.
	case "${image##*:}" in
		latest | stable | main | edge | nightly | develop | edge-dev)
			echo "floating tag: $image" >&2
			floating=$((floating + 1))
			;;
	esac
	case "$(resolve_image "$image")" in
		found) echo "resolves: $image" ;;
		missing)
			echo "unavailable: $image" >&2
			failed=$((failed + 1))
			;;
		*)
			echo "could not verify (registry throttled): $image" >&2
			throttled=$((throttled + 1))
			;;
	esac
done

if [ "$floating" -ne 0 ]; then
	echo "" >&2
	echo "$floating catalog image(s) use a floating tag." >&2
	echo "Pin an immutable version or digest so installs are reproducible and" >&2
	echo "the accepted vulnerability baseline stays meaningful." >&2
	exit 1
fi

if [ "$failed" -ne 0 ]; then
	echo "" >&2
	echo "$failed of $checked catalog image(s) no longer resolve upstream." >&2
	echo "Update the pin in catalog/apps.json, or remove the entry if the" >&2
	echo "upstream project is gone. Then re-sign the catalog with the" >&2
	echo "'Sign catalog' workflow." >&2
	exit 1
fi

if [ "$throttled" -ne 0 ]; then
	echo "NOTE: $throttled image(s) could not be verified because the registry" >&2
	echo "      throttled this runner. Treat them as unverified, not as clean." >&2
fi
echo "LumoNAS catalog image availability verified ($checked images, $throttled unverified)"
