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
race_match = re.search(r"(?ms)^  race-fuzz:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?(?=^  [A-Za-z0-9_-]+:|\Z)", workflow)
if not race_match or not re.search(r"^\s+- run: .*scripts/race-fuzz-smoke\.sh", race_match.group(0), re.M):
    raise SystemExit("race-fuzz job is not running the centralized race/fuzz harness")
race_script = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "race-fuzz-smoke.sh"
if "scripts/fuzz-smoke.sh" not in race_script.read_text(encoding="utf-8"):
    raise SystemExit("race/fuzz harness does not delegate to the bounded fuzz suite")
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
for marker in ("LUMONAS_QEMU_UPDATE_ASSERT", "updates/apply", "qemu smoke rollback", "activeSlot"):
    if marker not in qemu_smoke.read_text(encoding="utf-8"):
        raise SystemExit(f"QEMU smoke does not exercise signed update rollback: {marker}")
qemu_builder = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "qemu-build-image.sh"
for marker in ("LUMONAS_UPDATE_FIXTURE", "LUMONAS_UPDATE_PUBLIC_KEY", "update-fixture/package", "chown -R lumonas:lumonas /var/lib/lumonas/update-fixture"):
    if marker not in qemu_builder.read_text(encoding="utf-8"):
        raise SystemExit(f"QEMU builder does not stage the signed update fixture: {marker}")
api_smoke = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "api-smoke.sh"
api_smoke_text = api_smoke.read_text(encoding="utf-8")
if "Last-Event-ID" not in api_smoke_text or "cursor event twice" not in api_smoke_text:
    raise SystemExit("API smoke does not enforce non-duplicating SSE replay")
if "/api/v1/ups/config" not in api_smoke_text or '"names":[]' not in api_smoke_text:
    raise SystemExit("API smoke does not exercise persisted UPS configuration")
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
live_e2e = pathlib.Path(sys.argv[1]).parent.parent.parent / "web" / "e2e" / "live.spec.ts"
live_e2e_text = live_e2e.read_text(encoding="utf-8")
for marker in ("/api/v1/onboarding/complete", "/api/v1/events/stream", "snapraid.sync", "Overview"):
    if marker not in live_e2e_text:
        raise SystemExit(f"live browser smoke is missing real-runtime coverage: {marker}")
print("LumoNAS release gate policy passed: required blocking jobs are wired to publication")
PY
