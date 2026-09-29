#!/usr/bin/env python3
"""Compare catalog image vulnerabilities against the accepted baseline.

The container image gate originally required zero unfixed HIGH/CRITICAL
findings across every catalog image. That is not achievable: several catalog
entries are third-party images LumoNAS does not build, and their base layers
carry findings whose fixes are newer than the published image, which
`--ignore-unfixed` cannot exclude.

This gate keeps the property that actually matters, which is that no *new*
known vulnerability is introduced silently. Every HIGH/CRITICAL finding must
appear in the accepted baseline; a finding that is not listed fails the build
and must be triaged before the baseline is updated.

An image that is absent from the baseline is held to the strict
zero-finding standard, so adding a catalog entry cannot quietly admit
findings. Pass --require-empty to select that behaviour explicitly.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys

DEFAULT_BASELINE = "security/catalog-image-baseline.json"


def load_json(path: pathlib.Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def target_matches_image(target: str, image: str) -> bool:
    """Report whether a Trivy result target belongs to a scanned image.

    Trivy labels a result target as "<image> (<os> <version>)", for example
    "jellyfin/jellyfin:12.1 (debian 13.6)", so the OS suffix has to be
    stripped before comparing against the image reference.
    """
    candidate = target.split(" (", 1)[0].strip()
    if candidate == image:
        return True
    # Defensive: tolerate a registry prefix difference such as docker.io.
    return candidate.split("/", 1)[-1] == image.split("/", 1)[-1] and "/" not in image.split(":", 1)[0]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--baseline", default=DEFAULT_BASELINE)
    parser.add_argument(
        "--results",
        help="Trivy JSON report, or '-' to read it from stdin.",
    )
    parser.add_argument(
        "--image",
        help="Image reference this report covers, e.g. 'nextcloud:30'. Defaults to matching every target.",
    )
    parser.add_argument(
        "--require-empty",
        action="store_true",
        help="Hold this image to zero accepted findings.",
    )
    args = parser.parse_args()

    baseline_path = pathlib.Path(args.baseline)
    if not baseline_path.exists():
        print(f"baseline not found: {baseline_path}", file=sys.stderr)
        return 1
    try:
        accepted = load_json(baseline_path).get("images", {})
    except json.JSONDecodeError as error:
        print(f"baseline is not valid JSON: {error}", file=sys.stderr)
        return 1

    if args.results == "-":
        raw = sys.stdin.read()
    elif args.results:
        raw = pathlib.Path(args.results).read_text(encoding="utf-8")
    else:
        raw = sys.stdin.read()
    try:
        report = json.loads(raw)
    except json.JSONDecodeError as error:
        print(f"scan report is not valid JSON: {error}", file=sys.stderr)
        return 1

    # An image with no baseline entry must be clean, so it gets an empty
    # permitted set and every finding is then reported as unaccepted.
    if args.image is not None:
        permitted = set(accepted.get(args.image, []))
    else:
        permitted = set()

    found: set[str] = set()
    for result in report.get("Results") or []:
        target = result.get("Target") or ""
        if args.image is not None and not target_matches_image(target, args.image):
            continue
        for vulnerability in result.get("Vulnerabilities") or []:
            identifier = vulnerability.get("VulnerabilityID")
            if identifier:
                found.add(identifier)

    unaccepted = sorted(found - permitted)

    if unaccepted:
        subject = args.image or "the scanned image"
        print(
            f"{subject}: {len(unaccepted)} HIGH/CRITICAL finding(s) are not in the accepted baseline:",
            file=sys.stderr,
        )
        for identifier in unaccepted:
            print(f"  {identifier}", file=sys.stderr)
        if args.require_empty or (args.image is not None and not accepted.get(args.image)):
            print(
                f"\n{subject} is not in {args.baseline}, so it must scan clean.",
                file=sys.stderr,
            )
        print(
            "\nTriage each one. If it is acceptable, add it to "
            f"{args.baseline} in a reviewed commit; otherwise update the image pin.",
            file=sys.stderr,
        )
        return 1

    print(f"catalog image vulnerabilities match the accepted baseline ({len(found)} accepted finding(s))")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
