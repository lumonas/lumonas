#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
COMPOSE_FILE="${LUMONAS_COMPOSE_FILE:-$ROOT/test/compose/compose.yaml}"
[ -f "$COMPOSE_FILE" ] || { echo "Compose fixture not found: $COMPOSE_FILE" >&2; exit 1; }

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
	docker compose -f "$COMPOSE_FILE" config --quiet
elif command -v docker-compose >/dev/null 2>&1; then
	docker-compose -f "$COMPOSE_FILE" config --quiet
else
	echo "Docker Compose v2 or docker-compose is required" >&2
	exit 1
fi

echo "Docker Compose configuration verified: $COMPOSE_FILE"
