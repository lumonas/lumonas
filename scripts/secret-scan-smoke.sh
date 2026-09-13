#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/lumonas-secret-scan.XXXXXX")"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

python3 "$ROOT/scripts/secret-scan.py" --root "$ROOT"
printf '%s\n' 'AKIAIOSFODNN7EXAMPLE' >"$WORK/aws.txt"
if python3 "$ROOT/scripts/secret-scan.py" --root "$ROOT" --paths "$WORK/aws.txt"; then
	echo "secret scanner accepted a synthetic AWS credential" >&2
	exit 1
fi
printf '%s\n' 'https://user:password@example.test/hook' >"$WORK/url.txt"
if python3 "$ROOT/scripts/secret-scan.py" --root "$ROOT" --paths "$WORK/url.txt"; then
	echo "secret scanner accepted a credential URL" >&2
	exit 1
fi
printf '%s\n' 'https://example.test/hook' >"$WORK/safe.txt"
python3 "$ROOT/scripts/secret-scan.py" --root "$ROOT" --paths "$WORK/safe.txt"
echo "LumoNAS secret scan smoke test passed"
