SHELL := /bin/sh

VERSION ?= 0.1.0-dev
GO_ENV := GOCACHE=$${GOCACHE:-/tmp/lumonas-go-build} GOPATH=$${GOPATH:-/tmp/lumonas-gopath}
LUMONAS_ISO ?= $(CURDIR)/build/releases/lumonas-$(VERSION)-amd64.iso
LUMONAS_QEMU_IMAGE ?= $(CURDIR)/build/qemu/lumonas-debian13.raw

.PHONY: all dev dev-full test test-go test-web frontend-e2e frontend-live-e2e check-openapi check-openapi-duplicates check-api-contract validate-api-response validate-sse build build-go build-web package package-arm64 package-dependency-parity docker-engine-api iso arm64-image netboot recovery-fixture recovery-api-smoke recovery-bundle-smoke recovery-persistence-smoke api-smoke api-smoke-strict update-fixture disk-identity-smoke storage-loopback privileged-storage-loopback disk-full-smoke share-config-smoke share-protocol-smoke iso-smoke qemu-installer-smoke qemu-recovery-smoke qemu-recovery-live security-smoke secret-scan-smoke command-boundary-smoke request-limits-smoke retention-smoke generation-retention-smoke fuzz-smoke dependency-smoke container-scan race-fuzz interop-smoke upgrade-compatibility release-gate-policy systemd-smoke systemd-security-smoke permission-smoke postinst-policy-smoke log-retention-smoke log-identity-smoke upgrade-smoke release-artifacts-smoke installer-signature-smoke installer-workdir-policy-smoke arm64-image-smoke netboot-smoke qemu-image qemu-smoke qemu-ab-smoke qemu-uefi-ab-smoke verify-release release-readiness

all: build

dev:
	bash scripts/dev.sh mock

dev-full:
	bash scripts/dev.sh full

test: test-go test-web check-openapi check-openapi-duplicates check-api-contract validate-api-response validate-sse

test-go:
	$(GO_ENV) go test ./...

check-openapi:
	python3 scripts/check-openapi.py

check-openapi-duplicates:
	python3 scripts/check-openapi-duplicates.py

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

frontend-live-e2e:
	cd web && pnpm test:e2e:live

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

# Sign the built-in app catalog with the release Ed25519 key. The detached
# signature is packaged as catalog/apps.json.sig and verified by lumonasd
# before any catalog app may be installed.
#
# LUMONAS_CATALOG_PRIVATE_KEY must be a standard-base64 Ed25519 seed or full
# private key. The matching public key is committed in
# packaging/debian/lumonasd.env.example.
catalog-sign:
	@test -n "$$LUMONAS_CATALOG_PRIVATE_KEY" || { echo "LUMONAS_CATALOG_PRIVATE_KEY is required to sign the catalog" >&2; exit 1; }
	mkdir -p bin
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-catalog-sign ./cmd/lumonas-catalog-sign
	./bin/lumonas-catalog-sign catalog/apps.json
	@echo "wrote catalog/apps.json.sig"

# Verify the committed catalog signature against the configured public key.
# This is a release gate: a tampered catalog must never ship.
catalog-verify:
	mkdir -p bin
	$(GO_ENV) go build -trimpath -ldflags "-s -w" -o bin/lumonas-catalog-sign ./cmd/lumonas-catalog-sign
	@test -f catalog/apps.json.sig || { echo "catalog/apps.json.sig is missing" >&2; exit 1; }
	@key="$$(sed -n 's/^LUMONAS_CATALOG_PUBLIC_KEY=//p' packaging/debian/lumonasd.env.example | head -n1)"; \
		test -n "$$key" || { echo "LUMONAS_CATALOG_PUBLIC_KEY is not set in the env example" >&2; exit 1; }; \
		./bin/lumonas-catalog-sign --verify catalog/apps.json catalog/apps.json.sig "$$key"

package:
	bash scripts/reproducible-package-smoke.sh $(VERSION)

package-arm64:
	LUMONAS_DEB_ARCH=arm64 bash scripts/reproducible-package-smoke.sh $(VERSION)

arm64-image: package-arm64
	LUMONAS_DEB="$(CURDIR)/lumonas_$(VERSION)_arm64.deb" LUMONAS_ARM64_IMAGE="$(CURDIR)/build/arm64/lumonas-debian13-arm64.img" bash installer/build-arm64.sh "$(VERSION)"

