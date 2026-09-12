SHELL := /bin/sh

VERSION ?= 0.1.0-dev
GO_ENV := GOCACHE=$${GOCACHE:-/tmp/lumonas-go-build} GOPATH=$${GOPATH:-/tmp/lumonas-gopath}

.PHONY: all dev dev-full test test-go test-web check-openapi check-api-contract build build-go build-web package recovery-fixture recovery-api-smoke api-smoke storage-loopback share-config-smoke iso-smoke qemu-recovery-smoke qemu-recovery-live security-smoke dependency-smoke container-scan systemd-smoke systemd-security-smoke permission-smoke log-retention-smoke upgrade-smoke qemu-image qemu-smoke verify-release

all: build

dev:
	bash scripts/dev.sh mock

dev-full:
	bash scripts/dev.sh full

test: test-go test-web check-openapi check-api-contract

test-go:
	$(GO_ENV) go test ./...

check-openapi:
	python3 scripts/check-openapi.py

check-api-contract:
	python3 scripts/check-api-contract.py

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

recovery-api-smoke:
	bash scripts/recovery-api-smoke.sh

api-smoke:
	bash scripts/api-smoke.sh

storage-loopback:
	bash scripts/storage-loopback-smoke.sh

share-config-smoke:
	bash scripts/share-config-smoke.sh

iso-smoke:
	LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_ISO_ASSERT=true bash scripts/iso-smoke.sh

qemu-recovery-smoke: recovery-fixture
	sudo LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_RECOVERY_FIXTURE="$(CURDIR)/build/lumonas-recovery-fixture" LUMONAS_RECOVERY_ASSERT=true bash scripts/qemu-recovery-smoke.sh

qemu-recovery-live:
	sudo LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_RECOVERY_SOURCE_IMAGE="$(LUMONAS_QEMU_IMAGE)" LUMONAS_RECOVERY_ASSERT=true bash scripts/qemu-recovery-smoke.sh

security-smoke:
	bash scripts/security-smoke.sh

dependency-smoke:
	bash scripts/dependency-smoke.sh

container-scan:
	bash scripts/container-image-scan.sh

systemd-smoke:
	bash scripts/systemd-smoke.sh

systemd-security-smoke:
	bash scripts/systemd-security-smoke.sh

permission-smoke:
	bash scripts/permission-smoke.sh "$(PACKAGE)"

log-retention-smoke:
	bash scripts/log-retention-smoke.sh

upgrade-smoke:
	bash scripts/upgrade-smoke.sh "$(OLD_DEB)" "$(NEW_DEB)"

qemu-image:
	sudo LUMONAS_DEB="$(CURDIR)/lumonas_$(VERSION)_amd64.deb" LUMONAS_QEMU_IMAGE="$(CURDIR)/build/qemu/lumonas-debian13.raw" bash scripts/qemu-build-image.sh

qemu-smoke:
	LUMONAS_QEMU_IMAGE="$(CURDIR)/build/qemu/lumonas-debian13.raw" LUMONAS_QEMU_ASSERT=true bash scripts/qemu-smoke.sh

verify-release:
	bash scripts/verify-release.sh build/releases
