#!/usr/bin/env python3
"""Check the fields shared by the backend, OpenAPI, and frontend contracts."""

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OPENAPI = ROOT / "docs" / "openapi.yaml"
TYPES = ROOT / "web" / "src" / "api" / "types.ts"


def between(source: str, start: str, end: str) -> str:
    start_at = source.find(start)
    if start_at < 0:
        raise SystemExit(f"missing contract section: {start}")
    end_at = source.find(end, start_at + len(start))
    if end_at < 0:
        raise SystemExit(f"missing contract section terminator: {end}")
    return source[start_at:end_at]


def check_fields(label: str, section: str, fields: list[str], pattern: str) -> None:
    missing = [field for field in fields if not re.search(pattern.format(field=re.escape(field)), section, re.MULTILINE)]
    if missing:
        raise SystemExit(f"{label} is missing fields: {', '.join(missing)}")


def main() -> int:
    openapi = OPENAPI.read_text(encoding="utf-8")
    types = TYPES.read_text(encoding="utf-8")

    disk_fields = [
        "id", "name", "currentPath", "model", "serial", "wwn", "gptDiskGuid",
        "partitionUuid", "filesystemUuid", "sizeBytes", "usedBytes", "role",
        "rotational", "interface", "health", "temperatureC", "filesystem",
        "mounted", "poolId", "standby", "lastSeen", "smart",
    ]
    event_fields = [
        "schemaVersion", "id", "type", "timestamp", "severity", "correlationId",
        "operationId", "planHash", "actor", "generation", "resource", "data",
    ]

    disk_schema = between(openapi, "    Disk:\n", "    Pool:\n")
    disk_type = between(types, "export interface Disk {", "export interface PoolMember {")
    event_schema = between(openapi, "    Event:\n", "    ResourceRef:\n")
    event_type = between(types, "export interface LumoEvent", "export type IPMethod")

    check_fields("OpenAPI Disk", disk_schema, disk_fields, r"^\s+{field}:\s*$")
    check_fields("frontend Disk", disk_type, disk_fields, r"^\s+{field}\??\s*[:(]")
    check_fields("OpenAPI Event", event_schema, event_fields, r"^\s+{field}:\s*$")
    check_fields("frontend LumoEvent", event_type, event_fields, r"^\s+{field}\??\s*[:(]")

    print("API contract parity ok — Disk and LumoEvent fields match OpenAPI")
    return 0


if __name__ == "__main__":
    sys.exit(main())
