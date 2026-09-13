#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORKFLOW="$ROOT/.github/workflows/ci.yml"

python3 - "$WORKFLOW" <<'PY'
import pathlib
import re
import sys

workflow = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
match = re.search(r"(?ms)^  release:\n(?:(?!^  [A-Za-z0-9_-]+:).)*?^    needs: \[([^\]]+)\]", workflow)
if not match:
    raise SystemExit("release job needs list is missing")

needed = {
    "package", "package-permissions", "qemu-smoke", "iso", "installer-scripts", "frontend-e2e",
    "safety-recovery", "recovery-api", "storage-loopback",
    "privileged-storage-loopback", "disk-full", "share-integrations", "compose-validation",
    "security-controls", "dependency-controls", "race-fuzz", "upgrade-compatibility",
    "upgrade-debian",
}
actual = {item.strip() for item in match.group(1).split(",") if item.strip()}
missing = sorted(needed - actual)
if missing:
    raise SystemExit("release job is missing blocking gates: " + ", ".join(missing))
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
if "Last-Event-ID" not in api_smoke.read_text(encoding="utf-8") or "cursor event twice" not in api_smoke.read_text(encoding="utf-8"):
    raise SystemExit("API smoke does not enforce non-duplicating SSE replay")
print("LumoNAS release gate policy passed: required blocking jobs are wired to publication")
PY
