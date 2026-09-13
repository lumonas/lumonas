#!/usr/bin/env python3
"""Validate the runtime shape of the core API responses.

The OpenAPI parity checks catch route and type drift in source. This checker is
used by the black-box API smoke test to catch a daemon that starts successfully
but emits an incompatible response at runtime.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


def fail(message: str) -> None:
    raise SystemExit(message)


def object_value(value: Any, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        fail(f"{label} must be a JSON object")
    return value


def required(obj: dict[str, Any], fields: tuple[str, ...], label: str) -> None:
    missing = [field for field in fields if field not in obj]
    if missing:
        fail(f"{label} is missing fields: {', '.join(missing)}")


def string_field(obj: dict[str, Any], field: str, label: str) -> None:
    if not isinstance(obj.get(field), str):
        fail(f"{label}.{field} must be a string")


def number_field(obj: dict[str, Any], field: str, label: str) -> None:
    if not isinstance(obj.get(field), (int, float)) or isinstance(obj[field], bool):
        fail(f"{label}.{field} must be a number")


def validate_server(value: Any) -> None:
    obj = object_value(value, "server")
    fields = ("id", "name", "hostname", "version", "nasUuid", "timezone", "health", "ip")
    required(obj, fields, "server")
    for field in fields:
        string_field(obj, field, "server")


def validate_disks(value: Any) -> None:
    if not isinstance(value, list):
        fail("disks must be a JSON array")
    fields = (
        "id", "name", "model", "sizeBytes", "role", "rotational",
        "interface", "health", "temperatureC", "mounted", "lastSeen", "smart",
    )
    stable_identity = ("serial", "wwn", "gptDiskGuid", "partitionUuid", "filesystemUuid")
    for index, disk in enumerate(value):
        label = f"disks[{index}]"
        obj = object_value(disk, label)
        required(obj, fields, label)
        for field in ("id", "name", "model", "role", "interface", "health", "lastSeen"):
            string_field(obj, field, label)
        if not any(obj.get(field) for field in stable_identity):
            fail(f"{label} has no stable identity field")
        number_field(obj, "sizeBytes", label)
        if obj["temperatureC"] is not None:
            number_field(obj, "temperatureC", label)
        if not isinstance(obj["rotational"], bool) or not isinstance(obj["mounted"], bool):
            fail(f"{label}.rotational and mounted must be booleans")
        smart = object_value(obj["smart"], f"{label}.smart")
        required(smart, ("overall", "reallocatedSectors", "pendingSectors", "uncorrectableSectors", "crcErrors", "powerOnHours"), f"{label}.smart")


def validate_metrics(value: Any) -> None:
    obj = object_value(value, "metrics")
    required(obj, ("cpuPercent", "load", "ramUsedBytes", "ramTotalBytes", "cpuTempC", "uptimeSeconds", "net"), "metrics")
    for field in ("cpuPercent", "cpuTempC"):
        number_field(obj, field, "metrics")
    for field in ("ramUsedBytes", "ramTotalBytes", "uptimeSeconds"):
        if not isinstance(obj.get(field), int) or isinstance(obj[field], bool) or obj[field] < 0:
            fail(f"metrics.{field} must be a non-negative integer")
    if not isinstance(obj["load"], list) or len(obj["load"]) != 3:
        fail("metrics.load must contain exactly three numbers")
    for index, item in enumerate(obj["load"]):
        if not isinstance(item, (int, float)) or isinstance(item, bool):
            fail(f"metrics.load[{index}] must be a number")
    net = object_value(obj["net"], "metrics.net")
    required(net, ("interface", "upMbps", "downMbps"), "metrics.net")
    string_field(net, "interface", "metrics.net")
    number_field(net, "upMbps", "metrics.net")
    number_field(net, "downMbps", "metrics.net")


def validate_metrics_history(value: Any) -> None:
    if not isinstance(value, list):
        fail("metrics-history must be a JSON array")
    previous = None
    for index, sample in enumerate(value):
        label = f"metrics-history[{index}]"
        obj = object_value(sample, label)
        required(obj, ("capturedAt", "metrics"), label)
        string_field(obj, "capturedAt", label)
        validate_metrics(obj["metrics"])
        if previous is not None and obj["capturedAt"] > previous:
            fail("metrics-history must be ordered newest first")
        previous = obj["capturedAt"]


def validate_jobs(value: Any) -> None:
    if not isinstance(value, list):
        fail("jobs must be a JSON array")
    fields = ("id", "type", "title", "state", "progress", "createdAt")
    for index, job in enumerate(value):
        label = f"jobs[{index}]"
        obj = object_value(job, label)
        required(obj, fields, label)
        for field in ("id", "type", "title", "state", "createdAt"):
            string_field(obj, field, label)
        if obj["progress"] is not None and (not isinstance(obj["progress"], (int, float)) or isinstance(obj["progress"], bool)):
            fail(f"{label}.progress must be a number or null")


def validate_docker_summary(value: Any) -> None:
    obj = object_value(value, "docker-summary")
    required(obj, ("available", "stacks", "appsRunning", "updatesAvailable"), "docker-summary")
    if not isinstance(obj["available"], bool):
        fail("docker-summary.available must be a boolean")
    for field in ("stacks", "appsRunning", "updatesAvailable"):
        if not isinstance(obj[field], int) or isinstance(obj[field], bool) or obj[field] < 0:
            fail(f"docker-summary.{field} must be a non-negative integer")


def validate_docker_containers(value: Any) -> None:
    if not isinstance(value, list):
        fail("docker-containers must be a JSON array")
    fields = ("id", "name", "image", "state", "cpuPercent", "ramUsedBytes", "restarts", "ports")
    for index, container in enumerate(value):
        label = f"docker-containers[{index}]"
        obj = object_value(container, label)
        required(obj, fields, label)
        for field in ("id", "name", "image", "state"):
            string_field(obj, field, label)
        number_field(obj, "cpuPercent", label)
        for field in ("ramUsedBytes", "restarts"):
            if not isinstance(obj[field], int) or isinstance(obj[field], bool) or obj[field] < 0:
                fail(f"{label}.{field} must be a non-negative integer")
        if not isinstance(obj["ports"], list):
            fail(f"{label}.ports must be a JSON array")


def validate_docker_images(value: Any) -> None:
    if not isinstance(value, list):
        fail("docker-images must be a JSON array")
    fields = ("id", "repo", "tag", "sizeBytes", "createdDaysAgo", "updateAvailable", "inUse")
    for index, image in enumerate(value):
        label = f"docker-images[{index}]"
        obj = object_value(image, label)
        required(obj, fields, label)
        for field in ("id", "repo", "tag"):
            string_field(obj, field, label)
        for field in ("sizeBytes", "createdDaysAgo"):
            if not isinstance(obj[field], int) or isinstance(obj[field], bool) or obj[field] < 0:
                fail(f"{label}.{field} must be a non-negative integer")
        for field in ("updateAvailable", "inUse"):
            if not isinstance(obj[field], bool):
                fail(f"{label}.{field} must be a boolean")


def validate_docker_volumes(value: Any) -> None:
    if not isinstance(value, list):
        fail("docker-volumes must be a JSON array")
    fields = ("id", "name", "usedBytes")
    for index, volume in enumerate(value):
        label = f"docker-volumes[{index}]"
        obj = object_value(volume, label)
        required(obj, fields, label)
        for field in ("id", "name"):
            string_field(obj, field, label)
        if not isinstance(obj["usedBytes"], int) or isinstance(obj["usedBytes"], bool) or obj["usedBytes"] < 0:
            fail(f"{label}.usedBytes must be a non-negative integer")


def validate_events(value: Any) -> None:
    if not isinstance(value, list):
        fail("events must be a JSON array")
    for index, event in enumerate(value):
        label = f"events[{index}]"
        obj = object_value(event, label)
        required(obj, ("schemaVersion", "id", "type", "timestamp", "severity", "data"), label)
        if not isinstance(obj["schemaVersion"], int) or obj["schemaVersion"] < 1:
            fail(f"{label}.schemaVersion must be a positive integer")
        for field in ("id", "type", "timestamp", "severity"):
            string_field(obj, field, label)
        object_value(obj["data"], f"{label}.data")


def self_test() -> None:
    validate_server({field: "value" for field in ("id", "name", "hostname", "version", "nasUuid", "timezone", "health", "ip")})
    validate_disks([{
        "id": "wwn-123", "name": "sda", "model": "virtual", "gptDiskGuid": "guid-1",
        "sizeBytes": 1024, "role": "data", "rotational": False, "interface": "virtio",
        "health": "healthy", "temperatureC": None, "mounted": False, "lastSeen": "now",
        "smart": {"overall": "healthy", "reallocatedSectors": 0, "pendingSectors": 0,
                   "uncorrectableSectors": 0, "crcErrors": 0, "powerOnHours": 0},
    }])
    validate_metrics({
        "cpuPercent": 1.0, "load": [0.0, 0.0, 0.0], "ramUsedBytes": 1,
        "ramTotalBytes": 2, "cpuTempC": 0.0, "uptimeSeconds": 1,
        "net": {"interface": "", "upMbps": 0.0, "downMbps": 0.0},
    })
    validate_metrics_history([{
        "capturedAt": "2026-01-02T00:00:00Z",
        "metrics": {
            "cpuPercent": 1.0, "load": [0.0, 0.0, 0.0], "ramUsedBytes": 1,
            "ramTotalBytes": 2, "cpuTempC": 0.0, "uptimeSeconds": 1,
            "net": {"interface": "", "upMbps": 0.0, "downMbps": 0.0},
        },
    }])
    validate_jobs([{"id": "job-1", "type": "smart.short", "title": "SMART", "state": "queued", "progress": None, "createdAt": "now"}])
    validate_docker_summary({"available": True, "stacks": 0, "appsRunning": 0, "updatesAvailable": 0})
    validate_docker_containers([])
    validate_docker_images([])
    validate_docker_volumes([])
    validate_events([{"schemaVersion": 1, "id": "evt-1", "type": "system.metrics", "timestamp": "now", "severity": "info", "data": {}}])


VALIDATORS = {
    "server": validate_server,
    "disks": validate_disks,
    "metrics": validate_metrics,
    "metrics-history": validate_metrics_history,
    "jobs": validate_jobs,
    "docker-summary": validate_docker_summary,
    "docker-containers": validate_docker_containers,
    "docker-images": validate_docker_images,
    "docker-volumes": validate_docker_volumes,
    "events": validate_events,
}


def main(argv: list[str]) -> int:
    if len(argv) == 2 and argv[1] == "--self-test":
        self_test()
        print("API response validator self-test passed")
        return 0
    if len(argv) != 3 or argv[1] not in VALIDATORS:
        choices = ", ".join(sorted(VALIDATORS))
        print(f"usage: {argv[0]} <{choices}> response.json | --self-test", file=sys.stderr)
        return 2
    kind, filename = argv[1], Path(argv[2])
    try:
        value = json.loads(filename.read_text(encoding="utf-8"))
        VALIDATORS[kind](value)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"{kind}: invalid JSON response: {exc}", file=sys.stderr)
        return 1
    except SystemExit as exc:
        print(str(exc), file=sys.stderr)
        return 1
    print(f"{kind} response contract ok")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
