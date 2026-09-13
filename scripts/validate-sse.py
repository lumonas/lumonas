#!/usr/bin/env python3
"""Validate an SSE capture using the core API event contract."""

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path


def load_response_validator():
    path = Path(__file__).with_name("validate-api-response.py")
    spec = importlib.util.spec_from_file_location("lumonas_validate_api_response", path)
    if spec is None or spec.loader is None:
        raise SystemExit(f"could not load response validator: {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main(argv: list[str]) -> int:
    if len(argv) == 2 and argv[1] == "--self-test":
        validator = load_response_validator()
        validator.validate_events([{"schemaVersion": 1, "id": "evt-1", "type": "system.metrics", "timestamp": "now", "severity": "info", "data": {}}])
        print("SSE response validator self-test passed")
        return 0
    if len(argv) not in (2, 3):
        print(f"usage: {argv[0]} events.sse [expected-event-type] | --self-test", file=sys.stderr)
        return 2
    capture = Path(argv[1])
    expected_type = argv[2] if len(argv) == 3 else ""
    try:
        text = capture.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"could not read SSE capture: {exc}", file=sys.stderr)
        return 1
    if "retry: 3000" not in text:
        print("SSE capture is missing the bounded retry directive", file=sys.stderr)
        return 1
    events = []
    for line in text.splitlines():
        if not line.startswith("data: "):
            continue
        try:
            events.append(json.loads(line[6:]))
        except json.JSONDecodeError as exc:
            print(f"SSE data frame is not JSON: {exc}", file=sys.stderr)
            return 1
    if not events:
        print("SSE capture contained no data frames", file=sys.stderr)
        return 1
    validator = load_response_validator()
    try:
        validator.validate_events(events)
    except SystemExit as exc:
        print(str(exc), file=sys.stderr)
        return 1
    if expected_type and not any(event.get("type") == expected_type for event in events):
        print(f"SSE capture did not contain event type {expected_type!r}", file=sys.stderr)
        return 1
    print(f"SSE response contract ok (events={len(events)})")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
