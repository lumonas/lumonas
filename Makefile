SHELL := /bin/sh

VERSION ?= 0.1.0-dev
GO_ENV := GOCACHE=$${GOCACHE:-/tmp/lumonas-go-build} GOPATH=$${GOPATH:-/tmp/lumonas-gopath}

.PHONY: all test test-go test-web check-openapi build build-go build-web package api-smoke storage-loopback qemu-image qemu-smoke verify-release

all: build

test: test-go test-web check-openapi

test-go:
	$(GO_ENV) go test ./...

check-openapi:
	python3 scripts/check-openapi.py

test-web:
	cd web && pnpm lint && pnpm typecheck && pnpm test -- --run && pnpm build

build: build-go build-web

build-go:
	mkdir -p bin
	$(GO_ENV) go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/lumonasd ./cmd/lumonasd
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-web ./cmd/lumonas-web
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-privd ./cmd/lumonas-privd

build-web:
	cd web && pnpm build

package:
	bash packaging/build-deb.sh $(VERSION)
	bash scripts/verify-deb.sh lumonas_$(VERSION)_amd64.deb

api-smoke:
	bash scripts/api-smoke.sh

storage-loopback:
	bash scripts/storage-loopback-smoke.sh

qemu-image:
	sudo LUMONAS_DEB="$(CURDIR)/lumonas_$(VERSION)_amd64.deb" LUMONAS_QEMU_IMAGE="$(CURDIR)/build/qemu/lumonas-debian13.raw" bash scripts/qemu-build-image.sh

qemu-smoke:
	LUMONAS_QEMU_IMAGE="$(CURDIR)/build/qemu/lumonas-debian13.raw" LUMONAS_QEMU_ASSERT=true bash scripts/qemu-smoke.sh

verify-release:
	bash scripts/verify-release.sh build/releases
