#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
OUT_DIR="${1:-$ROOT/build/workstation-clients}"
VERSION="${LUMONAS_VERSION:-0.1.0-dev}"
mkdir -p "$OUT_DIR"

build_client() {
	goos=$1
	goarch=$2
	extension=$3
	name="lumonas-workstation_${VERSION}_${goos}_${goarch}${extension}"
	GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 GOCACHE="${GOCACHE:-/tmp/lumonas-go-build}" GOPATH="${GOPATH:-/tmp/lumonas-gopath}" go build -trimpath -ldflags "-s -w" -o "$OUT_DIR/$name" "$ROOT/cmd/lumonas-workstation"
}

build_client linux amd64 ""
build_client linux arm64 ""
build_client windows amd64 .exe
build_client darwin amd64 ""
build_client darwin arm64 ""
(cd "$OUT_DIR" && sha256sum lumonas-workstation_"$VERSION"_* > SHA256SUMS)
printf 'Built workstation clients in %s\n' "$OUT_DIR"
