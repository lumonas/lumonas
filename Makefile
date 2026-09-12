SHELL := /bin/sh

VERSION ?= 0.1.0-dev
GO_ENV := GOCACHE=$${GOCACHE:-/tmp/lumonas-go-build} GOPATH=$${GOPATH:-/tmp/lumonas-gopath}

.PHONY: all test test-go test-web build build-go build-web package api-smoke qemu-image qemu-smoke

all: build

test: test-go test-web

test-go:
	$(GO_ENV) go test ./...

test-web:
	cd web && pnpm lint && pnpm typecheck && pnpm build

build: build-go build-web

build-go:
	mkdir -p bin
	$(GO_ENV) go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/mynasd ./cmd/mynasd
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/mynas-web ./cmd/mynas-web
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/mynas-privd ./cmd/mynas-privd

build-web:
	cd web && pnpm build

package:
	bash packaging/build-deb.sh $(VERSION)

api-smoke:
	bash scripts/api-smoke.sh

qemu-image:
	sudo MYNAS_DEB="$(CURDIR)/lumonas_$(VERSION)_amd64.deb" MYNAS_QEMU_IMAGE="$(CURDIR)/build/qemu/mynas-debian13.raw" bash scripts/qemu-build-image.sh

qemu-smoke:
	MYNAS_QEMU_IMAGE="$(CURDIR)/build/qemu/mynas-debian13.raw" MYNAS_QEMU_ASSERT=true bash scripts/qemu-smoke.sh
