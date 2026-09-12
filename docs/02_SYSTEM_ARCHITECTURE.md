# System Architecture

## Recommended runtime stack

### Base OS

- Debian 13 Stable (Trixie) as the September 2026 baseline.
- Minimal package selection.
- systemd.
- NetworkManager.
- nftables.
- Docker Engine + Compose v2.
- SQLite for LumoNAS state.
- No Node.js runtime required after frontend build.

### Backend

Use Go for system services:

- low idle memory;
- simple static deployment;
- concurrency suits event collection;
- Docker SDK/API support;
- easy systemd service integration.

### Frontend

- React;
- TypeScript;
- Vite;
- Tailwind CSS;
- shadcn/ui/Radix primitives;
- Lucide icons;
- TanStack Query;
- small client store for live resource state.

Build to static assets and embed/serve through `lumonas-web`.

## Process architecture

```text
Browser
   │
   ├─ REST
   ├─ SSE
   └─ WebSocket (terminal only)
   ▼
lumonas-web
unprivileged HTTP/UI service
   │
   ▼
lumonasd
domain logic / desired state / events
mostly unprivileged
   │
   ├── read collectors
   ├── Docker control module
   ├── configuration store
   ├── job coordinator
   ├── alert engine
   └── mutation planner
   │
   ▼
lumonas-privd
small root service
structured, allow-listed operations
```

## Why three services

`lumonas-web` should not be root.

`lumonasd` should own product state and business rules but should not contain a generic privileged shell.

`lumonas-privd` should remain intentionally small, auditable, and boring.

If the web server is compromised, the attacker must still cross another strongly constrained boundary before performing destructive disk operations.

## IPC

Recommended:

- Unix domain socket between `lumonasd` and `lumonas-privd`;
- peer credential verification;
- protobuf/gRPC or a small framed typed protocol;
- no TCP listener for `lumonas-privd`;
- Linux peer credentials (`SO_PEERCRED`) are checked in addition to socket mode;
- requests include operation ID and immutable mutation plan hash.

## Desired-state configuration

Store canonical LumoNAS intent in SQLite and version it by generation.

Examples:

- share `Documents` should exist at resource path X;
- interface profile Y should use static IP;
- stack `Immich` should use Compose file version/hash Z;
- SnapRAID should protect disk identities A/B/C.

Apply process:

```text
validate proposed state
→ create new generation
→ plan mutations
→ snapshot previous generation
→ apply
→ health-check
→ commit
```

If validation/application fails, mark generation failed and revert what is safely reversible.

## Event architecture

All major subsystems emit typed events into an internal bus.

Examples:

- `disk.temperature.changed`
- `disk.disconnected`
- `docker.container.unhealthy`
- `snapraid.sync.completed`
- `network.link.degraded`
- `backup.failed`
- `config.generation.committed`

Consumers:

- SSE UI stream;
- alert engine;
- audit log;
- metrics/history;
- automation rules.

Use bounded queues and persistent event storage only for meaningful events; do not persist every 1-second metric tick.

## Background jobs

Implement a persistent job table in SQLite.

Job lifecycle:

- queued;
- preparing;
- running;
- waiting-confirmation;
- successful;
- failed;
- cancelled.

Examples:

- SMART test;
- SnapRAID sync/scrub;
- file copy;
- app backup;
- Docker image pull;
- storage migration;
- config restore.

Long jobs survive UI navigation and restart where safely possible.

## Configuration files

Prefer generated fragments owned by LumoNAS rather than taking ownership of entire system config when possible.

Examples:

```text
/etc/samba/smb.conf
/etc/samba/conf.d/lumonas.conf

/etc/nftables.d/lumonas.nft
/etc/systemd/system/...
```

Before reload:

- render to temporary file;
- syntax validate;
- atomically replace;
- reload;
- health-check.

## Runtime paths

Suggested:

```text
/var/lib/lumonas/
├── lumonas.db
├── generations/
├── recovery/
├── secrets/
├── jobs/
└── state/

/srv/lumonas/
└── docker/
    ├── stacks/
    ├── templates/
    └── exports/

/srv/disks/<stable-logical-id>/
/srv/pools/<pool-name>/
```

Do not use `/dev/sdX` in persisted configuration.

## Resource usage target

The management layer should remain lightweight:

- no Prometheus/Grafana by default;
- no Redis;
- no message broker;
- no Node runtime;
- bounded telemetry retention;
- single SQLite state database with careful write behavior.

Docker applications remain the dominant optional resource consumers, not the NAS UI itself.
