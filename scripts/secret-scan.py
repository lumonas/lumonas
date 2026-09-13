#!/usr/bin/env python3
"""Scan tracked repository files for high-confidence credential formats."""

from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys


PATTERNS = (
    ("private key block", re.compile(rb"-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----\s*(?:[A-Za-z0-9+/]{32,}={0,2}\s*)+-----END (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----")),
    ("AWS access key", re.compile(rb"\bAKIA[0-9A-Z]{16}\b")),
    ("GitHub token", re.compile(rb"\b(?:gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,})\b")),
    ("Slack token", re.compile(rb"\bxox[baprs]-[0-9A-Za-z-]{20,}\b")),
    ("Google API key", re.compile(rb"\bAIza[0-9A-Za-z_-]{35}\b")),
    ("Stripe secret key", re.compile(rb"\bsk_(?:live|test)_[0-9A-Za-z]{16,}\b")),
    ("npm token", re.compile(rb"\bnpm_[A-Za-z0-9]{36}\b")),
    ("credential URL", re.compile(rb"https?://[^/\s:@]+:[^@\s/]+@")),
)

def tracked_files(root: pathlib.Path) -> list[pathlib.Path]:
    result = subprocess.run(
        ["git", "-C", str(root), "ls-files", "-z"],
        check=True,
        stdout=subprocess.PIPE,
    )
    return [root / item for item in result.stdout.decode("utf-8").split("\0") if item]


def scan(paths: list[pathlib.Path], root: pathlib.Path) -> list[str]:
    findings: list[str] = []
    for path in paths:
        try:
            data = path.read_bytes()
        except OSError as error:
            findings.append(f"{path}: cannot read file: {error}")
            continue
        if b"\0" in data:
            continue
        relative = path.relative_to(root).as_posix() if path.is_relative_to(root) else str(path)
        for label, pattern in PATTERNS:
            match = pattern.search(data)
            if match:
                line = data.count(b"\n", 0, match.start()) + 1
                findings.append(f"{relative}:{line}: {label}")
    return findings


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=pathlib.Path, default=pathlib.Path.cwd())
    parser.add_argument("--paths", nargs="*", type=pathlib.Path)
    args = parser.parse_args()
    root = args.root.resolve()
    paths = args.paths
    if paths is None:
        paths = tracked_files(root)
    else:
        paths = [(item if item.is_absolute() else root / item).resolve() for item in paths]
    findings = scan(paths, root)
    if findings:
        print("high-confidence credential material found:", file=sys.stderr)
        print("\n".join(findings), file=sys.stderr)
        return 1
    print(f"secret scan passed ({len(paths)} files checked)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
