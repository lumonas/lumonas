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
    "package", "package-permissions", "qemu-smoke", "iso", "installer-scripts",
    "safety-recovery", "recovery-api", "storage-loopback",
    "privileged-storage-loopback", "share-integrations", "compose-validation",
    "security-controls", "dependency-controls", "race-fuzz", "upgrade-debian",
}
actual = {item.strip() for item in match.group(1).split(",") if item.strip()}
missing = sorted(needed - actual)
if missing:
    raise SystemExit("release job is missing blocking gates: " + ", ".join(missing))
print("LumoNAS release gate policy passed: required blocking jobs are wired to publication")
PY
