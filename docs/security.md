# Security

This guide covers AllureDeck's security controls in depth. For deployment configuration (TLS, JWT secrets, CORS) see [deployment.md](deployment.md#security). For OIDC SSO setup see [authentication.md](authentication.md).

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

The API refuses to start when `SECURITY_ENABLED=true` and `JWT_SECRET_KEY` is the insecure default (`super-secret-key-for-dev`).

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
| Account deactivation | All refresh-token families for the target user; all API keys deleted |
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

A token-bucket rate limiter operates at the IP level for all API requests. Each IP gets an independent bucket. When the bucket empties, the API returns `429 Too Many Requests` with `Retry-After: 1`.

Behind a reverse proxy, set `TRUST_FORWARDED_FOR=true` so the limiter uses the real client IP from `X-Forwarded-For` rather than the proxy's IP.

Stale IP entries are cleaned up periodically in the background.

---

## Per-Request Active User Check

Every authenticated request checks that the user's `is_active` flag is still true before proceeding. This ensures deactivated accounts lose access without waiting for their token to expire.

To avoid a database round-trip on every request, the result is cached in an in-process `UserActiveCache` with a **30-second TTL**. Deactivation propagates within 30 seconds across all pod instances under load.

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

---

## Webhook URL Security

Webhook target URLs are validated before storage to prevent Server-Side Request Forgery (SSRF):

- Only `http` and `https` schemes are accepted
- The URL's hostname is resolved; any result in a private IP range (RFC 1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16), loopback (127.0.0.1, ::1), or link-local range is rejected
- `localhost` is explicitly blocked

Webhook URLs and secrets are **encrypted at rest** using AES-GCM. URLs are **masked in API responses** (scheme + host only, e.g. `https://hooks.slack.com/****`). Secrets are never returned by any API endpoint; only the `has_secret` boolean is exposed.

---

## Encryption at Rest

Sensitive webhook fields (URL and secret) are encrypted at rest using AES-GCM with a key derived from the `JWT_SECRET_KEY`. The encryption key is set at startup and must not change between restarts — changing `JWT_SECRET_KEY` on a running deployment with existing webhooks will break decryption for those records.

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
- [ ] If rotating `JWT_SECRET_KEY`, re-encrypt webhook secrets before deploying the new key
- [ ] Use [API keys](features.md#api-keys-for-cicd) rather than user credentials for CI/CD pipelines — API keys are role-scoped, audited, and revocable without affecting the user's session. Keys are **instance-wide by default**; set an optional `project_ids` allow-list at creation to restrict a key to specific projects (enforced on project routes — a scoped key cannot read, modify, delete, or create projects outside its list).
