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
security_smoke = (pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "security-smoke.sh").read_text(encoding="utf-8")
if "TestRequestAuditPersistsTypedObservabilityFields" not in security_smoke:
    raise SystemExit("security-controls gate is missing typed audit observability coverage")
api_smoke_text = (repo_root / "scripts/api-smoke.sh").read_text(encoding="utf-8")
if "audit response did not include typed trace fields after a mutation" not in api_smoke_text:
    raise SystemExit("API smoke does not require typed audit trace fields after a mutation")
for script, marker in (("scripts/api-smoke.sh", "/api/v1/events"), ("scripts/qemu-smoke.sh", "EVENTS_COMPAT_LOG"), ("scripts/iso-smoke.sh", "validate-sse.py"), ("scripts/qemu-recovery-smoke.sh", "RECOVERED_EVENTS_LOG")):
    if marker not in (repo_root / script).read_text(encoding="utf-8"):
        raise SystemExit(f"runtime contract smoke is missing compatibility SSE coverage: {script}")
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
print("LumoNAS release gate policy passed: required blocking jobs are wired to publication")
PY
