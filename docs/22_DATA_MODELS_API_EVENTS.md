# Suggested Data Models, API and Events

This is a starting model, not a frozen database schema.

Jobs, events, and audit entries expose first-class `correlationId`,
`operationId`, `planHash`, `actor`, and `generation` fields where applicable.
Request-created jobs retain the authenticated actor through every queued state
transition and chained recovery job; scheduled and daemon-created work uses
`system`. Resource type and ID remain
explicit fields; the original metadata/data payloads are retained for backward
compatibility.

## Core entities

### Server

```text
id
nas_uuid
name
hostname
timezone
version
config_generation
health
```

### Disk

```text
id
stable_fingerprint
wwn
serial
gpt_disk_guid
model
size_bytes
current_device_path
role
rotational
health
temperature
last_seen
metadata_generation
```

### Filesystem

```text
id
disk_id
partition_uuid
filesystem_uuid
type
label
mount_resource_id
```

### Pool

```text
id
name
type=mergerfs
mount_path
status
policy
```

### PoolMember

```text
pool_id
filesystem_id
enabled
```

### SnapraidArray

```text
id
name
last_sync
last_scrub
status
policy
```

### Share

```text
id
name
storage_resource_id
relative_path
description
status
```

### ShareProtocol

```text
share_id
protocol
enabled
settings_json
```

### Principal

```text
id
type=user|group|service
name
uid/gid
management_role
```

### AccessRule

```text
resource_id
principal_id
level=none|read|write
```

### DockerStack

```text
id
name
source_type
compose_path
compose_hash
catalog_id
status
backup_policy_id
last_deploy
```

### DockerDeployment

```text
id
stack_name
kind=install|update|rollback
state=pending|committed|rolled_back|failed
compose_before
compose_after
error
created_at
updated_at
image_before_json (internal recovery field)
```

Docker stack installation and updates are recorded as transactions. The
daemon reconciles pending transactions during startup: interrupted installs
are stopped and interrupted updates restore the last known Compose content.
Cleanup failures remain `failed` so the UI cannot report an unsafe rollback as
successful. Terminal history is bounded by operational retention; pending
transactions are retained until reconciliation.

### Variable

```text
id
scope
name
kind=text|storage_ref|secret_ref
value/reference
```

### Secret

```text
id
name
encrypted_blob
created_at
rotated_at
```

### AlertRule

```text
id
name
condition
pending_duration
severity
routes
enabled
```

### Alert

```text
id
rule_id
resource_id
state
started
resolved
acknowledged_by
```

### Job

```text
id
type
resource_id
state
progress
stage
created
started
finished
error
```

### ConfigGeneration

```text
generation
created
actor
status
parent_generation
summary
snapshot_ref
```

### AuditEvent

```text
id
timestamp
actor
action
resource
operation_id
generation
metadata
```

## API style

Version:

```text
/api/v1/
```

Examples:

```text
GET  /disks
GET  /disks/{id}
POST /disks/{id}/smart-tests

GET  /pools
POST /pools

GET  /shares
POST /shares
PATCH /shares/{id}

GET  /docker/stacks
POST /docker/stacks
GET  /docker/deployments?stack=<name>&limit=<n>
POST /docker/stacks/{id}/deploy

GET  /network/interfaces
POST /network/checkpoints
PATCH /network/connections/{id}

GET  /jobs/{id}
GET  /events/stream
GET  /events                 (compatibility alias)
```

Dangerous API never accepts `/dev/sdb` as the only target identity.

## Mutation planning API

Pattern:

```text
POST /operations/plan
→ plan preview + plan_id + hash

POST /operations/{plan_id}/confirm
→ privileged job
```

Plan response contains dependencies and warnings.

When the daemon restarts, any queued, preparing, or running job is failed
closed and emits one persisted `job.state_changed` event with
`reason: "daemon_restart"`. Clients reconnecting with `Last-Event-ID` can
therefore observe the same terminal transition as clients that were online.

## Realtime event envelope

```json
{
  "id": "...",
  "type": "docker.container.state_changed",
  "timestamp": "...",
  "resource": {"type": "container", "id": "..."},
  "severity": "info",
  "data": {}
}
```

## Event taxonomy

### Storage

- disk.added
- disk.removed
- disk.temperature.changed
- disk.smart.warning
- filesystem.mounted
- pool.degraded
- storage.safety_mode.entered

### SnapRAID

- snapraid.sync.started/progress/completed/failed
- snapraid.scrub.*
- snapraid.protection.stale

### Docker

- docker.stack.deployed
- docker.container.started/stopped/unhealthy
- docker.image.update_available
- docker.backup.*

### Network

- network.link.up/down
- network.link.degraded
- network.config.changed
- network.rollback
- network.internet.unavailable

### Security

- auth.login.success/failed
- auth.session.revoked
- security.recommendation.created

### Recovery

- recovery.backup.created/verified/failed
- recovery.readiness.changed
- recovery.restore.*

Job history is bounded in SQLite: all queued/running jobs remain available,
while only the newest 1,000 terminal jobs (`completed`, `failed`, or
`canceled`) are retained.

## Idempotency

Mutating HTTP operations should accept/request idempotency keys where repeated browser/API submission could duplicate a job.

## Concurrency

Use optimistic generation checks:

```text
expected_generation
```

Reject stale mutations and ask client to refresh/replan.

This is especially important for destructive plans.
