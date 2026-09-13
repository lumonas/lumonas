#!/usr/bin/env python3
"""Normalize the hand-maintained OpenAPI path map.

Some contract additions were historically appended as a second YAML mapping
for an existing path. YAML parsers then discard earlier methods. This utility
merges path blocks by HTTP method and keeps the last definition of a repeated
operation, which makes the result safe for strict YAML/OpenAPI parsers.

Run with ``--write`` to update the supplied file. Without it, the normalized
document is written to stdout for review.
"""

from collections import OrderedDict
from pathlib import Path
import argparse
import re
import sys


PATH_RE = re.compile(r"^  (/[^:]+):\s*$")
METHOD_RE = re.compile(r"^    (get|post|put|patch|delete|options|head|trace):\s*$")


def normalize(lines: list[str]) -> list[str]:
    paths_index = next((index for index, line in enumerate(lines) if line == "paths:"), None)
    if paths_index is None:
        raise ValueError("OpenAPI document has no paths section")

    components_index = next(
        (index for index in range(paths_index + 1, len(lines)) if lines[index] == "components:"),
        len(lines),
    )
    prefix = lines[: paths_index + 1]
    path_lines = lines[paths_index + 1 : components_index]
    suffix = lines[components_index:]

    blocks: OrderedDict[str, OrderedDict[str, list[str]]] = OrderedDict()
    current_path = None
    current_method = None
    for line in path_lines:
        path_match = PATH_RE.match(line)
        if path_match:
            current_path = path_match.group(1)
            blocks.setdefault(current_path, OrderedDict())
            current_method = None
            continue
        method_match = METHOD_RE.match(line)
        if method_match and current_path:
            current_method = method_match.group(1)
            blocks[current_path][current_method] = [line]
            continue
        if current_path and current_method:
            blocks[current_path][current_method].append(line)
        elif line.strip():
            raise ValueError(f"content outside an OpenAPI path block: {line!r}")

    output = prefix
    for path, methods in blocks.items():
        output.append(f"  {path}:")
        for method_lines in methods.values():
            while method_lines and not method_lines[-1].strip():
                method_lines.pop()
            output.extend(method_lines)
        output.append("")
    while output and output[-1] == "":
        output.pop()
    output.append("")
    output.extend(suffix)
    return output


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("path", type=Path)
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    source = args.path.read_text(encoding="utf-8").splitlines()
    normalized = normalize(source)
    rendered = "\n".join(normalized) + "\n"
    if args.write:
        args.path.write_text(rendered, encoding="utf-8")
    else:
        sys.stdout.write(rendered)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
