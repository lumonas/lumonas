# Implementation Roadmap

Scope is intentionally broad. This roadmap does not remove features; it orders them by architectural dependency so later modules are built on safe foundations.

## Phase 0 — Engineering foundation

Deliver:

- Go monorepo layout;
- frontend shell;
- SQLite schema/migrations;
- event bus;
- job engine;
- config generations;
- audit framework;
- typed `lumonas-privd` IPC;
- common resource IDs.
- OpenAPI and frontend response-contract parity checks.
- bounded journald retention policy checks.
- typed bounded network diagnostic command runner.
- GPT disk GUID and partition UUID revalidation across destructive plans.

Exit criteria:

- config transaction can commit/rollback a harmless setting;
- SSE delivers events;
- privileged API has no generic shell execution.

## Phase 1 — Hardware and system observability

Deliver:

- disk discovery by stable identity;
- partition-table identity fallback when WWN and serial are unavailable;
- SMART;
- sensors;
- CPU/RAM;
- network discovery;
- service status;
- dashboard skeleton;
- Monitoring basics.

Exit:

- reboot/device-letter reorder does not change disk identity;
- live UI works.

## Phase 2 — Storage foundation

Deliver:

- mount/import XFS/ext4;
- filesystem metadata;
- disk roles;
- mergerfs pool creation/import;
- dependency graph;
- Storage Safety Lock;
- Class D operation planner;
- SMART jobs.

Exit:

- destructive race tests pass;
- existing disk can be imported without modification.

## Phase 3 — SnapRAID

Deliver:

- config generation;
- content replication;
- sync/scrub/status;
- fail-closed sync/scrub command outcomes;
- safety mode;
- scheduling;
- job progress;
- replacement workflow foundation.

Exit:

- missing disk freezes automation;
- no automatic force behavior.

## Phase 4 — Identity, shares and permissions

Deliver:

- management/file users;
- groups;
- simple canonical ACL;
- SMB;
- NFS;
- SFTP;
- FTP/FTPS;
- rsync;
- Time Machine wizard;
- Avahi.

Exit:

- common family-share scenario works without CLI.

## Phase 5 — Docker foundation

Deliver:

- Docker API integration;
- Compose stack directory;
- Apps/Cache disk role;
- variables/secrets;
- import/paste Compose;
- editor;
- logs;
- image import;
- health/resource status.

Exit:

- standard exported Compose works outside LumoNAS.

## Phase 6 — Simple Docker app experience

Deliver:

- catalog format;
- dynamic form schema;
- simple mode;
- storage picker;
- common environment vars;
- Compose-to-form inference;
- preserved unknown YAML;
- update review/rollback.

Exit:

- user can install Jellyfin without seeing YAML and later add custom Compose options safely.

## Phase 7 — Networking UI

Deliver:

- DHCP/static/DNS;
- checkpoints;
- interfaces;
- diagnostics;
- Tailscale;
- WireGuard;
- service binding;
- nftables service policy;
- VLAN/bond/bridge.

Exit:

- failed remote IP change auto-recovers.

## Phase 8 — Recovery and backup

Recovery schema should have existed from early phases; this phase completes product UX.

Deliver:

- automatic config snapshots;
- recovery key;
- multi-disk recovery copies;
- verification;
- restore planner;
- system-disk recovery installer;
- Docker recovery contracts;
- appdata backup;
- remote destinations.

Exit:

- release-blocking delete-system-disk test passes.

## Phase 9 — Installer and polished onboarding

Deliver:

- branded offline image;
- hardware detection;
- protected disk review;
- first browser setup;
- recovery boot flow;
- local console.

The installer can be developed earlier in parallel, but Stable release requires this phase.

## Phase 10 — Alerts/notifications/power

Deliver:

- alert rules;
- Telegram/Slack/Discord/email/Gotify/ntfy/webhooks;
- NUT;
- scheduled power;
- WOL;
- recovery/health scoring.

## Phase 11 — Files and admin productivity

Deliver:

- file browser;
- recycle bin;
- transfer jobs;
- command palette;
- support bundle;
- configuration history/diff;
- improved audit.

## Phase 12 — Update hardening

Deliver:

- signed update channel;
- pre-update recovery verification;
- rollback strategy;
- evaluate A/B OS deployment.

## Continuous tracks

Always active:

- UX/accessibility;
- security review;
- recovery tests;
- hardware compatibility;
- performance/resource optimization;
- documentation;
- catalog maintenance.

## Definition of Stable 1.0

Because scope is not being treated as a problem, 1.0 may include the broad system. Still, Stable designation should depend on **reliability gates**, not feature count:

- offline installer;
- safe storage;
- shares;
- Compose;
- recovery;
- network rollback;
- monitoring/alerts;
- signed update process;
- destructive test suite.
