# Security

This guide covers AllureDeck's security controls in depth. For deployment configuration (TLS, JWT secrets, CORS) see [deployment.md](deployment.md#security). For OIDC SSO setup see [authentication.md](authentication.md). To report a vulnerability, follow [SECURITY.md](../SECURITY.md).

---

## Table of Contents

1. [Authentication Overview](#authentication-overview)
2. [Sliding Sessions and Rotating Refresh Tokens](#sliding-sessions-and-rotating-refresh-tokens)
3. [Session Revocation](#session-revocation)
4. [Per-Account Brute-Force Throttle](#per-account-brute-force-throttle)
5. [Per-IP Rate Limiting](#per-ip-rate-limiting)
6. [Per-Request Active User Check](#per-request-active-user-check)
7. [CSRF Protection](#csrf-protection)
8. [Security Headers](#security-headers)
9. [Email Uniqueness Across Providers](#email-uniqueness-across-providers)
10. [Webhook URL Security](#webhook-url-security)
11. [Encryption at Rest](#encryption-at-rest)
12. [Audit Log](#audit-log)
13. [Production Checklist](#production-checklist)

---

## Authentication Overview

AllureDeck uses JWT-based authentication with two token types:

| Token | Lifetime (default) | Delivery | Purpose |
|-------|--------------------|----------|---------|
| **Access token** | 1 hour (`JWT_ACCESS_TOKEN_EXPIRES`) | `jwt` HttpOnly cookie or `Authorization: Bearer` header | Authenticates API requests |
| **Refresh token** | 30 days (`JWT_REFRESH_TOKEN_EXPIRES`) | `refresh_jwt` HttpOnly cookie | Obtains new access tokens |

Algorithm: HMAC-SHA256 (`HS256`). Key: `JWT_SECRET_KEY`.

Every token carries a unique **JTI** (JWT ID). Revoked JTIs are persisted to the `jwt_blacklist` table and survive restarts. A background goroutine prunes expired blacklist entries.

Authentication is off unless `SECURITY_ENABLED=true` — the API default is `false`, and the full-stack Compose files and the Helm chart turn it on. Role checks run either way: with it off, requests carry no identity and role-checked endpoints return `403 Access denied: missing claims`. The API refuses to start when `SECURITY_ENABLED=true` and `JWT_SECRET_KEY` is the insecure default (`super-secret-key-for-dev`).

For full token lifetime configuration and RBAC details see [deployment.md](deployment.md#security).

---

## Sliding Sessions and Rotating Refresh Tokens

AllureDeck implements **refresh token rotation**: each time a refresh token is used, a new refresh token is issued and the old one is invalidated. This means:

- Active users get a silently-extended session (the 30-day window slides forward on each refresh)
- Stolen refresh tokens are detected on reuse: if an old token in a family is presented after rotation, the entire token family is immediately revoked and an `auth.refresh.compromise` audit event is recorded (see [Audit Log](audit-log.md#events-recorded))

Refresh-token families are stored in the `refresh_token_families` table. Each row tracks the current JTI, the previous JTI (with a short grace window for benign retries), and the family status (`active`, `compromised`, or `revoked`).

**Proactive token refresh:** The UI refreshes the access token before expiry to avoid mid-session interruptions, using a background timer that fires when the token is within a configurable window of expiry.

---

## Session Revocation

Sessions are revoked immediately (not just after token expiry) in these cases:

| Trigger | What is revoked |
|---------|----------------|
| User password change (self) | All other refresh-token families for that user; current access token JTI blacklisted |
| Admin password reset | All refresh-token families for the target user; all API keys deleted |
| Account deactivation (`DELETE /api/v1/users/{id}`, the Users page) | All refresh-token families for the target user; all API keys deleted |
| Account deactivation (`PATCH /api/v1/users/{id}` with `active: false`) | All refresh-token families for the target user; API keys are suspended and work again on reactivation |
| Refresh token reuse detected | Entire token family marked `compromised` |
| Explicit logout | Current refresh-token family revoked; access token JTI blacklisted |

Revocation is best-effort and non-blocking: failures are logged at warn level but do not prevent the triggering operation (e.g. a password change succeeds even if session revocation encounters a transient DB error). The per-request active-user cache (see below) closes any residual access window within its TTL.

---

## Per-Account Brute-Force Throttle

In addition to per-IP rate limiting, AllureDeck applies a **per-account throttle** that resists distributed credential-stuffing attacks where an attacker rotates source IPs.

**Policy (production defaults):**

| Parameter | Value | Description |
|-----------|-------|-------------|
| Sliding window | 15 minutes | Failure counter resets after this period of inactivity |
| Soft threshold | 1 | Any failure starts recording delays |
| Lockout threshold | 20 | Failures within the window trigger a hard lockout |
| Lockout duration | 15 minutes | Duration of the hard lockout from the threshold-crossing failure |
| Backoff | 1s → 2s → 4s → … → 60s max | Exponential delay recommended before each response |

**On lockout:** The API returns `429 Too Many Requests` with a `Retry-After` header and records an `auth.login.failure` audit event with `metadata.lockout=true` and `metadata.locked_until`.

**On success:** The failure counter for the account is reset immediately. A legitimate user who fat-fingers their password is not penalised across sessions.

Username matching is case-insensitive and whitespace-trimmed.

---

## Per-IP Rate Limiting

Token-bucket rate limiters operate at the IP level on the credential, token, and account-provisioning endpoints. They do **not** cover other API requests. Each IP gets an independent bucket per limiter. When the bucket empties, the API returns `429 Too Many Requests` with `Retry-After: 1`.

| Limiter | Rate and burst | Routes |
|---------|----------------|--------|
| Shared | 5 requests/s, burst 10 | `POST /api/v1/login`<br>`POST /api/v1/auth/refresh`<br>`POST /api/v1/users/me/password`<br>`GET /api/v1/projects/{project_id}/builds/{build_id}/tests/{history_id}/failure-summary` (AI summaries)<br>`GET /api/v1/auth/oidc/login` and `GET /api/v1/auth/oidc/callback` (only when OIDC is enabled) |
| User provisioning | 5 requests/min, burst 5 | `POST /api/v1/users`<br>`POST /api/v1/users/{id}/password` (admin reset) |

Behind a reverse proxy, set `TRUST_FORWARDED_FOR=true` so the limiter uses the real client IP (the first `X-Forwarded-For` entry) rather than the proxy's IP. Enable it only when a trusted proxy sets that header; otherwise clients can spoof their IP.

Stale IP entries are cleaned up periodically in the background.

---

## Per-Request Active User Check

Every authenticated request checks that the user's `is_active` flag is still true before proceeding. This ensures deactivated accounts lose access without waiting for their token to expire.

To avoid a database round-trip on every request, the result is cached in an in-process `UserActiveCache` with a **2-second TTL**. Deactivation propagates within 2 seconds to every API replica and to the MCP server, none of which share a cache.

Cache details:
- Keyed by stringified `user.id`
- Lookups by ID (JWT `sub` claim) and by email (API-key path) resolve to the same entry
- Bounded to 10,000 entries; oldest entry evicted when the cap is exceeded
- Concurrent loads for the same key are deduplicated via an inline singleflight to prevent DB stampedes

---

## CSRF Protection

AllureDeck uses the **double-submit cookie** pattern:

1. On login, a `csrf_token` cookie is set (random 64-character hex string)
2. Mutating requests (POST, PUT, DELETE, PATCH) from browser sessions must include an `X-CSRF-Token` header matching the cookie value
3. Comparison uses `crypto/subtle.ConstantTimeCompare` to prevent timing attacks

**Exempt paths:** `GET`, `HEAD`, `OPTIONS` methods; the `/login` endpoint (no cookie exists yet); any request without a `jwt` session cookie (API key / programmatic access — CSRF does not apply because these requests do not use cookie-based authentication).

CSRF protection is only active when `SECURITY_ENABLED=true`.

---

## Security Headers

The following headers are set on every API response:

| Header | Value | Purpose |
|--------|-------|---------|
| `X-Content-Type-Options` | `nosniff` | Prevent MIME-type sniffing |
| `X-Frame-Options` | `DENY` | Prevent clickjacking |
| `Content-Security-Policy` | `default-src 'self'` | Restrict resource loading to same origin |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Limit URL leakage in Referer header |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=(), usb=()` | Deny browser features the API does not use |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | HSTS — **only sent when `TLS=true`** to avoid confusing plain-HTTP operators |

---

## Email Uniqueness Across Providers

A user's email address is globally unique across all authentication providers. This prevents account confusion or privilege escalation via cross-provider email collisions:

- A `local` user and an OIDC user cannot share the same email
- When an OIDC user authenticates via JIT provisioning, the server rejects the provisioning if the email is already registered under a different provider
- Email addresses are normalised to lowercase at creation time

By default an OIDC sign-in that collides with an existing account is rejected with `409` and an `auth.login.failure` audit event (`metadata.reason="oidc_email_collision"`). Setting `OIDC_AUTO_LINK_BY_EMAIL=true` instead rebinds the existing account to the new OIDC identity, but **only** when the identity provider asserts `email_verified=true`; an unverified colliding email is still rejected (`oidc_email_collision_unverified`). Successful links record the account's `previous_provider` in the `auth.login.success` audit metadata. Enable it only for identity providers you fully trust to verify email ownership — otherwise anyone who can register an arbitrary email at the provider can take over the matching local account.

---

## Webhook URL Security

Webhook target URLs are checked twice to prevent Server-Side Request Forgery (SSRF).

**At save time** (create and update):

- Only `http` and `https` schemes are accepted
- The URL's hostname is resolved; any result in a private IP range (RFC 1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, plus IPv6 `fc00::/7`), loopback (127.0.0.1, ::1), link-local, or unspecified range is rejected, including IPv4-mapped IPv6 forms such as `::ffff:127.0.0.1`
- `localhost` is explicitly blocked

**At delivery time** — a hostname that was public when saved can later resolve to an internal address (DNS rebinding), so every delivery re-checks:

- The delivery client resolves the hostname itself and refuses the connection if **any** resolved address is loopback, private, link-local, or unspecified
- It then connects to the validated address directly, so the address that was checked is the address that is used and DNS cannot change in between
- At most 4 redirects are followed (the 5th is refused), and each redirect target is re-validated with the same rules, so a public URL cannot bounce a delivery to an internal host
- The delivery client does not use proxy environment variables

Webhook URLs and secrets are **encrypted at rest** using AES-GCM. URLs are **masked in API responses** (scheme + host only, e.g. `https://hooks.slack.com/****`). Secrets are never returned by any API endpoint; only the `has_secret` boolean is exposed.

---

## Encryption at Rest

Sensitive webhook fields (URL and secret) are encrypted at rest using AES-256-GCM. The key is the SHA-256 of `ENCRYPTION_KEY` (any string, any length), or of `JWT_SECRET_KEY` when that is not set. Webhooks saved under the JWT secret or under the development default stay readable and are re-encrypted under the current key the next time they are saved. A key that is no longer configured cannot be recovered, and webhooks saved under it are skipped. To rotate `JWT_SECRET_KEY`, first set `ENCRYPTION_KEY` to the current JWT secret.

---

## Audit Log

AllureDeck records all authentication events, user lifecycle changes, and session revocations to the `audit_log` PostgreSQL table. See [Audit Log](audit-log.md) for the full event catalog, schema, and example investigation queries.

---

## Production Checklist

For the deployment security checklist (JWT secret requirements, CORS, TLS, Helm secrets management, OIDC hardening) see [deployment.md](deployment.md#production-security-checklist).

Key items specific to this guide:

- [ ] Set `JWT_SECRET_KEY` to at least 32 random bytes — the API rejects the insecure default at startup
- [ ] Configure `TRUST_FORWARDED_FOR=true` when running behind a reverse proxy
- [ ] Review `audit_log` table periodically; implement a retention policy appropriate for your compliance requirements
- [ ] If rotating `JWT_SECRET_KEY`, first set `ENCRYPTION_KEY` to the current JWT secret so stored webhooks stay readable
- [ ] Use [API keys](features.md#api-keys-for-cicd) rather than user credentials for CI/CD pipelines — API keys are role-scoped, audited, and revocable without affecting the user's session. Keys are **instance-wide by default**; set an optional `project_ids` allow-list at creation to restrict a key to specific projects (enforced on project routes — a scoped key cannot read, modify, delete, or create projects outside its list).
