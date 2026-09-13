#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORKFLOW="$ROOT/.github/workflows/ci.yml"

python3 - "$WORKFLOW" <<'PY'
import pathlib
import re
import sys

workflow = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
makefile = pathlib.Path(sys.argv[1]).parent.parent.parent / "Makefile"
makefile_text = makefile.read_text(encoding="utf-8")
for target, dependency_text in (
    ("qemu-smoke", "qemu-smoke: qemu-image"),
    ("qemu-recovery-smoke", "qemu-recovery-smoke: iso recovery-fixture"),
    ("qemu-recovery-live", "qemu-recovery-live: iso qemu-image"),
    ("iso-smoke", "iso-smoke: iso"),
    ("qemu-installer-smoke", "qemu-installer-smoke: iso"),
):
    if dependency_text not in makefile_text:
        raise SystemExit(f"Makefile target {target} is missing its artifact dependency chain")
for variable, marker in (
    ("LUMONAS_ISO", "LUMONAS_ISO ?= $(CURDIR)/build/releases/lumonas-$(VERSION)-amd64.iso"),
    ("LUMONAS_QEMU_IMAGE", "LUMONAS_QEMU_IMAGE ?= $(CURDIR)/build/qemu/lumonas-debian13.raw"),
):
    if marker not in makefile_text:
        raise SystemExit(f"Makefile does not define a deterministic default for {variable}")
match = re.search(r"(?ms)^  release:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?^    needs: \[([^\]]+)\]", workflow)
repo_root = pathlib.Path(sys.argv[1]).parent.parent.parent
if not match:
    raise SystemExit("release job needs list is missing")

needed = {
    "package", "package-permissions", "deb-verify", "qemu-smoke", "iso", "installer-scripts", "frontend-e2e", "frontend-live-e2e",
    "safety-recovery", "recovery-api", "storage-loopback",
    "privileged-storage-loopback", "disk-full", "share-integrations", "compose-validation",
    "security-controls", "dependency-controls", "race-fuzz", "upgrade-compatibility",
    "upgrade-debian",
}
actual = {item.strip() for item in match.group(1).split(",") if item.strip()}
missing = sorted(needed - actual)
if missing:
    raise SystemExit("release job is missing blocking gates: " + ", ".join(missing))
