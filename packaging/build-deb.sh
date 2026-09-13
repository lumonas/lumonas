#!/bin/sh
set -eu

VERSION="${1:-0.1.0-dev}"
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

GOOS="${LUMONAS_GOOS:-linux}" GOARCH="${LUMONAS_GOARCH:-amd64}" CGO_ENABLED="${LUMONAS_CGO_ENABLED:-1}" GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonasd" "$ROOT/cmd/lumonasd"
GOOS="${LUMONAS_GOOS:-linux}" GOARCH="${LUMONAS_GOARCH:-amd64}" CGO_ENABLED="${LUMONAS_CGO_ENABLED:-1}" GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonas-web" "$ROOT/cmd/lumonas-web"
GOOS="${LUMONAS_GOOS:-linux}" GOARCH="${LUMONAS_GOARCH:-amd64}" CGO_ENABLED="${LUMONAS_CGO_ENABLED:-1}" GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonas-privd" "$ROOT/cmd/lumonas-privd"
GOOS="${LUMONAS_GOOS:-linux}" GOARCH="${LUMONAS_GOARCH:-amd64}" CGO_ENABLED="${LUMONAS_CGO_ENABLED:-1}" GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonas-recover" "$ROOT/cmd/lumonas-recover"
GOOS="${LUMONAS_GOOS:-linux}" GOARCH="${LUMONAS_GOARCH:-amd64}" CGO_ENABLED="${LUMONAS_CGO_ENABLED:-1}" GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonas-migrate" "$ROOT/cmd/lumonas-migrate"
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
install -m 0750 "$ROOT/packaging/scripts/install-disk.sh" "$OUT/usr/share/lumonas/install-disk"
sed "s/^Version:.*/Version: $VERSION/" "$ROOT/packaging/debian/control" > "$OUT/DEBIAN/control"
printf '%s\n' \
	'{' \
	'  "schemaVersion": 1,' \
	'  "package": "lumonas",' \
	"  \"version\": \"$VERSION\", " \
	'  "architecture": "amd64",' \
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
dpkg-deb --root-owner-group --build "$OUT" "$ROOT/lumonas_${VERSION}_amd64.deb"
echo "Created $ROOT/lumonas_${VERSION}_amd64.deb"
