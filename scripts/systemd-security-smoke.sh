#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
UNIT_DIR="$ROOT/packaging/systemd"
REQUIRE="${LUMONAS_REQUIRE_SYSTEMD_SECURITY:-false}"

if ! command -v systemd-analyze >/dev/null 2>&1; then
	if [ "$REQUIRE" = "true" ]; then
		echo "systemd-analyze is required for the systemd sandbox gate" >&2
		exit 1
	fi
	echo "systemd-analyze is not installed; sandbox policy verification skipped" >&2
	exit 0
fi

systemd-analyze verify "$UNIT_DIR"/*.service

python3 - "$UNIT_DIR" <<'PY'
import pathlib
import sys

unit_dir = pathlib.Path(sys.argv[1])
units = sorted(unit_dir.glob("*.service"))
if not units:
    raise SystemExit("no systemd service units found")

common = {
    "NoNewPrivileges": "true",
    "ProtectHome": "true",
    "ProtectSystem": "strict",
    "LockPersonality": "true",
    "MemoryDenyWriteExecute": "true",
}
resource_keys = ("MemoryMax", "CPUQuota", "TasksMax", "TimeoutStopSec", "UMask")

def service_values(path):
    values = {}
    section = ""
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("[") and line.endswith("]"):
            section = line[1:-1]
            continue
        if section != "Service" or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values.setdefault(key, []).append(value)
    return values

for path in units:
    values = service_values(path)
    name = path.name
    for key, expected in common.items():
        if values.get(key) != [expected]:
            raise SystemExit(f"{name}: {key} must be exactly {expected!r}")
    expected_private_tmp = "false" if name == "lumonas-runtime.service" else "true"
    if values.get("PrivateTmp") != [expected_private_tmp]:
        raise SystemExit(f"{name}: PrivateTmp must be exactly {expected_private_tmp!r}")
    for key in resource_keys:
        if not values.get(key) or not values[key][0].strip():
            raise SystemExit(f"{name}: {key} must set a finite resource limit")
    if values.get("ExecStart", [""])[0].startswith("/bin/sh"):
        raise SystemExit(f"{name}: ExecStart must not invoke a shell")

    if name in {"lumonas-web.service", "lumonasd.service"}:
        if values.get("User") != ["lumonas"] or values.get("Group") != ["lumonas"]:
            raise SystemExit(f"{name}: web and daemon services must run as lumonas")
        families = " ".join(values.get("RestrictAddressFamilies", []))
        required_families = ("AF_INET", "AF_INET6")
        if name == "lumonasd.service":
            required_families += ("AF_UNIX",)
        if any(family not in families for family in required_families):
            raise SystemExit(f"{name}: network service must explicitly restrict address families")
    elif name.startswith("lumonas-privd"):
        if values.get("User") != ["root"] or values.get("Group") != ["lumonas"]:
            raise SystemExit(f"{name}: privileged workers must run root:lumonas")
        if "AF_UNIX" not in " ".join(values.get("RestrictAddressFamilies", [])):
            raise SystemExit(f"{name}: privileged worker must be Unix-socket only")
        if "CapabilityBoundingSet" not in values:
            raise SystemExit(f"{name}: capability bounding set is missing")
    elif name == "lumonas-runtime.service":
        if values.get("User") != ["root"] or values.get("Group") != ["lumonas"]:
            raise SystemExit(f"{name}: runtime provisioner must run root:lumonas")
        if "AF_UNIX" not in " ".join(values.get("RestrictAddressFamilies", [])):
            raise SystemExit(f"{name}: runtime provisioner must be Unix-socket only")
        if "CapabilityBoundingSet" not in values:
            raise SystemExit(f"{name}: runtime capability bounding set is missing")

print(f"LumoNAS systemd sandbox policy verified ({len(units)} units)")
PY
