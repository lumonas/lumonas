# Data Safety and Privileged Operations

## Objective

LumoNAS must make accidental data destruction difficult even if:

- UI code has a bug;
- device letters change after reboot;
- a disk disappears;
- a user clicks the wrong resource;
- a stale browser submits an old operation;
- an automated job encounters an unexpected storage state.

No software can guarantee that data can never be lost. The goal is to prevent silent or ambiguous destructive behavior and make failure modes explicit.

## Safety classes

### Class A — read-only

Examples:

- SMART;
- temperature;
- filesystem usage;
- Docker status/logs;
- SnapRAID status;
- network counters.

No destructive confirmation.

### Class B — reversible

Examples:

- restart container;
- enable/disable share;
- change alert rule.

Create config generation first where relevant.

### Class C — disruptive

Examples:

- change IP;
- migrate Docker data-root;
- large ACL change;
- unmount a filesystem.

Must show impact and support rollback/verification.

### Class D — destructive

Examples:

- filesystem format;
- partition deletion;
- secure erase;
- parity initialization over an existing disk;
- remove storage in a way that deletes files.

Requires storage safety unlock, a signed immutable operation plan, reauthentication, and strong physical identity confirmation.

## Stable disk identity

Persist a composite identity:

```text
WWN
serial
model
capacity
GPT disk GUID
partition UUID
filesystem UUID
```

The current kernel path is metadata only.

Never persist:

```text
/dev/sdb = Data Disk 2
```

Instead:

```text
disk_id = sha256(normalized WWN/serial identity)
current_path = /dev/sdb
```

## Pre-execution revalidation

Immediately before a Class D action verify:

- same WWN/serial;
- expected capacity;
- expected partition table generation/hash;
- expected filesystem UUID;
- current mount state;
- pool membership;
- SnapRAID membership;
- share dependencies;
- Docker bind-mount dependencies;
- no newer configuration generation invalidated the plan.

Any mismatch aborts.

## Immutable operation plan

The planner produces:

```text
Operation ID
Action
Stable target identity
Expected current state
Requested final state
Dependency snapshot
Config generation
Expiry
Plan hash
```

The UI confirms exactly that plan.

`lumonas-privd` executes that signed/hashed plan, not user-supplied replacement parameters.

Plans expire quickly.

## Storage Safety Lock

Default:

```text
Storage Safety: LOCKED
```

Class D operations are not even available to the privileged executor until an administrator:

1. reauthenticates;
2. explicitly unlocks destructive storage maintenance;
3. receives a short-lived capability;
4. confirms the exact action.

Automatically relock after ~15 minutes or after the destructive job completes.

## Dependency graph

Maintain resource relationships:

```text
Physical Disk
→ Partition
→ Filesystem
→ Mount
→ mergerfs Pool
→ Share
→ Docker bind mounts / Backups / Services
```

Class D operations should normally be **blocked**, not merely warned, while dependencies exist.

Example:

```text
Cannot format WD Red •••4F29

Used by:
- Media Pool
- SMB Media
- Jellyfin
- SnapRAID d3
```

The user must intentionally dismantle dependencies first.

## Unknown disks

When a disk first appears:

```text
Unknown disk
Existing filesystem/data detected
```

Default actions:

1. Import read-only / inspect;
2. Adopt without formatting where compatible;
3. Erase and initialize — advanced/destructive.

Never automatically initialize.

## Suspicious storage state

Enter **Storage Safety Mode** when:

- a protected disk disappears;
- filesystem UUID unexpectedly changes;
- expected mount becomes empty;
- multiple disk identities conflict;
- SnapRAID reports a condition requiring force flags;
- pool branch resolves to unexpected backing path.

While active, block:

- automatic SnapRAID sync;
- destructive pool balancing;
- pruning that could make recovery worse;
- automated force options.

Reads may continue when safe.

Onboarding uses the same boundary: the initial SnapRAID sync is withheld until
the generated configuration is accepted by the privileged broker and every
referenced stable disk identity has been revalidated.

## SnapRAID rules

Never automatically use force options to “fix” failed automation.

If sync refuses because a disk is missing/unmounted:

- stop;
- preserve parity;
- raise critical alert;
- show exact disk identity;
- require investigation.

Configure multiple content files on separate devices.

## Installer safety

The installer is allowed to modify only the selected **system target**.

Existing LumoNAS disks receive a hard protection marker in the install plan.

Data disks are mounted read-only during recovery discovery.

The review screen must list:

- exact erased system disk;
- exact protected data disks;
- whether any non-system disk write is planned.

## Privileged service API

Allowed operations should be narrow:

- `MountFilesystem`
- `UnmountFilesystem`
- `CreateFilesystem`
- `ApplyACL`
- `ApplyNetworkProfile`
- `ReloadService`
- `PowerAction`

Never implement:

- `RunShell`
- `ExecuteCommand`
- `RunScriptAsRoot`

The Unix socket is protected by ownership and mode, and Linux builds also
verify `SO_PEERCRED` before parsing a request. Only UID 0 or the `lumonas`
service group may connect; unrelated local processes are closed immediately.

If an advanced terminal is offered, it is a separate explicitly authenticated admin feature, not the control channel used by normal product functions.

## Worker isolation

For high-risk operations, use one-shot systemd transient workers with:

- capability bounding;
- device allow-list;
- private temporary directory;
- restricted filesystem view;
- no network unless needed.

## Audit

Every mutation stores:

- operation ID;
- actor/session;
- time;
- plan hash;
- target stable identity;
- old state;
- requested new state;
- result;
- error;
- config generation.

## Acceptance criteria

A destructive operation must abort if:

- `/dev/sdX` changes;
- disk serial differs;
- filesystem UUID differs;
- target becomes mounted;
- target becomes part of a pool after plan creation;
- plan expires;
- another admin changes relevant config;
- `lumonas-privd` receives a parameter not present in the confirmed plan.

CI must include simulated races for these cases.
