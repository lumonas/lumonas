#!/bin/sh
set -eu

VERSION="${1:-0.1.0-dev}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUT="$ROOT/build/package"
rm -rf "$OUT"
mkdir -p "$OUT/DEBIAN" "$OUT/usr/lib/lumonas" "$OUT/usr/share/lumonas/web" "$OUT/usr/share/lumonas/catalog" "$OUT/lib/systemd/system" "$OUT/etc/mynas"

GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/mynasd" "$ROOT/cmd/mynasd"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/mynas-web" "$ROOT/cmd/mynas-web"
GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT/usr/lib/lumonas/mynas-privd" "$ROOT/cmd/mynas-privd"
(cd "$ROOT/web" && pnpm build)
cp -R "$ROOT/web/dist/." "$OUT/usr/share/lumonas/web/"
cp "$ROOT/packaging/systemd/"*.service "$OUT/lib/systemd/system/"
cp "$ROOT/packaging/debian/mynasd.env.example" "$OUT/etc/mynas/mynasd.env.example"
cp "$ROOT/packaging/debian/mynas-web.env.example" "$OUT/etc/mynas/mynas-web.env.example"
cp "$ROOT/catalog/apps.json" "$OUT/usr/share/lumonas/catalog/apps.json"
sed "s/^Version:.*/Version: $VERSION/" "$ROOT/packaging/debian/control" > "$OUT/DEBIAN/control"
cp "$ROOT/packaging/debian/postinst" "$OUT/DEBIAN/postinst"
chmod 0755 "$OUT/DEBIAN/postinst" "$OUT/usr/lib/lumonas/"*
dpkg-deb --root-owner-group --build "$OUT" "$ROOT/lumonas_${VERSION}_amd64.deb"
echo "Created $ROOT/lumonas_${VERSION}_amd64.deb"