netboot: iso
	LUMONAS_ISO="$(CURDIR)/build/releases/lumonas-$(VERSION)-amd64.iso" LUMONAS_NETBOOT_DIR="$(CURDIR)/build/netboot" bash installer/build-netboot.sh "$(VERSION)"

arm64-image-smoke:
	bash scripts/arm64-image-smoke.sh $(VERSION)

netboot-smoke:
	bash scripts/netboot-smoke.sh $(VERSION)

qemu-ab-smoke:
	LUMONAS_AB_ASSERT="$${LUMONAS_AB_ASSERT:-false}" LUMONAS_QEMU_IMAGE="$(LUMONAS_QEMU_IMAGE)" LUMONAS_QEMU_SSH_KEY="$${LUMONAS_QEMU_SSH_KEY:-}" bash scripts/qemu-ab-smoke.sh

qemu-uefi-ab-smoke:
	LUMONAS_UEFI_AB_ASSERT="$${LUMONAS_UEFI_AB_ASSERT:-false}" LUMONAS_QEMU_IMAGE="$(LUMONAS_QEMU_IMAGE)" LUMONAS_QEMU_SSH_KEY="$${LUMONAS_QEMU_SSH_KEY:-}" LUMONAS_UEFI_AB_SSH_PORT="$${LUMONAS_UEFI_AB_SSH_PORT:-18024}" LUMONAS_UEFI_AB_WEB_PORT="$${LUMONAS_UEFI_AB_WEB_PORT:-18025}" bash scripts/qemu-uefi-ab-smoke.sh

package-dependency-parity:
	bash scripts/package-dependency-parity-smoke.sh

docker-engine-api:
	$(GO_ENV) go test ./internal/docker -run 'TestEngineAPIReadOnlyCollectors|TestEngineAPIRejectsEngineErrors|TestSplitImageReference|TestContainerCPUPercent'

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

api-smoke-strict:
	LUMONAS_API_SMOKE_ASSERT=true bash scripts/api-smoke.sh

update-fixture:
	mkdir -p build
	$(GO_ENV) go build -trimpath -o build/lumonas-update-fixture ./cmd/lumonas-update-fixture

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

iso-smoke: iso
	LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_ISO_ASSERT=true bash scripts/iso-smoke.sh

qemu-installer-smoke: iso
	sudo LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_INSTALLER_ASSERT=true LUMONAS_INSTALLER_CONTRACT_ASSERT=true bash scripts/qemu-installer-smoke.sh

qemu-recovery-smoke: iso recovery-fixture
	sudo LUMONAS_ISO="$(LUMONAS_ISO)" LUMONAS_RECOVERY_FIXTURE="$(CURDIR)/build/lumonas-recovery-fixture" LUMONAS_RECOVERY_ASSERT=true bash scripts/qemu-recovery-smoke.sh

qemu-recovery-live: iso qemu-image
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

interop-smoke:
	bash scripts/interop/run-all.sh

upgrade-compatibility:
	$(GO_ENV) go test ./internal/store -run 'TestStoreReopenPreservesStateAcrossMigrations|TestOpenMigratesLegacyEventSchema|TestOpenMigratesLegacyLanHostSchema|TestOpenMigratesLegacyRuntimeSchemaAsOneUpgrade|TestStorageSnapshotMigrationAddsOriginAndPreservesRows'

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

installer-workdir-policy-smoke:
	bash scripts/installer-workdir-policy-smoke.sh

qemu-image: package
	sudo LUMONAS_DEB="$(CURDIR)/lumonas_$(VERSION)_amd64.deb" LUMONAS_QEMU_IMAGE="$(LUMONAS_QEMU_IMAGE)" bash scripts/qemu-build-image.sh

qemu-smoke: qemu-image
	LUMONAS_QEMU_IMAGE="$(LUMONAS_QEMU_IMAGE)" LUMONAS_QEMU_ASSERT=true bash scripts/qemu-smoke.sh

verify-release:
	bash scripts/verify-release.sh build/releases

release-readiness: build check-openapi check-openapi-duplicates check-api-contract validate-api-response validate-sse release-gate-policy verify-release
