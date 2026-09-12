# Weak Points, Risks and Mitigations

Scope is explicitly not considered a blocker here. Risks are solved architecturally rather than by deleting desired features.

## 1. Privileged destructive operations — Critical

**Risk:** bug or stale device path destroys data.

**Mitigation:**

- three-process privilege model;
- stable disk IDs;
- immutable signed operation plan;
- revalidation at execution;
- dependency graph;
- Storage Safety Lock;
- fail closed;
- destructive CI races.

## 2. Recovery promise — Critical

**Risk:** “restore everything” becomes marketing rather than reality.

**Mitigation:**

- layered R0/R1/R2/R3 recovery;
- recovery contracts per Docker app;
- verified encrypted backups;
- multi-disk config copies;
- versioned generations;
- restore planning;
- release-blocking blank-system-disk restore.

## 3. Permissions complexity — High

**Risk:** POSIX/Samba/NFS/Docker identities conflict.

**Mitigation:**

- one canonical MyNAS access model;
- stable UID/GID;
- simple access levels;
- advanced ACL later but same source of truth;
- drift detection.

## 4. Simple Docker UI versus arbitrary Compose — High

**Risk:** graphical save destroys custom YAML.

**Mitigation:**

- Compose remains source of truth;
- AST-aware targeted edits;
- preserve unknown constructs;
- show “advanced configuration detected”;
- diff before deploy.

## 5. Docker backup consistency — High

**Risk:** copying running DB directories produces unreliable recovery.

**Mitigation:**

- recovery contract;
- default stop-and-backup;
- catalog DB dump hooks;
- verify restore metadata;
- readiness score reflects real coverage.

## 6. Network lockout — High

**Mitigation:** NetworkManager checkpoints with browser confirmation and timeout rollback.

## 7. Offline image maintenance — High

**Risk:** ISO packages become stale/security-sensitive.

**Mitigation:**

- automated image builds;
- pinned manifest;
- signed updates;
- clear support lifecycle;
- regular refreshed Stable point images.

## 8. OS upgrades — High

**Risk:** Debian upgrade breaks storage/network.

**Mitigation:**

- no unattended major dist-upgrade;
- pre-update recovery verification;
- tested supported migration paths;
- future A/B system slots.

## 9. App catalog maintenance — High

**Mitigation:**

- catalog separate from core OS;
- app templates versioned;
- health/backup schema;
- community catalogs later;
- normal Compose always available.

## 10. Hardware diversity — High

**Risk:** SMART/HBA/sensors/NIC differ.

**Mitigation:**

- rely on mature Linux tools;
- capability detection;
- graceful feature fallback;
- diagnostic bundle;
- hardware compatibility tests.

## 11. SnapRAID misunderstanding — High

**Mitigation:**

- show unsynced changes;
- never label parity “backup”;
- show last sync;
- clear protection wording;
- freeze automation on suspicious state.

## 12. mergerfs behavior misunderstanding — Medium/High

**Mitigation:**

- opinionated presets;
- show physical backing disk;
- explain files remain on individual disks;
- advanced policy only when needed.

## 13. Monitoring write amplification — Medium

**Mitigation:**

- in-memory sampling;
- downsampling;
- bounded retention;
- Docker log limits;
- zram.

## 14. Configuration drift through SSH — Medium

**Mitigation:**

- detect actual-vs-desired state;
- show drift;
- allow import/restore;
- avoid silently overwriting external changes.

## 15. Secrets leakage — Critical

**Mitigation:**

- encrypted secret store;
- reference IDs;
- redacted diagnostics/logs;
- encrypted recovery;
- automated tests with planted canary secret.

## 16. UI becoming OMV-like complexity — High

**Mitigation:**

- short sidebar;
- progressive disclosure;
- resource-centric Shares/Apps/Pools;
- command palette;
- user-facing names rather than Linux internals.

## 17. Background job conflicts — Medium

**Risk:** scrub, backup, SMART and migration saturate disks.

**Mitigation:**

- scheduler resource locks;
- detect heavy-job overlap;
- priority;
- maintenance windows.

## 18. Long-running operation interruption — Medium

**Mitigation:**

- persistent job state;
- idempotent stages where possible;
- resumption only when safe;
- explicit recovery instructions otherwise.

## 19. Data metadata on disks becoming stale — Medium

**Mitigation:**

- generation numbers;
- checksums;
- newest-consistent selection;
- never trust metadata alone for destructive identity.

## 20. False health score confidence — Medium

**Mitigation:**

- score explanation;
- critical condition overrides;
- do not hide unknown/unmonitored status;
- distinguish parity, backup, appdata recovery.

## Security architecture review gate

Before Stable:

- threat model;
- privilege-boundary review;
- API authorization review;
- backup crypto review;
- path traversal/fuzz tests;
- Docker socket exposure review.
