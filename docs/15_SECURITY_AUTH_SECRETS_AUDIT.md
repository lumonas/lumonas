# Security, Authentication, Secrets and Audit

## Authentication

Initial:

- local admin account;
- strong password;
- secure session cookies;
- CSRF protection;
- rate limiting;
- optional TOTP 2FA.

Implemented backend primitives:

- username-bound passkey/WebAuthn registration and sign-in ceremonies;
- encrypted-at-rest-independent credential metadata persisted in SQLite;
- single-use, expiring ceremony state and monotonic signature counters;
- self-service passkey management with owner/admin delegation controls.

The management UI exposes both passkey sign-in (AuthGate) and self-service
enrollment/revocation (settings → security → Passkeys). Discoverable
username-less sign-in remains follow-up work; sign-in currently requires the
username. Multiple admins and role-based permissions are supported by the
existing management identity model.

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
- Tailscale auth keys;
- SMTP passwords;
- ACME account/private keys as applicable.

Never emit secrets into:

- diagnostic bundle;
- audit log;
- normal API responses;
- UI event stream;
- exports by default.

Tailscale authentication keys are staged in a mode-0600 temporary file and
passed to `tailscale up` using its `file:` auth-key form. This keeps the key
out of process arguments and structured command logs; the file is removed when
the bounded command returns.

Release CI adds a high-confidence credential-format scan over tracked files and
executes canary redaction plus privileged-operation rejection tests. This is a
leak-detection backstop, not a replacement for keeping real secret values out
of the repository and diagnostic inputs.

## Secret references

Domain entities reference secret IDs.

Do not copy plaintext secret values into many tables/files.

## HTTPS

Default local HTTPS path:

- local LumoNAS CA/cert;
- explain client trust;
- allow user certificate import;
- ACME optional when network/domain conditions allow.

## CORS Configuration

Cross-Origin Resource Sharing (CORS) is configured via the `LUMONAS_CORS_ORIGINS` environment variable.

- **Default**: Same-origin only (no cross-origin requests allowed)
- **Configuration**: Comma-separated list of allowed origins
- **Example**: `LUMONAS_CORS_ORIGINS=http://localhost:5173,https://nas.example.com`

When enabled, the server sets `Access-Control-Allow-Origin`, `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`, and `Access-Control-Allow-Credentials` headers.

## CSRF Protection

CSRF tokens are generated on login and must be included in all state-changing requests (POST, PUT, PATCH, DELETE).

- **Token generation**: Included in login response as `csrfToken` field
- **Token usage**: Include as `X-CSRF-Token` header on mutations
- **Token expiry**: Tokens expire with the session (12 hours)
- **Storage**: In-memory map with automatic cleanup

## Rate Limiting

Authentication endpoints are rate-limited to prevent brute-force attacks:

- **Login endpoint**: 5 attempts per 5-minute window per IP
- **Response**: HTTP 429 with `Retry-After` header
- **Scope**: Per-IP tracking using `X-Forwarded-For` or direct connection

## Request Body Size Limits

API endpoints enforce request body size limits:

- **JSON requests**: 32 MB maximum
- **Multipart uploads**: 2 GB maximum
- **Response**: HTTP 413 Payload Too Large when exceeded

## Security Response Headers

All API responses include security headers:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Content-Security-Policy: frame-ancestors 'none'`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Strict-Transport-Security: max-age=63072000; includeSubDomains` (HTTPS only)

## SSH

Recommended defaults:

- root login disabled;
- admin SSH key;
- password authentication disabled if key exists;

Docker stack creation/actions, container actions, image updates, and offline
image imports use the same management-role check and request audit path as
other mutating API operations. Read-only Docker inventory remains available to
authenticated management users.
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