frontend_e2e = re.search(r"(?ms)^  frontend-e2e:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
if not frontend_e2e or "pnpm test:e2e" not in frontend_e2e.group(0):
    raise SystemExit("frontend-e2e job is not running the browser smoke suite")
if not (repo_root / "web/e2e/network-lan.spec.ts").is_file():
    raise SystemExit("LAN browser smoke coverage is missing")
for job in ("qemu-smoke", "package-permissions"):
    job_match = re.search(rf"(?ms)^  {job}:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
    if not job_match or not re.search(r"^    needs: \[package, deb-verify\]$", job_match.group(0), re.M):
        raise SystemExit(f"{job} must wait for the Debian manifest gate")
for job, artifact in (("qemu-smoke", "lumonas-qemu"), ("iso", "lumonas-iso-smoke")):
    job_match = re.search(rf"(?ms)^  {job}:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
    block = job_match.group(0) if job_match else ""
    if "uses: actions/upload-artifact@v4" not in block or "if: always()" not in block or artifact not in block:
        raise SystemExit(f"{job} must retain appliance diagnostics when the gate fails")
iso_job = re.search(r"(?ms)^  iso:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
iso_block = iso_job.group(0) if iso_job else ""
for marker in ("LUMONAS_RECOVERY_DEBUG_DIR", "build/qemu/recovery-debug/**"):
    if marker not in iso_block:
        raise SystemExit(f"ISO/recovery gate does not preserve failure diagnostics: {marker}")
if "qemu-installer-smoke.sh" not in iso_block or "LUMONAS_INSTALLER_ASSERT=true" not in iso_block:
    raise SystemExit("ISO gate is missing the real live installer smoke")
if "LUMONAS_INSTALLER_CONTRACT_ASSERT=true" not in iso_block:
    raise SystemExit("ISO gate is missing the installed runtime contract assertion")
installer_smoke = (pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "qemu-installer-smoke.sh").read_text(encoding="utf-8")
for marker in ("installed-disks.json", "installed-metrics.json", "installed-jobs.json", "installed-health.json", "installed-services.json", "validate-sse.py", "LUMONAS_INSTALLER_DEBUG_DIR"):
    if marker not in installer_smoke:
        raise SystemExit(f"installer gate is missing post-install contract validation: {marker}")
qemu_job = re.search(r"(?ms)^  qemu-smoke:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
qemu_block = qemu_job.group(0) if qemu_job else ""
if "actions/setup-go@v5" not in qemu_block or "./cmd/lumonas-update-fixture" not in qemu_block:
    raise SystemExit("QEMU smoke must provision Go and build the signed update fixture")
safety_job = re.search(r"(?ms)^  safety-recovery:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
safety_block = safety_job.group(0) if safety_job else ""
for marker in ("TestPoolSetupPlanRejectsSanitizedBranchCollision", "TestRenderSnapraidConfigRejectsUnstableOrCollidingIdentities", "TestValidateSnapraidConfigRejectsBranchCollisions", "TestReplacementPlanRejectsUnstableReplacementIdentity", "TestReplacementPlanRejectsAmbiguousSanitizedBranch", "TestReplacementPlanRejectsTamperedHash", "TestDiskReplacementPlanAndConfirmChain"):
    if marker not in safety_block:
        raise SystemExit(f"safety-recovery gate is missing replacement coverage: {marker}")
if "TestEnsureRestartedJobsFailsClosedForInterruptedWork" not in safety_block:
    raise SystemExit("safety-recovery gate is missing restart job event coverage")
if "TestPublishActorSetsEventEnvelopeWithoutPayloadMutation" not in safety_block:
    raise SystemExit("safety-recovery gate is missing request actor event coverage")
if "TestRunDueSchedulesFiresSnapshotScheduleAndPersistsOrigin" not in safety_block:
    raise SystemExit("safety-recovery gate is missing scheduled snapshot coverage")
if "TestFailInterruptedBackupRunsClosesRunsAndCopies" not in safety_block:
    raise SystemExit("safety-recovery gate is missing interrupted backup reconciliation coverage")
if "TestBackupCompletionEventRequiresDurableState" not in safety_block or "TestBackupDoesNotStartWhenRunningStateCannotPersist" not in safety_block:
    raise SystemExit("safety-recovery gate is missing backup persistence fail-closed coverage")
security_smoke = (pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "security-smoke.sh").read_text(encoding="utf-8")
if "TestRequestAuditPersistsTypedObservabilityFields" not in security_smoke:
    raise SystemExit("security-controls gate is missing typed audit observability coverage")
for marker in ("TestPeerAllowedRejectsNonUnixConnections", "TestPeerAllowedAcceptsCurrentUnixPeer", "TestPeerAllowedRejectsUnixPeerOutsideServiceGroup"):
    if marker not in security_smoke:
        raise SystemExit(f"security-controls gate is missing privileged peer-boundary coverage: {marker}")
security_job = re.search(r"(?ms)^  security-controls:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
if not security_job or "TestPeerAllowedRejectsUnixPeerOutsideServiceGroup" not in security_job.group(0):
    raise SystemExit("security-controls CI job does not explicitly run peer-credential rejection coverage")
api_smoke_text = (repo_root / "scripts/api-smoke.sh").read_text(encoding="utf-8")
if "audit response did not include typed trace fields after a mutation" not in api_smoke_text:
    raise SystemExit("API smoke does not require typed audit trace fields after a mutation")
if "writable Unix-domain socket in assertion mode" not in api_smoke_text or "SOCKET_PROBE" not in api_smoke_text:
    raise SystemExit("API smoke does not fail closed on an unavailable privileged socket boundary")
peercred_test = (repo_root / "cmd/lumonas-privd/peercred_linux_test.go").read_text(encoding="utf-8")
if "TestPeerAllowedRejectsUnixPeerOutsideServiceGroup" not in peercred_test:
    raise SystemExit("privileged broker coverage is missing wrong-service-group rejection")
for script, marker in (("scripts/api-smoke.sh", "/api/v1/events"), ("scripts/qemu-smoke.sh", "EVENTS_COMPAT_LOG"), ("scripts/iso-smoke.sh", "validate-sse.py"), ("scripts/qemu-recovery-smoke.sh", "RECOVERED_EVENTS_LOG")):
    if marker not in (repo_root / script).read_text(encoding="utf-8"):
        raise SystemExit(f"runtime contract smoke is missing compatibility SSE coverage: {script}")
for script in ("scripts/qemu-smoke.sh", "scripts/iso-smoke.sh", "scripts/qemu-live-recovery-source.sh", "scripts/qemu-recovery-smoke.sh"):
    if 'validate-api-response.py" services' not in (repo_root / script).read_text(encoding="utf-8"):
        raise SystemExit(f"runtime contract smoke is missing service response validation: {script}")
if "docker-engine-api" not in match.group(1):
    raise SystemExit("release job must require the Docker Engine API gate")
for marker in ("TestEngineAPIReadOnlyCollectors", "TestEngineAPIRejectsEngineErrors", "TestSplitImageReference", "TestContainerCPUPercent", "TestImageUsageFallsBackToContainerReference", "TestImageIDNormalization", "TestVolumeUsageFailureKeepsInventory"):
    if marker not in workflow:
        raise SystemExit(f"Docker Engine API coverage is missing from CI: {marker}")
if '"available":true' not in (repo_root / "scripts/qemu-smoke.sh").read_text(encoding="utf-8") or '"available":true' not in (repo_root / "scripts/qemu-recovery-smoke.sh").read_text(encoding="utf-8"):
    raise SystemExit("QEMU runtime smoke must require Docker Engine availability")
validator_text = (repo_root / "scripts/validate-api-response.py").read_text(encoding="utf-8")
for marker in ("docker-containers", "docker-images", "docker-volumes", "audit", "lan-hosts", "readiness", "recovery-status", "recovery-plan", "updates-status"):
    if marker not in validator_text:
        raise SystemExit(f"Docker response contract validator is missing {marker}")
for script in ("scripts/api-smoke.sh", "scripts/qemu-smoke.sh", "scripts/qemu-recovery-smoke.sh"):
    script_text = (repo_root / script).read_text(encoding="utf-8")
    if script == "scripts/api-smoke.sh":
        if "for docker_resource in containers images volumes" not in script_text:
            raise SystemExit("Docker inventory API smoke is missing its complete resource loop")
    else:
        for marker in ("docker/containers", "docker/images", "docker/volumes"):
            if marker not in script_text:
                raise SystemExit(f"Docker inventory smoke is missing {marker}: {script}")
    if script != "scripts/qemu-recovery-smoke.sh" and "/api/v1/audit" not in script_text:
        raise SystemExit(f"audit response smoke is missing from {script}")
for script in ("scripts/api-smoke.sh", "scripts/qemu-smoke.sh", "scripts/qemu-recovery-smoke.sh"):
    if "health/components" not in (repo_root / script).read_text(encoding="utf-8"):
        raise SystemExit(f"health component smoke is missing from {script}")
api_contract = (repo_root / "scripts/api-smoke.sh").read_text(encoding="utf-8")
for marker in ("validate_response recovery-status", "validate_response recovery-plan", "validate_response updates-status"):
    if marker not in api_contract:
        raise SystemExit(f"API smoke is missing {marker}")
if "validate_response lan-hosts" not in api_contract or "validate_response readiness" not in api_contract:
    raise SystemExit("API smoke is missing LAN/readiness response validation")
qemu_contract = (repo_root / "scripts/qemu-smoke.sh").read_text(encoding="utf-8")
for marker in ("validate-api-response.py\" recovery-status", "validate-api-response.py\" recovery-plan", "validate-api-response.py\" updates-status"):
    if marker not in qemu_contract:
        raise SystemExit(f"QEMU smoke is missing {marker}")
if "validate-api-response.py\" lan-hosts" not in qemu_contract or "validate-api-response.py\" readiness" not in qemu_contract:
    raise SystemExit("QEMU smoke is missing LAN/readiness response validation")
for script, markers in (
    ("scripts/iso-smoke.sh", ("validate-api-response.py", "readiness", "server")),
    ("scripts/qemu-recovery-smoke.sh", ("recovery-plan", "readiness", "server")),
):
    script_text = (repo_root / script).read_text(encoding="utf-8")
    if any(marker not in script_text for marker in markers):
        raise SystemExit(f"{script} is missing readiness/recovery contract validation")
collector_text = (repo_root / "internal/collector/docker.go").read_text(encoding="utf-8")
if "dockerruntime.New(\"\", nil)" not in collector_text or "DockerSummaryFromService" not in collector_text:
    raise SystemExit("legacy Docker collector is not delegated to the controlled runtime service")
if "docker ps" in collector_text or "exec.LookPath" in collector_text:
    raise SystemExit("legacy Docker collector still parses direct CLI output")
upgrade_job = re.search(r"(?ms)^  upgrade-compatibility:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
if not upgrade_job or "TestStorageSnapshotMigrationAddsOriginAndPreservesRows" not in upgrade_job.group(0) or "TestOpenMigratesLegacyJobObservabilitySchema" not in upgrade_job.group(0) or "TestOpenMigratesLegacyLanHostSchema" not in upgrade_job.group(0):
    raise SystemExit("upgrade-compatibility gate is missing snapshot migration coverage")
upgrade_debian = re.search(r"(?ms)^  upgrade-debian:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
if not upgrade_debian or "needs: [package, deb-verify]" not in upgrade_debian.group(0) or "scripts/upgrade-smoke.sh" not in upgrade_debian.group(0):
    raise SystemExit("tagged Debian upgrade gate must depend on package verification and run the upgrade smoke")
upgrade_smoke = (repo_root / "scripts" / "upgrade-smoke.sh").read_text(encoding="utf-8")
for marker in ("apt-get install -y --no-install-recommends systemd passwd ca-certificates sqlite3", "CREATE TABLE network_connections", "legacy-user", "pragma_table_info", "service_bindings"):
    if marker not in upgrade_smoke:
        raise SystemExit(f"Debian upgrade smoke is missing legacy migration coverage: {marker}")
upgrade_order = (repo_root / "scripts" / "upgrade-service-order-smoke.sh").read_text(encoding="utf-8")
if "runuser -u lumonas -- /usr/lib/lumonas/lumonas-migrate" not in upgrade_order:
    raise SystemExit("upgrade ordering smoke does not require migrations before service restart")
race_match = re.search(r"(?ms)^  race-fuzz:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
if not race_match or not re.search(r"^\s+- run: .*scripts/race-fuzz-smoke\.sh", race_match.group(0), re.M):
    raise SystemExit("race-fuzz job is not running the centralized race/fuzz harness")
race_script = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "race-fuzz-smoke.sh"
if "scripts/fuzz-smoke.sh" not in race_script.read_text(encoding="utf-8"):
    raise SystemExit("race/fuzz harness does not delegate to the bounded fuzz suite")
if "go test -race ./..." not in race_script.read_text(encoding="utf-8"):
    raise SystemExit("race/fuzz harness does not cover every Go package")
installer = pathlib.Path(sys.argv[1]).parent.parent.parent / "installer" / "build-iso.sh"
installer_text = installer.read_text(encoding="utf-8")
for marker in (
    "REPO_SIGNATURE_REQUIRED",
    "signed LumoNAS repository metadata could not be verified",
    "signed LumoNAS repository package installation failed",
):
    if marker not in installer_text:
        raise SystemExit(f"installer signed-repository fail-closed marker is missing: {marker}")
qemu_smoke = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "qemu-smoke.sh"
if "validate-api-response.py" not in qemu_smoke.read_text(encoding="utf-8") or "validate-sse.py" not in qemu_smoke.read_text(encoding="utf-8"):
    raise SystemExit("QEMU smoke does not validate live API and SSE response contracts")
for marker in ("LUMONAS_QEMU_SSH_ASSERT", "systemctl restart lumonasd.service", "REPLAY_AFTER_RESTART", "Last-Event-ID: $RESTART_EVENT_ID"):
    if marker not in qemu_smoke.read_text(encoding="utf-8"):
        raise SystemExit(f"QEMU smoke does not validate daemon restart persistence: {marker}")
for marker in ("Create ephemeral QEMU SSH key", "LUMONAS_QEMU_SSH_PUBLIC_KEY", "LUMONAS_QEMU_SSH_PRIVATE_KEY", "Remove ephemeral QEMU SSH access", "authorized_keys", "update-fixture/package", "LUMONAS_UPDATE_PUBLIC_KEY"):
    if marker not in workflow:
        raise SystemExit(f"QEMU CI job is missing ephemeral guest control: {marker}")
if "metrics-history" not in qemu_smoke.read_text(encoding="utf-8"):
    raise SystemExit("QEMU smoke does not validate retained system metrics")
for marker in ("LUMONAS_QEMU_UPDATE_ASSERT", "updates/apply", "qemu smoke rollback", "activeSlot"):
    if marker not in qemu_smoke.read_text(encoding="utf-8"):
        raise SystemExit(f"QEMU smoke does not exercise signed update rollback: {marker}")
if "if curl -kfsS -X POST -H" not in qemu_smoke.read_text(encoding="utf-8"):
    raise SystemExit("QEMU signed-update smoke must guard the apply request with a shell conditional")
disk_identity_smoke = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "disk-identity-smoke.sh"
if "DisksPromoteMountedPartitionMetadata" not in disk_identity_smoke.read_text(encoding="utf-8"):
    raise SystemExit("disk identity smoke does not cover mounted partition metadata")
storage_loopback = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "storage-loopback-smoke.sh"
if "TestDisksReadOnlyIdentityAgainstRealDevice" not in storage_loopback.read_text(encoding="utf-8"):
    raise SystemExit("storage loopback smoke does not exercise the production disk collector")
collector_tests = pathlib.Path(sys.argv[1]).parent.parent.parent / "internal" / "collector" / "disks_test.go"
if "TestDisksReadOnlyIdentityAgainstRealDevice" not in collector_tests.read_text(encoding="utf-8"):
    raise SystemExit("collector is missing its real-device read-only identity test")
qemu_builder = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "qemu-build-image.sh"
for marker in ("LUMONAS_UPDATE_FIXTURE", "LUMONAS_UPDATE_PUBLIC_KEY", "update-fixture/package", "chown -R lumonas:lumonas /var/lib/lumonas/update-fixture"):
    if marker not in qemu_builder.read_text(encoding="utf-8"):
        raise SystemExit(f"QEMU builder does not stage the signed update fixture: {marker}")
for builder, markers in (
    (qemu_builder, ("snapshot.debian.org/archive/debian/20260201T000000Z", "LUMONAS_DEBIAN_MIRROR", "LUMONAS_DEBIAN_MIRROR must use HTTPS", "debianMirror=$LUMONAS_DEBIAN_MIRROR", "Acquire::Check-Valid-Until")),
    (pathlib.Path(sys.argv[1]).parent.parent.parent / "installer" / "build-iso.sh", ("snapshot.debian.org/archive/debian/20260201T000000Z", "LUMONAS_DEBIAN_MIRROR", "LUMONAS_DEBIAN_MIRROR must use HTTPS", "debianMirror=$DEBIAN_MIRROR", "Acquire::Check-Valid-Until")),
):
    builder_text = builder.read_text(encoding="utf-8")
    for marker in markers:
        if marker not in builder_text:
            raise SystemExit(f"Debian appliance builder is missing reproducibility marker: {marker}")
installer = pathlib.Path(sys.argv[1]).parent.parent.parent / "installer" / "build-iso.sh"
for marker in ("LUMONAS_ISO_WORKDIR must be an absolute path", "LUMONAS_ISO_CACHE_SOURCE must not be inside the ISO workdir", 'rm -rf "$WORK"'):
    if marker not in installer.read_text(encoding="utf-8"):
        raise SystemExit(f"installer workdir safety marker is missing: {marker}")
api_smoke = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "api-smoke.sh"
api_smoke_text = api_smoke.read_text(encoding="utf-8")
if "Last-Event-ID" not in api_smoke_text or "cursor event twice" not in api_smoke_text:
    raise SystemExit("API smoke does not enforce non-duplicating SSE replay")
if "/api/v1/ups/config" not in api_smoke_text or '"names":[]' not in api_smoke_text:
    raise SystemExit("API smoke does not exercise persisted UPS configuration")
if "/api/v1/ups/status" not in api_smoke_text:
    raise SystemExit("API smoke does not exercise configured UPS status")
shutdown_text = (repo_root / "cmd" / "lumonasd" / "shutdown_api.go").read_text(encoding="utf-8")
for marker in ("ups.shutdown.pending", "ups.shutdown.failed", "context.WithTimeout(context.Background(), 30*time.Second)"):
    if marker not in shutdown_text:
        raise SystemExit(f"UPS shutdown observability is missing {marker}")
if "/api/v1/system/metrics/history" not in api_smoke_text:
    raise SystemExit("API smoke does not exercise persisted system metrics")
for marker in ("start_privileged_stack()", 'LUMONAS_PRIVD_SOCKET="$PRIVD_DIR/privd.sock"', "-worker \"$worker\""):
    if marker not in api_smoke_text:
        raise SystemExit(f"API smoke does not start the real privileged stack: {marker}")
for marker in ("updates/apply", "api smoke rollback", "LUMONAS_UPDATE_PUBLIC_KEY"):
    if marker not in api_smoke_text:
        raise SystemExit(f"API smoke does not exercise the signed update rollback path: {marker}")
qemu_recovery_smoke = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "qemu-recovery-smoke.sh"
iso_smoke = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "iso-smoke.sh"
live_recovery_source = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "qemu-live-recovery-source.sh"
if any('"privilegedBroker":true' not in path.read_text(encoding="utf-8") for path in (qemu_smoke, qemu_recovery_smoke, iso_smoke, live_recovery_source)):
    raise SystemExit("QEMU smoke does not assert the privileged broker readiness contract")
readiness_validator = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "validate-api-response.py"
readiness_text = readiness_validator.read_text(encoding="utf-8")
if "privilegedWorkers" not in readiness_text:
    raise SystemExit("readiness validator does not require privilegedWorkers")
daemon_source = (pathlib.Path(sys.argv[1]).parent.parent.parent / "cmd" / "lumonasd" / "main.go").read_text(encoding="utf-8")
for marker in ("privilegedWorkersReady", "worker.ping."):
    if marker not in daemon_source:
        raise SystemExit(f"daemon readiness is missing {marker}")
live_recovery_text = live_recovery_source.read_text(encoding="utf-8")
for marker in ("SOURCE_API=\"https://127.0.0.1:18083\"", "curl -kfsS \"$SOURCE_API/healthz\"", "SOURCE_COOKIES=", "SOURCE_CSRF=", "X-CSRF-Token: $SOURCE_CSRF", "api/v1/recovery/export"):
    if marker not in live_recovery_text:
        raise SystemExit(f"live recovery source is missing its HTTPS/auth contract: {marker}")
for marker in ("live recovery source shutdown request failed", "live recovery source appliance did not power off cleanly", "wait \"$SOURCE_PID\""):
    if marker not in live_recovery_text:
        raise SystemExit(f"live recovery source is missing clean-shutdown enforcement: {marker}")
if 'api/v1/power/shutdown" >/dev/null 2>&1 || true' in live_recovery_text:
    raise SystemExit("live recovery source ignores a failed shutdown request")
recovery_text = qemu_recovery_smoke.read_text(encoding="utf-8")
if 'wait "$QEMU_PID"\n' not in recovery_text:
    raise SystemExit("QEMU recovery smoke does not make guest exit status authoritative")
if 'wait "$QEMU_PID" || true' in recovery_text:
    raise SystemExit("QEMU recovery smoke ignores guest exit status")
for path in (qemu_smoke, qemu_recovery_smoke, iso_smoke, live_recovery_source):
    text = path.read_text(encoding="utf-8")
    if text.count("hostfwd=") != text.count("restrict=on,hostfwd="):
        raise SystemExit(f"QEMU appliance smoke is not isolated from guest egress: {path.name}")
recovery_text = qemu_recovery_smoke.read_text(encoding="utf-8")
for marker in ("recovered-disks.json", "recovered-metrics.json", "recovered-jobs.json", "validate-api-response.py\" services"):
    if marker not in recovery_text:
        raise SystemExit(f"recovery smoke is missing full runtime contract coverage: {marker}")
iso_text = iso_smoke.read_text(encoding="utf-8")
if "LOG.disks" not in iso_text or "validate-api-response.py\" disks" not in iso_text:
    raise SystemExit("ISO smoke is missing disk response contract validation")
live_e2e = pathlib.Path(sys.argv[1]).parent.parent.parent / "web" / "e2e" / "live.spec.ts"
live_e2e_text = live_e2e.read_text(encoding="utf-8")
for marker in ("/api/v1/onboarding/complete", "/api/v1/events/stream", "snapraid.sync", "Overview"):
    if marker not in live_e2e_text:
        raise SystemExit(f"live browser smoke is missing real-runtime coverage: {marker}")
onboarding = pathlib.Path(sys.argv[1]).parent.parent.parent / "cmd" / "lumonasd" / "onboarding_api.go"
onboarding_text = onboarding.read_text(encoding="utf-8")
for marker in ("onboardingProtectionReady", "Do not start SnapRAID while onboarding"):
    if marker not in onboarding_text:
        raise SystemExit(f"onboarding sync safety marker is missing: {marker}")
privd = pathlib.Path(sys.argv[1]).parent.parent.parent / "cmd" / "lumonas-privd" / "main.go"
privd_text = privd.read_text(encoding="utf-8")
for marker in ("could not verify target mount state", '"lsblk", "-nrpo", "MOUNTPOINT"'):
    if marker not in privd_text:
        raise SystemExit(f"privileged mount-state safety marker is missing: {marker}")
privileged_loopback = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "privileged-storage-loopback-smoke.sh"
privileged_loopback_text = privileged_loopback.read_text(encoding="utf-8")
for marker in ("sfdisk", "partx --update", "partition-mounted", "mounted-partition rejection"):
    if marker not in privileged_loopback_text:
        raise SystemExit(f"privileged loopback smoke is missing partition coverage: {marker}")
snapshot_browse = (repo_root / "cmd" / "lumonas-privd" / "browse_test.go").read_text(encoding="utf-8")
if "TestSnapshotBrowseRejectsEscapeAndUnknownPaths" not in snapshot_browse:
    raise SystemExit("snapshot browse safety coverage is missing path traversal rejection")
snapshot_api = (repo_root / "cmd" / "lumonasd" / "snapshot_browse_api_test.go").read_text(encoding="utf-8")
if "TestStorageSnapshotFilesListsBrokerEntries" not in snapshot_api:
    raise SystemExit("snapshot browse API coverage is missing broker response handling")
if "/storage/snapshots/{id}/files" not in (repo_root / "docs" / "openapi.yaml").read_text(encoding="utf-8"):
    raise SystemExit("snapshot browse endpoint is missing from OpenAPI")
for path, markers in (
    (repo_root / "installer" / "build-arm64.sh", ("LUMONAS_DEB", "grub-install --target=arm64-efi", 'losetup -d "$LOOP"')),
    (repo_root / "installer" / "build-netboot.sh", ("lumonas.squashfs", "grub.cfg", "ipxe")),
    (repo_root / "scripts" / "arm64-image-smoke.sh", ("LUMONAS_ARM64_ASSERT", "LUMONAS_DEB_ARCH=arm64")),
    (repo_root / "scripts" / "netboot-smoke.sh", ("LUMONAS_NETBOOT_ASSERT", "lumonas.squashfs")),
):
    text = path.read_text(encoding="utf-8")
    for marker in markers:
        if marker not in text:
            raise SystemExit(f"installer variant gate is missing {marker}: {path.name}")
workflow_text = (repo_root / ".github" / "workflows" / "ci.yml").read_text(encoding="utf-8")
if "make arm64-image-smoke netboot-smoke" not in workflow_text:
    raise SystemExit("CI does not run architecture and netboot packaging smokes")
ab_smoke = repo_root / "scripts" / "qemu-ab-smoke.sh"
ab_text = ab_smoke.read_text(encoding="utf-8")
for marker in ("LUMONAS_AB_ASSERT", "system.slot.write", "system.slot.bootnext", "slot confirm should fail closed", "restrict=on"):
    if marker not in ab_text:
        raise SystemExit(f"QEMU A/B smoke is missing {marker}")
if "LUMONAS_AB_ASSERT=true" not in workflow_text:
    raise SystemExit("QEMU release job does not run the A/B slot smoke")
makefile_text = (repo_root / "Makefile").read_text(encoding="utf-8")
if 'qemu-ab-smoke:' not in makefile_text or 'LUMONAS_AB_ASSERT="$${LUMONAS_AB_ASSERT:-false}"' not in makefile_text:
    raise SystemExit("Makefile A/B smoke target does not preserve shell assertion defaults")
for marker in ("package-arm64:", "LUMONAS_CC=aarch64-linux-gnu-gcc", "name: lumonas-deb-arm64", "deb-verify-arm64:"):
    if marker not in workflow_text:
        raise SystemExit(f"CI does not build and verify the arm64 package: {marker}")
slot_source = (repo_root / "cmd" / "lumonas-privd" / "system_slots.go").read_text(encoding="utf-8")
for marker in ("slotTargetStat", "ModeCharDevice", "slot image digest does not match", "could not verify slot target mount state"):
    if marker not in slot_source:
        raise SystemExit(f"slot writer safety guard is missing {marker}")
slot_tests = (repo_root / "cmd" / "lumonas-privd" / "system_slots_test.go").read_text(encoding="utf-8")
for marker in ("TestSlotWriteVerifiesDigestBeforeWrite", "TestSlotWriteFailsClosedWhenMountStateUnknown", "TestSlotWriteRejectsCharacterDevice"):
    if marker not in slot_tests:
        raise SystemExit(f"slot writer safety test is missing {marker}")
update_source = (repo_root / "internal" / "updates" / "updates.go").read_text(encoding="utf-8")
if "syncDirectory(filepath.Dir(target))" not in update_source:
    raise SystemExit("A/B staging does not sync the target directory")
slot_image_source = (repo_root / "internal" / "updates" / "slotimage.go").read_text(encoding="utf-8")
if "slot health version does not match pending version" not in slot_image_source:
    raise SystemExit("A/B slot promotion does not bind health to the pending version")
slot_api = (repo_root / "cmd" / "lumonasd" / "slot_api.go").read_text(encoding="utf-8")
for marker in ("parseSlotDeviceMapping", "stageSlotImage", "activateSlotImage", "confirmSlotImage", "system.slot.write", "system.slot.bootnext"):
    if marker not in slot_api:
        raise SystemExit(f"slot API integration is missing {marker}")
slot_api_tests = (repo_root / "cmd" / "lumonasd" / "slot_api_test.go").read_text(encoding="utf-8")
for marker in ("TestParseSlotDeviceMappingRejectsUnsafeAndDuplicateEntries", "TestSlotAPIStagesActivatesAndConfirmsSignedImage"):
    if marker not in slot_api_tests:
        raise SystemExit(f"slot API coverage is missing {marker}")
openapi_text = (repo_root / "docs" / "openapi.yaml").read_text(encoding="utf-8")
for marker in ("/updates/slot/stage:", "/updates/slot/activate:", "/updates/slot/confirm:"):
    if marker not in openapi_text:
        raise SystemExit(f"slot API route is missing from OpenAPI: {marker}")
print("LumoNAS release gate policy passed: required blocking jobs are wired to publication")
PY
