SHELL := /bin/sh

VERSION ?= 0.1.0-dev
GO_ENV := GOCACHE=$${GOCACHE:-/tmp/lumonas-go-build} GOPATH=$${GOPATH:-/tmp/lumonas-gopath}

.PHONY: all dev dev-full test test-go test-web frontend-e2e check-openapi check-api-contract validate-api-response validate-sse build build-go build-web package package-dependency-parity iso recovery-fixture recovery-api-smoke recovery-bundle-smoke recovery-persistence-smoke api-smoke disk-identity-smoke storage-loopback privileged-storage-loopback disk-full-smoke share-config-smoke share-protocol-smoke iso-smoke qemu-recovery-smoke qemu-recovery-live security-smoke secret-scan-smoke command-boundary-smoke request-limits-smoke retention-smoke generation-retention-smoke fuzz-smoke dependency-smoke container-scan race-fuzz upgrade-compatibility release-gate-policy systemd-smoke systemd-security-smoke permission-smoke postinst-policy-smoke log-retention-smoke log-identity-smoke upgrade-smoke release-artifacts-smoke installer-signature-smoke qemu-image qemu-smoke verify-release

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

validate-api-response:
	python3 scripts/validate-api-response.py --self-test

validate-sse:
	python3 scripts/validate-sse.py --self-test

test-web:
	cd web && pnpm lint && pnpm typecheck && pnpm test -- --run && pnpm build

frontend-e2e:
	cd web && pnpm test:e2e

build: build-go build-web

build-go:
	mkdir -p bin
	$(GO_ENV) go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/lumonasd ./cmd/lumonasd
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-web ./cmd/lumonas-web
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-privd ./cmd/lumonas-privd
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-recover ./cmd/lumonas-recover
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-migrate ./cmd/lumonas-migrate

build-web:
	cd web && pnpm build

package:
	bash scripts/reproducible-package-smoke.sh $(VERSION)

package-dependency-parity:
	bash scripts/package-dependency-parity-smoke.sh

iso: package
	LUMONAS_DEB="$(CURDIR)/lumonas_$(VERSION)_amd64.deb" bash installer/build-iso.sh "$(VERSION)"

recovery-fixture:
	mkdir -p build
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o build/lumonas-recovery-fixture ./cmd/lumonas-recovery-fixture

recovery-api-smoke:
	bash scripts/recovery-api-smoke.sh

recovery-bundle-smoke:
	bash scripts/recovery-bundle-smoke.sh

recovery-persistence-smoke:
	bash scripts/recovery-persistence-smoke.sh

api-smoke:
	bash scripts/api-smoke.sh

disk-identity-smoke:
	bash scripts/disk-identity-smoke.sh

storage-loopback:
	bash scripts/storage-loopback-smoke.sh

privileged-storage-loopback:
	sudo LUMONAS_PRIVILEGED_STORAGE_ASSERT=true bash scripts/privileged-storage-loopback-smoke.sh

disk-full-smoke:
	sudo LUMONAS_DISK_FULL_ASSERT=true bash scripts/disk-full-smoke.sh

share-config-smoke:
	bash scripts/share-config-smoke.sh

share-protocol-smoke:
	bash scripts/share-protocol-smoke.sh

iso-smoke:
	LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_ISO_ASSERT=true bash scripts/iso-smoke.sh

qemu-recovery-smoke: recovery-fixture
	sudo LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_RECOVERY_FIXTURE="$(CURDIR)/build/lumonas-recovery-fixture" LUMONAS_RECOVERY_ASSERT=true bash scripts/qemu-recovery-smoke.sh

qemu-recovery-live:
	sudo LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_RECOVERY_SOURCE_IMAGE="$(LUMONAS_QEMU_IMAGE)" LUMONAS_RECOVERY_ASSERT=true bash scripts/qemu-recovery-smoke.sh

security-smoke:
	bash scripts/security-smoke.sh

secret-scan-smoke:
	bash scripts/secret-scan-smoke.sh

command-boundary-smoke:
	bash scripts/command-boundary-smoke.sh

fuzz-smoke:
	bash scripts/fuzz-smoke.sh

request-limits-smoke:
	bash scripts/request-limits-smoke.sh

retention-smoke:
	bash scripts/retention-smoke.sh

generation-retention-smoke:
	bash scripts/generation-retention-smoke.sh

dependency-smoke:
	bash scripts/dependency-smoke.sh

container-scan:
	bash scripts/container-image-scan.sh

race-fuzz:
	GOCACHE=$${GOCACHE:-/tmp/lumonas-go-race-cache} GOPATH=$${GOPATH:-/tmp/lumonas-gopath} bash scripts/race-fuzz-smoke.sh

upgrade-compatibility:
	$(GO_ENV) go test ./internal/store -run 'TestStoreReopenPreservesStateAcrossMigrations|TestOpenMigratesLegacyEventSchema|TestOpenMigratesLegacyRuntimeSchemaAsOneUpgrade'

release-gate-policy:
	bash scripts/release-gate-policy-smoke.sh

systemd-smoke:
	bash scripts/systemd-smoke.sh

systemd-security-smoke:
	bash scripts/systemd-security-smoke.sh

permission-smoke:
	bash scripts/permission-smoke.sh "$(PACKAGE)"

postinst-policy-smoke:
	bash scripts/postinst-policy-smoke.sh

log-retention-smoke:
	bash scripts/log-retention-smoke.sh

log-identity-smoke:
	bash scripts/log-identity-smoke.sh

upgrade-smoke:
	bash scripts/upgrade-smoke.sh "$(OLD_DEB)" "$(NEW_DEB)"

release-artifacts-smoke:
	bash scripts/release-artifacts-smoke.sh

installer-signature-smoke:
	bash scripts/installer-signature-policy-smoke.sh

qemu-image: package
	sudo LUMONAS_DEB="$(CURDIR)/lumonas_$(VERSION)_amd64.deb" LUMONAS_QEMU_IMAGE="$(CURDIR)/build/qemu/lumonas-debian13.raw" bash scripts/qemu-build-image.sh

qemu-smoke:
	LUMONAS_QEMU_IMAGE="$(CURDIR)/build/qemu/lumonas-debian13.raw" LUMONAS_QEMU_ASSERT=true bash scripts/qemu-smoke.sh

verify-release:
	bash scripts/verify-release.sh build/releases
