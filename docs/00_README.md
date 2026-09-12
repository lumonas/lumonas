# MyNAS OS — Detailed Product & Engineering Plan

**Planning snapshot:** September 2026  
**Working name:** MyNAS  
**Primary target:** x86-64 home / prosumer NAS and Docker host  
**Baseline OS:** Debian 13 Stable (Trixie), pinned by MyNAS release image  
**Core philosophy:** offline-first, data-safe, Compose-first, observable, recoverable, Linux-compatible.

## Purpose of this package

This package turns the product discussion into an implementation-oriented specification. It is intentionally split by subsystem so storage, Docker, UI, networking, recovery, and installer work can proceed without creating one unmaintainable document.

The documents describe:

- what the feature should do;
- how it should behave in the UI;
- the recommended underlying Linux technology;
- safety invariants;
- data and API implications;
- failure handling;
- testing expectations;
- acceptance criteria;
- future extension points.

If a technical choice was not previously fixed, this plan selects the option that best fits the goals: low resource use, strong safety, normal Linux interoperability, and a modern browser UI.

## Product statement

MyNAS should feel like a modern appliance but remain a normal Linux system underneath.

A beginner should be able to:

1. write the ISO to a USB drive;
2. install fully offline;
3. open `mynas.local`;
4. create/import storage;
5. create SMB/NFS shares;
6. deploy Docker apps without seeing YAML;
7. receive useful alerts;
8. replace a failed system SSD and restore the NAS.

An expert should still be able to:

- SSH into the host;
- inspect standard filesystems;
- use normal Docker Compose;
- inspect SMART/SnapRAID/mergerfs;
- import existing disks and stacks;
- use advanced networking and ACLs;
- export configurations without proprietary lock-in.

## Documents

| File | Topic |
|---|---|
| `01_PRODUCT_VISION_AND_PRINCIPLES.md` | Product principles and boundaries |
| `02_SYSTEM_ARCHITECTURE.md` | Services, processes, privilege boundaries |
| `03_DATA_SAFETY_PRIVILEGED_OPERATIONS.md` | Destructive-operation safety model |
| `04_OFFLINE_INSTALLER_IMAGE.md` | USB ISO, offline installation and recovery media |
| `05_FIRST_BOOT_AND_ONBOARDING.md` | First browser setup and recommended storage |
| `06_STORAGE_DISKS_POOLS_SNAPRAID.md` | Disks, filesystems, mergerfs, SnapRAID |
| `07_DISASTER_RECOVERY_AND_CONFIG_BACKUP.md` | Config backups and full system-SSD recovery |
| `08_SHARES_USERS_PERMISSIONS_FILE_SERVICES.md` | SMB/NFS/SFTP/FTP/rsync/Time Machine |
| `09_DOCKER_COMPOSE_APPS_ENV_AND_BACKUPS.md` | Docker UX, Compose, common vars, secrets, app backup |
| `10_NETWORKING_REMOTE_ACCESS_FIREWALL.md` | Network UI, Tailscale/WireGuard, firewall |
| `11_MONITORING_ALERTS_NOTIFICATIONS_JOBS.md` | Live status, alerts, telemetry, notifications |
| `12_UI_UX_DESIGN_SYSTEM_AND_WIREFRAMES.md` | Navigation, design language, interaction patterns |
| `13_FILES_BROWSER_AND_DATA_OPERATIONS.md` | File browser, copy/move/upload/recycle bin |
| `14_SYSTEM_RUNTIME_WRITE_OPTIMIZATION_UPDATES_POWER.md` | zram/tmpfs, updates, A/B future, UPS/power |
| `15_SECURITY_AUTH_SECRETS_AUDIT.md` | Authentication, secrets, audit and API security |
| `16_TESTING_CI_RELEASE_ENGINEERING.md` | CI, destructive tests, ISO and upgrade tests |
| `17_IMPLEMENTATION_ROADMAP.md` | Dependency-based implementation sequence |
| `18_RISKS_WEAK_POINTS_MITIGATIONS.md` | Weak-point analysis and mitigations |
| `19_FUTURE_FEATURES.md` | Future product features |
| `20_FUTURE_UI_IMPROVEMENTS.md` | UI/UX evolution ideas |
| `21_FUTURE_ARCHITECTURE_EXPERIMENTS.md` | A/B OS, immutable root and other experiments |
| `22_DATA_MODELS_API_EVENTS.md` | Suggested entities, APIs and event taxonomy |
| `23_ACCEPTANCE_TESTS.md` | Product-level acceptance scenarios |
| `24_SOURCES_AND_BASELINES.md` | External technical baselines |

## Recommended repository shape

```text
mynas/
├── cmd/
│   ├── mynasd/
│   ├── mynas-web/
│   └── mynas-privd/
├── internal/
│   ├── storage/
│   ├── docker/
│   ├── shares/
│   ├── network/
│   ├── monitoring/
│   ├── alerts/
│   ├── recovery/
│   ├── jobs/
│   ├── auth/
│   └── config/
├── web/
│   └── React/Vite application
├── catalog/
│   └── optional built-in app templates
├── packaging/
│   ├── debian/
│   └── systemd/
├── installer/
│   ├── image/
│   ├── offline-repo/
│   └── recovery/
├── schemas/
│   ├── api/
│   ├── events/
│   └── backup/
├── test/
│   ├── integration/
│   ├── qemu/
│   └── destructive/
└── docs/
```

## Non-negotiable rules

1. Never identify a destructive target only by `/dev/sdX`.
2. Never expose an arbitrary root-shell endpoint from the management API.
3. Never automatically bypass a safety check after an operation fails.
4. Unknown/existing data disks default to read-only import, never erase.
5. `compose.yaml` remains standard Compose and is the Docker source of truth.
6. A MyNAS release is not considered recoverable until CI destroys the system disk and restores onto a blank replacement disk.
7. Cloud services may enhance MyNAS but must never be required for core NAS operation.
