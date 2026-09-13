#!/usr/bin/env python3
"""Reject duplicate OpenAPI path or operation keys."""

from pathlib import Path
import re
import sys


ROOT = Path(__file__).resolve().parent.parent
SPEC = ROOT / "docs" / "openapi.yaml"
METHOD_RE = re.compile(r"^    (get|post|put|patch|delete|options|head|trace):\s*$")


def main() -> int:
    in_paths = False
    current_path = None
    paths: dict[str, int] = {}
    operations: dict[tuple[str, str], int] = {}
    errors: list[str] = []
    for number, line in enumerate(SPEC.read_text(encoding="utf-8").splitlines(), 1):
        if line == "paths:":
            in_paths = True
            continue
        if in_paths and line and not line.startswith(" "):
            in_paths = False
            current_path = None
            continue
        if not in_paths:
            continue
        if line.startswith("  /") and line.rstrip().endswith(":"):
            current_path = line.strip()[:-1]
            if current_path in paths:
                errors.append(f"duplicate path {current_path!r}: lines {paths[current_path]} and {number}")
            else:
                paths[current_path] = number
            continue
        method = METHOD_RE.match(line)
        if method and current_path:
            key = (current_path, method.group(1))
            if key in operations:
                errors.append(f"duplicate operation {method.group(1).upper()} {current_path}: lines {operations[key]} and {number}")
            else:
                operations[key] = number
    if errors:
        print("OpenAPI duplicate-key check failed:", file=sys.stderr)
        print("\n".join(f"- {error}" for error in errors), file=sys.stderr)
        return 1
    print(f"OpenAPI duplicate-key check passed — {len(paths)} paths, {len(operations)} operations")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
