#!/usr/bin/env python3
"""Check that every route registered by lumonasd is documented in docs/openapi.yaml.

Parses the route switch in cmd/lumonasd/main.go and the paths section of
docs/openapi.yaml (indentation-based, no external YAML dependency), then
reports routes missing from the spec. Exits non-zero when the docs drift.
"""

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MAIN_GO = ROOT / "cmd" / "lumonasd" / "main.go"
OPENAPI = ROOT / "docs" / "openapi.yaml"

CASE_RE = re.compile(
    r"case r\.Method == http\.Method(?P<method>\w+) && (?P<cond>.+?):"
)
LITERAL_RE = re.compile(r'endpoint == "(?P<endpoint>[^"]+)"')
PREFIX_RE = re.compile(r'strings\.HasPrefix\(endpoint, "(?P<prefix>[^"]+)"\)')
SUFFIX_RE = re.compile(r'strings\.HasSuffix\(endpoint, "(?P<suffix>[^"]+)"\)')
PROTOCOLS_RE = re.compile(r'strings\.TrimPrefix\(endpoint, "(?P<prefix>[^"]+)"\), "(?P<marker>[^"]+)/"')

METHODS = {"Get": "get", "Post": "post", "Patch": "patch", "Put": "put", "Delete": "delete"}

# Handler-level overrides for prefix routes whose parameter shape differs from
# the generic "{id}" tail (multi-segment or action suffixes), keyed by
# (method, prefix).
SPECIAL_PREFIX = {
    ("post", "/docker/stacks/"): "/docker/stacks/{id}/{action}",
    ("post", "/docker/containers/"): "/docker/containers/{id}/{action}",
    ("post", "/docker/images/"): "/docker/images/{id}/update",
    ("post", "/network/checkpoints/"): "/network/checkpoints/{id}/{action}",
    ("get", "/docker/logs/"): "/docker/logs/{container}",
    ("patch", "/alerts/"): "/alerts/{id}/ack",
}

# Routes registered outside the /api/v1 switch (health endpoints).
WHITELIST = {
    ("get", "/healthz"),
    ("get", "/readyz"),
}


def join_path(prefix: str, tail: str) -> str:
    if prefix.endswith("/") and tail.startswith("/"):
        return prefix + "{id}/" + tail[1:]
    if not tail.startswith("/"):
        return prefix + "{id}/" + tail
    return prefix + "{id}" + tail


def routes_from_go(source: str) -> set[tuple[str, str]]:
    routes: set[tuple[str, str]] = set()
    for match in CASE_RE.finditer(source):
        method = METHODS.get(match.group("method"))
        condition = match.group("cond")
        if method is None:
            continue
        literal = LITERAL_RE.search(condition)
        prefix = PREFIX_RE.search(condition)
        suffix = SUFFIX_RE.search(condition)
        protocols = PROTOCOLS_RE.search(condition)
        if literal:
            routes.add((method, literal.group("endpoint")))
        elif protocols and prefix:
            marker = protocols.group("marker").lstrip("/")
            routes.add((method, f"{prefix.group('prefix')}{{id}}/{marker}/{{protocol}}"))
        elif prefix and (method, prefix.group("prefix")) in SPECIAL_PREFIX:
            routes.add((method, SPECIAL_PREFIX[(method, prefix.group("prefix"))]))
        elif prefix and suffix:
            routes.add((method, join_path(prefix.group("prefix"), suffix.group("suffix"))))
        elif prefix:
            routes.add((method, f"{prefix.group('prefix')}{{id}}"))
    return routes


def paths_from_openapi(source: str) -> set[tuple[str, str]]:
    routes: set[tuple[str, str]] = set()
    in_paths = False
    current_path = None
    for line in source.splitlines():
        if not in_paths:
            if line.rstrip() == "paths:":
                in_paths = True
            continue
        if line.startswith("  /") and line.rstrip().endswith(":"):
            current_path = line.strip().rstrip(":")
            continue
        if line.startswith("components:") or (line and not line.startswith(" ") and not line.startswith("  /")):
            in_paths = False
            current_path = None
            continue
        method_match = re.match(r"^    (\w+):\s*$", line)
        if method_match and current_path:
            method = method_match.group(1)
            if method in {"get", "post", "patch", "put", "delete"}:
                routes.add((method, current_path))
    return routes


def main() -> int:
    implemented = routes_from_go(MAIN_GO.read_text())
    documented = paths_from_openapi(OPENAPI.read_text())
    missing = sorted((implemented - documented) - WHITELIST)
    extra = sorted((documented - implemented) - WHITELIST)
    if missing:
        print(f"missing from {OPENAPI.name} ({len(missing)}):")
        for method, path in missing:
            print(f"  {method.upper():6} {path}")
    if extra:
        print(f"documented but not registered in main.go ({len(extra)}):")
        for method, path in extra:
            print(f"  {method.upper():6} {path}")
    if not missing and not extra:
        print(f"openapi parity ok — {len(documented)} documented paths match the router")
        return 0
    return 1


if __name__ == "__main__":
    sys.exit(main())
