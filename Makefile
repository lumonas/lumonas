SHELL := /bin/sh

VERSION ?= 0.1.0-dev
GO_ENV := GOCACHE=$${GOCACHE:-/tmp/lumonas-go-build} GOPATH=$${GOPATH:-/tmp/lumonas-gopath}

.PHONY: all test test-go test-web check-openapi build build-go build-web package recovery-fixture api-smoke storage-loopback iso-smoke qemu-recovery-smoke security-smoke dependency-smoke container-scan systemd-smoke upgrade-smoke qemu-image qemu-smoke verify-release

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
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-recover ./cmd/lumonas-recover

build-web:
	cd web && pnpm build

package:
	bash packaging/build-deb.sh $(VERSION)
	bash scripts/verify-deb.sh lumonas_$(VERSION)_amd64.deb

recovery-fixture:
	mkdir -p build
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o build/lumonas-recovery-fixture ./cmd/lumonas-recovery-fixture

api-smoke:
	bash scripts/api-smoke.sh

storage-loopback:
	bash scripts/storage-loopback-smoke.sh

iso-smoke:
	LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_ISO_ASSERT=true bash scripts/iso-smoke.sh

qemu-recovery-smoke: recovery-fixture
	sudo LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_RECOVERY_FIXTURE="$(CURDIR)/build/lumonas-recovery-fixture" LUMONAS_RECOVERY_ASSERT=true bash scripts/qemu-recovery-smoke.sh

security-smoke:
	bash scripts/security-smoke.sh

dependency-smoke:
	bash scripts/dependency-smoke.sh

container-scan:
	bash scripts/container-image-scan.sh

systemd-smoke:
	bash scripts/systemd-smoke.sh

upgrade-smoke:
	bash scripts/upgrade-smoke.sh "$(OLD_DEB)" "$(NEW_DEB)"

qemu-image:
	sudo LUMONAS_DEB="$(CURDIR)/lumonas_$(VERSION)_amd64.deb" LUMONAS_QEMU_IMAGE="$(CURDIR)/build/qemu/lumonas-debian13.raw" bash scripts/qemu-build-image.sh

qemu-smoke:
	LUMONAS_QEMU_IMAGE="$(CURDIR)/build/qemu/lumonas-debian13.raw" LUMONAS_QEMU_ASSERT=true bash scripts/qemu-smoke.sh

verify-release:
	bash scripts/verify-release.sh build/releases
