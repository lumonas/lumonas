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

Support hysteresis/debounce to avoid spam.

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

Planned:

- web UI;
- email;
- Telegram;
- Slack;
- Discord;
- Gotify;
- ntfy;
- generic webhook.

Use a provider abstraction. Apprise may be used as a bridge initially, but keep MyNAS alert routing semantics independent so individual native integrations can replace it later.

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

Only display prediction after enough history.

Show range/uncertainty, e.g.:

> At the recent growth rate, Media may reach 90% in 3–5 months.

## Acceptance criteria

- A stopped container changes state without page refresh.
- Alert spam is suppressed while a condition remains firing.
- Critical storage events survive restart.
- Metrics retention does not generate unbounded SQLite growth.
