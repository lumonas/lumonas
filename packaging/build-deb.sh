#!/bin/sh
set -eu

VERSION="${1:-0.1.0-dev}"
DEB_ARCH="${LUMONAS_DEB_ARCH:-amd64}"
case "$DEB_ARCH" in
	amd64) DEFAULT_GOARCH=amd64 ;;
	arm64) DEFAULT_GOARCH=arm64 ;;
	*) echo "unsupported Debian package architecture: $DEB_ARCH" >&2; exit 1 ;;
esac
GOARCH="${LUMONAS_GOARCH:-$DEFAULT_GOARCH}"
case "$GOARCH" in
	amd64|arm64) ;;
	*) echo "unsupported Go package architecture: $GOARCH" >&2; exit 1 ;;
esac
CGO_ENABLED_VALUE="${LUMONAS_CGO_ENABLED:-1}"
case "$CGO_ENABLED_VALUE" in
	0|1) ;;
	*) echo "LUMONAS_CGO_ENABLED must be 0 or 1" >&2; exit 1 ;;
esac
BUILD_CC="${LUMONAS_CC:-}"
if [ "$CGO_ENABLED_VALUE" = "1" ] && [ "$GOARCH" != "$(go env GOARCH)" ] && [ -z "$BUILD_CC" ]; then
	case "$GOARCH" in
		arm64) BUILD_CC=aarch64-linux-gnu-gcc ;;
		amd64) BUILD_CC=x86_64-linux-gnu-gcc ;;
	esac
fi
if [ -n "$BUILD_CC" ]; then
	command -v "$BUILD_CC" >/dev/null 2>&1 || {
		echo "C compiler $BUILD_CC is required for GOARCH=$GOARCH (set LUMONAS_CC to override)" >&2
		exit 1
	}
fi
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUT="$ROOT/build/package"
GIT_COMMIT="${LUMONAS_GIT_COMMIT:-$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || printf '%s' unknown)}"
GO_VERSION="$(go version)"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || printf '%s' 0)}"
case "$SOURCE_DATE_EPOCH" in
	''|*[!0-9]*) echo "SOURCE_DATE_EPOCH must be a non-negative integer" >&2; exit 1 ;;
esac
export SOURCE_DATE_EPOCH
LOCK_SHA256="$(sha256sum "$ROOT/web/pnpm-lock.yaml" | awk '{print $1}')"
CATALOG_SHA256="$(sha256sum "$ROOT/catalog/apps.json" | awk '{print $1}')"
DEB_DEPENDS="$(awk -F': ' '/^Depends:/{print $2; exit}' "$ROOT/packaging/debian/control")"
DEB_RECOMMENDS="$(awk -F': ' '/^Recommends:/{print $2; exit}' "$ROOT/packaging/debian/control")"
rm -rf "$OUT"
mkdir -p "$OUT/DEBIAN" "$OUT/usr/lib/lumonas" "$OUT/usr/share/lumonas/web" "$OUT/usr/share/lumonas/catalog" "$OUT/lib/systemd/system" "$OUT/etc/lumonas" "$OUT/etc/docker" "$OUT/etc/systemd/journald.conf.d"
mkdir -p "$OUT/lib/systemd/system/smbd.service.d" "$OUT/lib/systemd/system/rsync.service.d" "$OUT/lib/systemd/system/vsftpd.service.d" "$OUT/lib/systemd/system/docker.service.d" "$OUT/lib/systemd/system/nfs-server.service.d" "$OUT/lib/systemd/system/ssh.service.d" "$OUT/lib/systemd/system/avahi-daemon.service.d"

