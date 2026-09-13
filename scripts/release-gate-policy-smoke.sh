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
api_smoke = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "api-smoke.sh"
api_smoke_text = api_smoke.read_text(encoding="utf-8")
if "Last-Event-ID" not in api_smoke_text or "cursor event twice" not in api_smoke_text:
    raise SystemExit("API smoke does not enforce non-duplicating SSE replay")
if "/api/v1/ups/config" not in api_smoke_text or '"names":[]' not in api_smoke_text:
    raise SystemExit("API smoke does not exercise persisted UPS configuration")
qemu_recovery_smoke = pathlib.Path(sys.argv[1]).parent.parent.parent / "scripts" / "qemu-recovery-smoke.sh"
if '"privilegedBroker":true' not in qemu_smoke.read_text(encoding="utf-8") or '"privilegedBroker":true' not in qemu_recovery_smoke.read_text(encoding="utf-8"):
    raise SystemExit("QEMU smoke does not assert the privileged broker readiness contract")
live_e2e = pathlib.Path(sys.argv[1]).parent.parent.parent / "web" / "e2e" / "live.spec.ts"
live_e2e_text = live_e2e.read_text(encoding="utf-8")
for marker in ("/api/v1/onboarding/complete", "/api/v1/events/stream", "snapraid.sync", "Overview"):
    if marker not in live_e2e_text:
        raise SystemExit(f"live browser smoke is missing real-runtime coverage: {marker}")
print("LumoNAS release gate policy passed: required blocking jobs are wired to publication")
PY
