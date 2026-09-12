# Future UI Improvements

## 1. Adaptive density

Offer display density:

- Comfortable;
- Compact.

Tables become denser without changing information architecture.

## 2. User-customizable dashboard

Allow adding/reordering safe widgets:

- storage;
- Docker;
- UPS;
- network;
- backups;
- temperatures.

Keep a high-quality default.

## 3. Resource relationship graph

Interactive graph:

```text
Disk → Pool → Share → App → Backup
```

Use it primarily for impact analysis, not as decorative visualization.

## 4. “What changed?” mode

After updates/config changes, highlight changed resources.

Useful after:

- Docker deploy;
- network change;
- restore;
- OS update.

## 5. Better comparison UI

Config generations:

- side-by-side diff;
- semantic changes;
- selective restore.

Docker:

- current vs proposed Compose.

## 6. Guided troubleshooting

Instead of generic errors:

```text
Jellyfin cannot start because port 8096 is used by X.
```

Offer safe actions:

- choose free port;
- stop conflicting service;
- inspect details.

## 7. Health explanations

Each score component explains:

- measured inputs;
- warnings;
- recommended fixes;
- what is unknown.

## 8. Natural-language command/search

Optional local helper later:

```text
"show disks hotter than 45°C"
"which apps use Media?"
```

Must not be required for normal UI and should not receive secrets by default.

## 9. Notification actions

On supported PWA/browser notifications:

- open resource;
- acknowledge;
- retry safe job.

Never put destructive action in a push notification.

## 10. Better mobile workflows

Optimize specifically for:

- restart app;
- acknowledge alert;
- inspect disk;
- run backup;
- check UPS;
- see active jobs.

Avoid raw Compose editor on small screens unless requested.

## 11. Installer web continuation

USB screen shows QR/address.

User continues installer on phone/laptop.

## 12. Accessibility profiles

- high contrast;
- reduced motion;
- large touch targets;
- persistent text labels;
- color-blind-safe status palette.

## 13. Keyboard productivity

Shortcuts:

- command palette;
- open notifications;
- open jobs;
- search current table;
- close drawer;
- next/previous resource.

## 14. Contextual help

Inline short explanations with optional deeper docs.

Examples:

- SnapRAID vs backup;
- parity disk sizing;
- LACP requirement;
- plain FTP risk;
- Docker privileged mode.

## 15. “Expert details” inspector

Consistent technical drawer across resources:

- raw JSON/API state;
- config paths;
- command-equivalent read-only hints;
- UUIDs/IDs.

Useful for support without cluttering normal UI.
