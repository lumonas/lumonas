# Security, Authentication, Secrets and Audit

## Authentication

Initial:

- local admin account;
- strong password;
- secure session cookies;
- CSRF protection;
- rate limiting;
- optional TOTP 2FA.

Planned:

- passkeys/WebAuthn;
- multiple admins;
- role-based permissions.

## Management vs file users

Keep management authorization separate from SMB/NFS file identities.

Management roles may include:

- Owner/Admin;
- Operator;
- Read-only.

Fine-grained roles can evolve later.

## Sessions

UI should show:

- active sessions;
- device/browser;
- IP/network;
- login time;
- last activity;
- revoke.

The settings API exposes active session metadata using token digests only and
supports explicit revocation by session ID. Raw session cookies are never
returned in the inventory response.

Alert optionally on new admin login.

## Passkeys/2FA

Passkeys should eventually become preferred for management UI.

Recovery should not depend only on one passkey device.

Provide emergency/recovery flow.

## Secrets

Store encrypted:

- Docker passwords;
- registry tokens;
- Telegram bot token;
- Slack webhook;
- S3 credentials;
- WireGuard private keys;
- SMTP passwords;
- ACME account/private keys as applicable.

Never emit secrets into:

- diagnostic bundle;
- audit log;
- normal API responses;
- UI event stream;
- exports by default.

## Secret references

Domain entities reference secret IDs.

Do not copy plaintext secret values into many tables/files.

## HTTPS

Default local HTTPS path:

- local LumoNAS CA/cert;
- explain client trust;
- allow user certificate import;
- ACME optional when network/domain conditions allow.

## SSH

Recommended defaults:

- root login disabled;
- admin SSH key;
- password authentication disabled if key exists;
- UI public-key management.

## API

- versioned API;
- authenticated;
- CSRF-safe browser actions;
- API tokens later;
- scoped API tokens rather than full session equivalence.

`lumonas-privd` has no network API.

## Docker security

Warn on high-risk Compose:

- privileged containers;
- Docker socket;
- host filesystem root;
- host PID;
- host network;
- sensitive device access.

Do not prohibit expert use, but require explicit understanding where risk is substantial.

## Audit log

Record:

- admin login/logout;
- failed auth;
- config changes;
- network changes;
- permission changes;
- Docker deploy/update;
- secret lifecycle metadata (not secret value);
- destructive operation plans/results;
- update/rollback;
- recovery.

Audit record should be append-oriented.

## Security recommendations

Health score may recommend:

- SSH password login enabled;
- no 2FA;
- plain FTP enabled;
- UI exposed to unexpected network;
- stale admin sessions;
- updates overdue;
- overly privileged Docker stack.

## Diagnostic bundle redaction

Before export:

- remove passwords/tokens/private keys;
- redact environment secrets;
- optionally hash public IP/MAC/serial data based on privacy mode.

Provide preview of included categories.

## Acceptance criteria

- Browser frontend never receives secret plaintext unless user explicitly requests reveal.
- A diagnostic bundle contains no configured secret values.
- Root privileged control socket cannot be reached remotely.
- Revoked admin session stops working immediately or within a clearly defined short TTL.