build_binary() {
	output=$1
	package=$2
	if [ -n "$BUILD_CC" ]; then
		GOOS="${LUMONAS_GOOS:-linux}" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED_VALUE" CC="$BUILD_CC" GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$output" "$package"
	else
		GOOS="${LUMONAS_GOOS:-linux}" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED_VALUE" GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$output" "$package"
	fi
}
build_binary "$OUT/usr/lib/lumonas/lumonasd" "$ROOT/cmd/lumonasd"
build_binary "$OUT/usr/lib/lumonas/lumonas-web" "$ROOT/cmd/lumonas-web"
build_binary "$OUT/usr/lib/lumonas/lumonas-privd" "$ROOT/cmd/lumonas-privd"
build_binary "$OUT/usr/lib/lumonas/lumonas-recover" "$ROOT/cmd/lumonas-recover"
build_binary "$OUT/usr/lib/lumonas/lumonas-migrate" "$ROOT/cmd/lumonas-migrate"
(cd "$ROOT/web" && pnpm build)
bash "$ROOT/scripts/frontend-runtime-smoke.sh"
cp -R "$ROOT/web/dist/." "$OUT/usr/share/lumonas/web/"
cp "$ROOT/packaging/systemd/"*.service "$OUT/lib/systemd/system/"
cp "$ROOT/packaging/systemd/"*.target "$OUT/lib/systemd/system/"
cp "$ROOT/packaging/systemd/smbd.service.d/lumonas.conf" "$OUT/lib/systemd/system/smbd.service.d/"
cp "$ROOT/packaging/systemd/rsync.service.d/lumonas.conf" "$OUT/lib/systemd/system/rsync.service.d/"
cp "$ROOT/packaging/systemd/vsftpd.service.d/lumonas.conf" "$OUT/lib/systemd/system/vsftpd.service.d/"
cp "$ROOT/packaging/systemd/docker.service.d/lumonas.conf" "$OUT/lib/systemd/system/docker.service.d/"
cp "$ROOT/packaging/systemd/nfs-server.service.d/lumonas.conf" "$OUT/lib/systemd/system/nfs-server.service.d/"
cp "$ROOT/packaging/systemd/ssh.service.d/lumonas.conf" "$OUT/lib/systemd/system/ssh.service.d/"
cp "$ROOT/packaging/systemd/avahi-daemon.service.d/lumonas.conf" "$OUT/lib/systemd/system/avahi-daemon.service.d/"
cp "$ROOT/packaging/systemd/journald-lumonas.conf" "$OUT/etc/systemd/journald.conf.d/lumonas.conf"
cp "$ROOT/packaging/docker-daemon.json" "$OUT/etc/docker/daemon.json.lumonas"
cp "$ROOT/packaging/debian/lumonasd.env.example" "$OUT/etc/lumonas/lumonasd.env.example"
cp "$ROOT/packaging/debian/lumonas-web.env.example" "$OUT/etc/lumonas/lumonas-web.env.example"
cp "$ROOT/catalog/apps.json" "$OUT/usr/share/lumonas/catalog/apps.json"
if [ -f "$ROOT/catalog/apps.json.sig" ]; then
	cp "$ROOT/catalog/apps.json.sig" "$OUT/usr/share/lumonas/catalog/apps.json.sig"
fi
install -m 0750 "$ROOT/packaging/scripts/install-disk.sh" "$OUT/usr/share/lumonas/install-disk"
sed -e "s/^Version:.*/Version: $VERSION/" -e "s/^Architecture:.*/Architecture: $DEB_ARCH/" "$ROOT/packaging/debian/control" > "$OUT/DEBIAN/control"
printf '%s\n' \
	'{' \
	'  "schemaVersion": 1,' \
	'  "package": "lumonas",' \
	"  \"version\": \"$VERSION\", " \
	"  \"architecture\": \"$DEB_ARCH\", " \
	"  \"sourceDateEpoch\": $SOURCE_DATE_EPOCH, " \
	"  \"sourceCommit\": \"$GIT_COMMIT\", " \
	"  \"goVersion\": \"$GO_VERSION\", " \
	"  \"frontendLockSHA256\": \"$LOCK_SHA256\", " \
	"  \"catalogSHA256\": \"$CATALOG_SHA256\", " \
	"  \"debianDepends\": \"$DEB_DEPENDS\", " \
	"  \"debianRecommends\": \"$DEB_RECOMMENDS\"" \
	'}' > "$OUT/usr/share/lumonas/build-manifest.json"
cp "$ROOT/packaging/debian/postinst" "$OUT/DEBIAN/postinst"
cp "$ROOT/packaging/debian/prerm" "$OUT/DEBIAN/prerm"
chmod 0755 "$OUT/DEBIAN/postinst" "$OUT/DEBIAN/prerm" "$OUT/usr/lib/lumonas/"*
# Normalize every package entry before dpkg-deb archives it. Together with
# SOURCE_DATE_EPOCH this makes repeated builds from the same source produce
# byte-identical artifacts instead of embedding checkout/build mtimes.
find "$OUT" -exec touch -h --date="@$SOURCE_DATE_EPOCH" {} +
dpkg-deb --root-owner-group --build "$OUT" "$ROOT/lumonas_${VERSION}_${DEB_ARCH}.deb"
echo "Created $ROOT/lumonas_${VERSION}_${DEB_ARCH}.deb"
