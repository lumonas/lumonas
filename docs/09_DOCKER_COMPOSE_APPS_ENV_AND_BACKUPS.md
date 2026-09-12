# Docker, Compose, Apps, Common Environment and Backups

## Principle

A beginner should install apps without YAML.

An expert should edit standard Compose.

Both modes operate on the same underlying stack.

## Filesystem layout

Suggested:

```text
/srv/mynas/docker/
├── stacks/
│   ├── jellyfin/
│   │   ├── compose.yaml
│   │   ├── stack.env
│   │   └── mynas.yaml
│   └── immich/
├── templates/
└── exports/

/var/lib/mynas/secrets/docker/
```

Appdata defaults to a configurable Apps storage resource, not necessarily the system SSD.

## Source of truth

`compose.yaml` is the authoritative Docker stack definition.

MyNAS metadata is separate.

Never regenerate the complete YAML from a simple form and discard unknown options.

Use a YAML AST/editor strategy that preserves unknown nodes and comments where realistically possible.

## UI levels

App detail tabs:

- Overview;
- Settings;
- Storage;
- Network;
- Environment;
- Logs;
- Backup;
- Compose.

Normal users never need the Compose tab.

## App catalog

Catalog entry includes:

- metadata/icon/category;
- upstream links;
- Compose template;
- form schema;
- default storage mappings;
- ports;
- optional hardware;
- secrets;
- health expectations;
- backup/recovery contract;
- version/update policy.

Keep catalog separately updateable from MyNAS core.

## Arbitrary Compose import

Sources:

- paste YAML;
- upload;
- Git repository;
- existing local directory.

Analyze Compose and generate a configuration form from unresolved variables:

```text
PORT → port field
UPLOAD_LOCATION → storage picker
PASSWORD → secret field
TZ → MyNAS timezone
```

Show advanced constructs that MyNAS does not graphically understand, but preserve them.

## Common environment variables

Variable scopes:

1. built-in MyNAS variables;
2. user global variables;
3. stack variables;
4. service/container variables;
5. secrets.

Suggested built-ins:

```text
MYNAS_HOSTNAME
MYNAS_LAN_IP
MYNAS_TIMEZONE
MYNAS_APPDATA
MYNAS_DATA
MYNAS_BACKUPS
```

## Storage references instead of fragile paths

A global variable such as `MEDIA` should optionally reference:

```text
Pool: Media
Directory: /
```

MyNAS resolves it to the current effective host path.

This lets internal mount locations change without editing dozens of stacks.

UI shows:

```text
MEDIA → Media Pool → used by 8 stacks
```

## Secrets

Do not store secrets in normal global `.env`.

Store encrypted in MyNAS secret store with restrictive files/runtime injection.

UI tracks where secrets are used.

Backups include encrypted secret material.

## App/Cache disk role

If a dedicated SSD/NVMe exists, recommend:

- Docker data-root;
- appdata;
- databases;
- caches;
- temporary download/transcode areas.

Keep changing databases away from SnapRAID archive data.

## Bind mounts

For NAS apps, prefer understandable bind mounts to hidden anonymous volumes.

Named volumes remain supported.

UI should present:

```text
Container /config → Apps / Jellyfin
Container /movies → Media / Movies
```

## Docker Engine access

`mynas-web` must not receive direct Docker socket access.

A controlled backend module uses Docker Engine API/SDK.

Advanced Compose can still request privileged Docker behavior, but the UI must flag risky constructs:

- `privileged: true`;
- Docker socket;
- host root bind;
- host PID;
- sensitive devices.

## Compose validation/deployment

Before apply:

1. parse YAML;
2. resolve variables;
3. validate Compose;
4. detect port conflicts;
5. validate storage references;
6. check missing secrets;
7. calculate diff;
8. snapshot configuration;
9. pull images if online;
10. deploy;
11. health-check;
12. commit/rollback metadata.

## Offline images

Docker → Images → Import should accept tar archives produced by `docker save`.

When stack image is unavailable and no Internet:

```text
Image not present locally.
Import image or connect to a registry.
```

Future optional offline app packs can bundle common images separately from the OS ISO.

## Updates

Default:

- notify;
- show current and available image/version;
- never blindly auto-update databases/apps.

Update wizard:

- config snapshot;
- optional appdata backup;
- pull image;
- stop/recreate;
- health-check;
- retain previous image temporarily;
- offer rollback.

## Logging

Set sane Docker log rotation defaults.

UI shows log disk usage and alerts on unusually noisy containers.

## App backup

Separate:

- stack configuration;
- appdata;
- external user data.

Recovery strategy choices:

- stop stack + backup appdata (safe default);
- crash-consistent;
- custom hooks;
- catalog database-aware backup.

Catalog apps such as PostgreSQL-backed apps should define pre-backup dump/restore steps.

## Git-backed Compose

Support:

- repository;
- branch/tag;
- path;
- SSH key/token secret;
- fetch changes;
- diff;
- manual deploy default;
- optional future automatic deploy.

## Acceptance criteria

- Simple-mode edits preserve unknown YAML.
- An exported stack runs with normal Docker Compose outside MyNAS.
- Changing a global storage reference lists affected stacks before apply.
- Secrets do not appear in normal exports/logs.
- A user can deploy a basic app without knowing Compose.
