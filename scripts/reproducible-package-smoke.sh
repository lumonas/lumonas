#!/bin/sh
set -eu

VERSION="${1:-0.1.0-dev}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct)}"
GIT_COMMIT="${LUMONAS_GIT_COMMIT:-$(git -C "$ROOT" rev-parse HEAD)}"
PACKAGE="$ROOT/lumonas_${VERSION}_amd64.deb"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-reproducible-package.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT INT TERM

build_once() {
	rm -f "$PACKAGE"
	SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" LUMONAS_GIT_COMMIT="$GIT_COMMIT" \
		bash "$ROOT/packaging/build-deb.sh" "$VERSION"
}

build_once
cp "$PACKAGE" "$WORK/first.deb"
build_once
cmp -s "$WORK/first.deb" "$PACKAGE" || {
	echo "repeated Debian package builds differ" >&2
	sha256sum "$WORK/first.deb" "$PACKAGE" >&2
	exit 1
}
bash "$ROOT/scripts/verify-deb.sh" "$PACKAGE"
echo "LumoNAS reproducible Debian package smoke passed: $PACKAGE"
