# UI/UX Design System and Wireframe Specification

## Product feel

Target:

- 70% clean/simple;
- 30% technical density where useful;
- modern but not flashy;
- excellent dark and light modes;
- desktop powerful;
- mobile genuinely usable.

## Desktop shell

Primary sidebar:

```text
Overview
Storage
Shares
Docker
Files
Backups
Network
Monitoring
Users
Settings
```

Top bar:

- server identity;
- command search (`Ctrl/⌘ K`);
- jobs;
- notifications;
- user menu.

Optional thin bottom status bar:

- health;
- CPU;
- RAM;
- network;
- uptime.

## Navigation rules

Use:

- tables for collections;
- drawers for resource detail;
- full pages/wizards for complex workflows;
- small modals for simple confirmations;
- global persistent jobs for long work.

Avoid full page navigation for every disk/container click.

## Dashboard

Default content:

- overall health;
- System summary;
- Storage summary;
- Protection/Backup;
- Docker;
- needs attention;
- recommendations;
- recent activity.

Avoid a wall of charts.

Charts appear when a user drills into a resource.

## Global status vocabulary

Use consistent states:

- Healthy;
- Attention;
- Warning;
- Critical;
- Offline.

Do not invent synonyms on different pages.

## Color

Neutral surfaces dominate.

Reserve semantic colors for state:

- success;
- attention;
- critical;
- active/information.

Never rely on color alone.

## Storage screen

Tabs:

- Overview;
- Disks;
- Pools;
- Protection;
- Activity.

Disk table normal columns:

- Name;
- Role;
- Capacity;
- Used;
- Temp;
- Health.

Advanced columns opt-in.

Disk drawer:

- Overview;
- SMART;
- Usage;
- Activity;
- Settings;
- dangerous actions separated visually.

## Shares screen

One share can expose multiple protocols.

Table:

```text
Name | Location | Protocols | Access
```

Create wizard:

```text
Location → Access → Protocols → Review
```

## Docker screen

Default to Apps.

Tabs:

- Apps;
- Stacks;
- Containers;
- Images;
- Volumes;
- Networks;
- Settings.

App page:

- Overview;
- Settings;
- Storage;
- Network;
- Environment;
- Logs;
- Backup;
- Compose.

## Simple vs advanced

Do not make a global “Beginner mode”.

Every screen starts simple and provides local `Advanced` disclosure.

Raw Linux details are always available but not primary.

## Compose editor

Features:

- syntax highlighting;
- validate;
- format;
- unresolved variable hints;
- MyNAS path variable completion;
- diff;
- deploy.

Message:

> Changes made here are preserved.

## Network UI

Overview:

- interfaces;
- Internet/gateway/DNS;
- VPN.

Editing remote network settings shows rollback timer.

## Backup/recovery UI

Separate:

- normal backup jobs;
- disaster recovery.

Recovery page shows a readiness score and exact missing items.

## Command palette

Search resources and actions:

- open disk;
- restart Jellyfin;
- view logs;
- run SMART test;
- create share;
- check updates.

Destructive operations should never execute directly from command palette without full safety workflow.

## Mobile

Bottom navigation:

```text
Home | Storage | Docker | More
```

More:

- Shares;
- Files;
- Backups;
- Network;
- Monitoring;
- Users;
- Settings.

Complex Compose editing may remain desktop-focused.

## Performance targets

Perceived:

- cached app shell loads quickly on LAN;
- navigation instant;
- UI reacts immediately to user input;
- long backend action acknowledges immediately and becomes a job;
- status events appear within roughly 1–2 seconds.

## Loading

Use skeletons for initial loading.

Use real progress for long jobs.

Avoid endless global spinners.

## Errors

Every error should explain:

- what failed;
- what was changed/not changed;
- whether rollback occurred;
- what to do next.

Example:

```text
SnapRAID sync stopped.
Disk Media 3 is unavailable.
No parity changes were made.
[Inspect storage]
```

## Dangerous actions

Do not place `Format` beside normal buttons.

Use separate Danger Zone/workflow.

Always show model, serial suffix/full identity, capacity and impact.

## Accessibility

- keyboard-first navigation;
- visible focus;
- WCAG-friendly contrast;
- screen-reader labels;
- reduced-motion preference;
- semantic status text;
- no icon-only critical action.

## Design components

Create reusable components:

- `HealthBadge`
- `Metric`
- `ResourceTable`
- `ResourceDrawer`
- `JobProgress`
- `DangerZone`
- `EmptyState`
- `DependencyList`
- `OperationReview`
- `AlertBanner`
- `StorageUsage`
- `DiskIdentity`
- `DiffViewer`
- `TimelineEvent`

## UI principles

1. Status before configuration.
2. Common task first.
3. Progressive disclosure.
4. No ambiguous destructive action.
5. Realtime but calm.
6. Mobile useful, desktop powerful.
7. Same resource name/status everywhere.
8. CLI-compatible, UI-friendly.
