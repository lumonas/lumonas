#!/bin/sh
set -eu

VERSION="${1:-0.1.0-dev}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUT="$ROOT/build/package"
rm -rf "$OUT"
mkdir -p "$OUT/DEBIAN" "$OUT/usr/lib/lumonas" "$OUT/usr/share/lumonas/web" "$OUT/usr/share/lumonas/catalog" "$OUT/lib/systemd/system" "$OUT/etc/lumonas" "$OUT/etc/systemd/journald.conf.d"
mkdir -p "$OUT/lib/systemd/system/smbd.service.d" "$OUT/lib/systemd/system/rsync.service.d" "$OUT/lib/systemd/system/vsftpd.service.d"

GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonasd" "$ROOT/cmd/lumonasd"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonas-web" "$ROOT/cmd/lumonas-web"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonas-privd" "$ROOT/cmd/lumonas-privd"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/lumonas-recover" "$ROOT/cmd/lumonas-recover"
(cd "$ROOT/web" && pnpm build)
cp -R "$ROOT/web/dist/." "$OUT/usr/share/lumonas/web/"
cp "$ROOT/packaging/systemd/"*.service "$OUT/lib/systemd/system/"
cp "$ROOT/packaging/systemd/smbd.service.d/lumonas.conf" "$OUT/lib/systemd/system/smbd.service.d/"
cp "$ROOT/packaging/systemd/rsync.service.d/lumonas.conf" "$OUT/lib/systemd/system/rsync.service.d/"
cp "$ROOT/packaging/systemd/vsftpd.service.d/lumonas.conf" "$OUT/lib/systemd/system/vsftpd.service.d/"
cp "$ROOT/packaging/systemd/journald-lumonas.conf" "$OUT/etc/systemd/journald.conf.d/lumonas.conf"
cp "$ROOT/packaging/debian/lumonasd.env.example" "$OUT/etc/lumonas/lumonasd.env.example"
cp "$ROOT/packaging/debian/lumonas-web.env.example" "$OUT/etc/lumonas/lumonas-web.env.example"
cp "$ROOT/catalog/apps.json" "$OUT/usr/share/lumonas/catalog/apps.json"
sed "s/^Version:.*/Version: $VERSION/" "$ROOT/packaging/debian/control" > "$OUT/DEBIAN/control"
cp "$ROOT/packaging/debian/postinst" "$OUT/DEBIAN/postinst"
chmod 0755 "$OUT/DEBIAN/postinst" "$OUT/usr/lib/lumonas/"*
dpkg-deb --root-owner-group --build "$OUT" "$ROOT/lumonas_${VERSION}_amd64.deb"
echo "Created $ROOT/lumonas_${VERSION}_amd64.deb"
