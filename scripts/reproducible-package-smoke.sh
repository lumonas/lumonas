#!/bin/sh
set -eu

VERSION="${1:-0.1.0-dev}"
DEB_ARCH="${LUMONAS_DEB_ARCH:-amd64}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct)}"
GIT_COMMIT="${LUMONAS_GIT_COMMIT:-$(git -C "$ROOT" rev-parse HEAD)}"
# build-deb.sh normalises a version that does not start with a digit (CI passes
# the commit SHA), so it may name the artifact differently than requested. Ask it
# to report the name it actually wrote rather than assuming one.
VERSION_FILE="$(mktemp "${TMPDIR:-/tmp}/lumonas-deb-version.XXXXXX")"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-reproducible-package.XXXXXX")"
cleanup() { rm -rf "$WORK" "$VERSION_FILE"; }
trap cleanup EXIT INT TERM
PACKAGE=""

build_once() {
	rm -f "$ROOT"/lumonas_*_"${DEB_ARCH}".deb
	SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" LUMONAS_GIT_COMMIT="$GIT_COMMIT" \
		LUMONAS_DEB_VERSION_FILE="$VERSION_FILE" \
		bash "$ROOT/packaging/build-deb.sh" "$VERSION"
	# The build reports the artifact it created; a stale file must never be
	# mistaken for this build's output.
	PACKAGE="$(cat "$VERSION_FILE")"
	[ -n "$PACKAGE" ] && [ -f "$PACKAGE" ] || {
		echo "build-deb.sh did not report a readable package path" >&2
		exit 1
	}
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
