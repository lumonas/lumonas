# Shares, Users, Permissions and File Services

## User mental model

The primary resource is a **Share**, not “a Samba config section”.

A share has:

- name;
- storage location;
- access policy;
- enabled protocols;
- protocol-specific advanced options.

## Protocols

Initial/following planned support:

- SMB;
- NFS;
- SFTP;
- FTP/FTPS;
- rsync;
- Time Machine over SMB.

## Canonical identity/access model

Avoid separate independent permission systems.

LumoNAS should own a canonical access policy:

```text
principal → resource → No access / Read only / Read & Write
```

Render this into:

- POSIX ownership/ACL;
- Samba config/ACL behavior;
- NFS export restrictions;
- FTP/SFTP restrictions.

## Users and groups

Distinguish:

- **Management users** — can log into LumoNAS UI;
- **File users** — can access shares;
- **Service identities** — apps/backup accounts.

One person may have both management and file access.

UI should not force all Samba users to become UI admins.

## UID/GID

Allocate stable Unix UID/GID and back them up.

Advanced UI exposes IDs.

Normal UI shows names/groups.

## Simple ACL editor

Default levels:

- Read & Write;
- Read only;
- No access.

Support groups such as:

- family;
- media;
- backup.

Advanced ACL editor can be added later but must not create a second contradictory source of truth.

## SMB

Features:

- share enable/disable;
- guest policy;
- recycle bin optional;
- macOS compatibility;
- browse/discovery;
- per-share advanced Samba options with validation.

Before reload run Samba config validation.

## Time Machine wizard

User selects:

- destination share/folder;
- allowed users;
- quota;
- advertise via mDNS.

Hide Samba-specific Time Machine VFS details in normal UI.

## NFS

Simple setup:

- share path;
- allowed host/network;
- read-only/read-write;
- root squashing safe default;
- advanced options collapsed.

Show CIDR examples and validate.

## SFTP

Prefer SFTP over plain FTP.

UI:

- allowed file users;
- root/chroot location;
- SSH key/password policy;
- service binding (LAN/Tailscale).

## FTP/FTPS

Support because required, but display warning for plain FTP.

Options:

- FTPS recommended;
- plain FTP advanced/legacy;
- passive port range;
- allowed users;
- service binding.

## rsync

Support:

- rsync daemon modules;
- SSH-based rsync jobs;
- inbound/outbound backup use.

Normal wizard should guide remote host, auth, source, destination, schedule.

## Share creation wizard

Flow:

1. Location
2. Access
3. Protocols
4. Review

Review shows:

- effective path;
- users/groups;
- protocol exposure;
- destructive/non-destructive changes.

## Service binding

Each protocol can listen on selected networks/interfaces:

```text
SMB:
LAN yes
Tailscale yes
IoT VLAN no
```

Firewall generation should align with binding.

## Permission migration

When changing a large tree ACL:

- estimate affected files;
- show background job;
- snapshot config;
- apply recursively only when explicitly selected;
- support cancel where safe;
- log failures.

Do not recursively `chown` terabytes because a user changed one share-level policy unless that operation is actually necessary and approved.

## Acceptance criteria

- A beginner can create a Windows/macOS share without seeing Samba syntax.
- A file user can exist without management UI access.
- A management admin can exist without SMB access.
- Service binding is reflected in firewall state.
- Config validation prevents broken Samba/NFS reload from replacing the working configuration.
