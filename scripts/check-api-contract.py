#!/usr/bin/env python3
"""Check the fields shared by the backend, OpenAPI, and frontend contracts."""

import re
import sys
import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OPENAPI = ROOT / "docs" / "openapi.yaml"
TYPES = ROOT / "web" / "src" / "api" / "types.ts"
QUERIES = ROOT / "web" / "src" / "api" / "queries.ts"
OPENAPI_CHECK = ROOT / "scripts" / "check-openapi.py"

FRONTEND_CALL_RE = re.compile(
    r"\bapi(?P<method>Get|Post|Patch|Put|Delete|Multipart)"
    r"(?:<[^>\n]*>)?\(\s*"
    r"(?:'(?P<single>[^'\n]*)'|\"(?P<double>[^\"\n]*)\"|`(?P<template>[^`\n]*)`)"
)
FRONTEND_METHODS = {
    "Get": "get",
    "Post": "post",
    "Patch": "patch",
    "Put": "put",
    "Delete": "delete",
    "Multipart": "post",
}
IGNORED_GENERIC_NAMES = {"Record"}


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


def frontend_routes(source: str) -> set[tuple[str, str]]:
    routes: set[tuple[str, str]] = set()
    for match in FRONTEND_CALL_RE.finditer(source):
        path = next(value for value in (match.group("single"), match.group("double"), match.group("template")) if value is not None)
        path = path.split("?", 1)[0]
        path = re.sub(r"\$\{[^}]+\}", "{param}", path)
        routes.add((FRONTEND_METHODS[match.group("method")], path))
    return routes


def frontend_response_types(source: str) -> set[str]:
    """Return named response types used by frontend API calls."""
    names: set[str] = set()
    for match in re.finditer(r"\bapi(?:Get|Post|Patch|Put|Delete|Multipart)<([^>\n]+)>", source):
        for name in re.findall(r"\b[A-Z][A-Za-z0-9_]*\b", match.group(1)):
            if name not in IGNORED_GENERIC_NAMES:
                names.add(name)
    return names


def declared_types(source: str) -> set[str]:
    return set(re.findall(r"\b(?:export\s+)?(?:interface|type)\s+([A-Z][A-Za-z0-9_]*)", source))


def openapi_schemas(source: str) -> set[str]:
    return set(re.findall(r"^    ([A-Z][A-Za-z0-9_]*):$", source, re.MULTILINE))


def route_matches(pattern: str, actual: str) -> bool:
    pattern_parts = [part for part in pattern.strip("/").split("/") if part]
    actual_parts = [part for part in actual.strip("/").split("/") if part]
    if len(pattern_parts) != len(actual_parts):
        return False
    return all(
        expected.startswith("{") and expected.endswith("}") or expected == received
        for expected, received in zip(pattern_parts, actual_parts)
    )


def main() -> int:
    openapi = OPENAPI.read_text(encoding="utf-8")
    types = TYPES.read_text(encoding="utf-8")
    queries = QUERIES.read_text(encoding="utf-8")

    response_types = frontend_response_types(queries)
    missing_frontend_types = sorted(response_types - declared_types(types + "\n" + queries))
    if missing_frontend_types:
        raise SystemExit("frontend response types are not declared: " + ", ".join(missing_frontend_types))
    missing_openapi_schemas = sorted(response_types - openapi_schemas(openapi))
    if missing_openapi_schemas:
        raise SystemExit("frontend response types are missing from OpenAPI schemas: " + ", ".join(missing_openapi_schemas))

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

    spec = importlib.util.spec_from_file_location("lumonas_check_openapi", OPENAPI_CHECK)
    if spec is None or spec.loader is None:
        raise SystemExit(f"could not load OpenAPI route checker: {OPENAPI_CHECK}")
    openapi_checker = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(openapi_checker)
    documented_routes = openapi_checker.paths_from_openapi(openapi)
    missing_routes = sorted(
        (method, path)
        for method, path in frontend_routes(queries)
        if not any(route_matches(documented_path, path) and documented_method == method for documented_method, documented_path in documented_routes)
    )
    if missing_routes:
        details = ", ".join(f"{method.upper()} {path}" for method, path in missing_routes)
        raise SystemExit(f"frontend API calls are missing from OpenAPI/router contract: {details}")

    print(f"API contract parity ok — {len(response_types)} named response types, Disk/Event fields, and {len(frontend_routes(queries))} frontend API routes match OpenAPI")
    return 0


if __name__ == "__main__":
    sys.exit(main())
