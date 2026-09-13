# Monitoring, Alerts, Notifications and Jobs

## Monitoring philosophy

The dashboard answers:

1. Is the NAS healthy?
2. What needs attention?
3. What changed recently?

Detailed metrics live under Monitoring.

Do not ship Prometheus/Grafana by default.

## Collection

Collect:

### System

- CPU;
- load;
- memory;
- temperature;
- uptime;
- zram;
- write activity where practical.

### Storage

- usage;
- per-disk temperature;
- SMART trends;
- read/write throughput;
- disk standby/wake;
- SSD endurance.

### Network

- throughput;
- errors/drops;
- link state/speed.

### Docker

- state;
- health;
- CPU/RAM;
- network;
- restart count;
- log storage.

### Services

- Samba;
- NFS;
- SSH;
- FTP;
- Docker;
- SnapRAID jobs.

## Sampling/downsampling

Keep high-frequency samples in RAM.

Persist aggregated data:

- recent: 1 minute;
- 7d: ~5 minutes;
- 30d: ~30 minutes;
- long-term: hourly.

Exact policy configurable.

This reduces SSD writes.

## Events vs alerts

An **event** is something that happened.

An **alert** is a condition requiring attention.

Example:

`docker.container.restarted` = event.

`container restarted >5 times in 10 minutes` = alert.

## Alert states

- normal;
- pending;
- firing;
- acknowledged;
- resolved.

Temperature conditions use a durable pending window: the first hot sample is
recorded as `pending`, the alert fires only after the threshold remains true
for the configured duration, and cooling clears the pending state. Pending
windows survive a daemon restart, so a restart cannot bypass the debounce.
The same lifecycle applies to Docker health checks, with a two-minute pending
window for unhealthy containers.

## Alert rule examples

- temperature > 50°C for 5m;
- SMART pending sector > 0;
- pool free < 10%;
- last SnapRAID sync > 48h;
- backup failed;
- Docker unhealthy for 2m;
- link speed degraded;
- UPS on battery;
- system SSD wear high.

## Notification providers

The current runtime provides native adapters for:

- web UI;
- email;
- Telegram;
- Slack;
- Discord;
- Gotify;
- ntfy;
- generic webhook.

SMTP is the email transport; its target uses `smtp://host:port?to=address` and
the encrypted credentials contain the sender/username and password. All
provider credentials are encrypted with the recovery key and are omitted from
API responses, exports, audit metadata, and support bundles. Provider responses
are bounded and delivery attempts are retried with exponential backoff. Event-
driven deliveries use the daemon's bounded HTTP client, which keeps provider
calls replaceable in tests and prevents an unbounded network wait.

Keep the provider abstraction independent from LumoNAS alert routing so a
future provider can be added without changing alert semantics.

## Routing

Users choose destination per severity/rule.

Example:

```text
Disk failure → Telegram + email + ntfy
Container stopped → Telegram only
Backup success → UI only
```

## Notification quality

Send context:

- resource;
- current state;
- threshold;
- duration;
- NAS name;
- action link if local context supports it.

Send recovery message when useful.

Repeated failures for the same channel and event enter a five-minute cooldown
after three consecutive failures. The failure counter and cooldown are stored
in SQLite, so a daemon restart cannot immediately resume spamming an
unavailable provider. Successful delivery clears the window, and orphaned
state is removed with operational retention cleanup.

## Jobs

All long operations use the job system.

Show global Jobs panel.

Job details contain:

- progress;
- stage;
- logs;
- resource locks;
- start/end;
- cancellation capability;
- result.

## Scheduler

Central Scheduled Jobs page should show:

- SnapRAID sync;
- scrub;
- SMART;
- backups;
- rsync;
- update checks;
- custom supported maintenance.

Detect schedule collisions and warn about multiple heavy disk jobs.

## Event timeline

Persist meaningful activity:

- config changes;
- disk events;
- Docker deployments;
- backup outcomes;
- SnapRAID;
- user login/admin actions;
- updates;
- UPS events.

Allow filtering by resource/severity/type.

## Capacity forecasting

Use stored daily/weekly capacity history.

The live system metrics contract also reports filesystem usage for `/`,
LumoNAS state, application data, disk branches, and pools when those paths are
available. Usage at 80% opens a warning alert and usage at 95% opens a critical
alert; returning below the warning threshold resolves the generated alert.
This keeps recovery state and logs visible before an `ENOSPC` failure.

Only display prediction after enough history.

Show range/uncertainty, e.g.:

> At the recent growth rate, Media may reach 90% in 3–5 months.

## Acceptance criteria

- A stopped container changes state without page refresh.
- Alert spam is suppressed while a condition remains firing.
- Critical storage events survive restart.

Backup runs are failed closed during daemon startup when they were queued or
running at the time of interruption. In-flight destination copies are marked
failed as well, and the backup health response reports the failure instead of
leaving stale work appearing active.

Backup execution is also fail-closed around SQLite persistence: the daemon must
durably record `running`, bundle metadata, each copy, and each verification
before continuing. Completion or failure events are published only after the
final run state is persisted, so a database write failure cannot produce a
false-success event.

- Metrics retention does not generate unbounded SQLite growth.
