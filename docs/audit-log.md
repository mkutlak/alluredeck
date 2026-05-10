# Audit Log

AllureDeck records security-sensitive operations to an `audit_log` table in PostgreSQL. Every authentication event and user lifecycle change produces a structured row that can be queried for incident response, compliance, or operational monitoring.

Related documentation: [Security](security.md) · [User Management](user-management.md) · [Authentication](authentication.md)

---

## Table of Contents

1. [Events Recorded](#events-recorded)
2. [Event Schema](#event-schema)
3. [Viewing the Audit Log](#viewing-the-audit-log)
4. [Retention](#retention)
5. [Using Audit Events for Security Investigations](#using-audit-events-for-security-investigations)

---

## Events Recorded

Every audit event has an **action** string, an **outcome** (`success` or `failure`), and optional **metadata** with action-specific detail.

### Authentication Events

| Action | Trigger | Metadata |
|--------|---------|----------|
| `auth.login.success` | Successful local or DB-backed login | — |
| `auth.login.failure` | Failed login attempt (wrong password or locked out) | `failure_count`, `lockout` (bool), `locked_until` (when locked) |
| `auth.logout` | User signed out | — |
| `auth.refresh.success` | Refresh token accepted; new access token issued | — |
| `auth.refresh.compromise` | Refresh token reuse detected (potential token theft) | `family_id`, `status` |

### User Lifecycle Events

| Action | Trigger | Minimum Role | Metadata |
|--------|---------|--------------|----------|
| `users.create` | Admin creates a new local user | admin | `new_user_email`, `role` |
| `users.update.role` | Admin changes a user's role | admin | `new_role` |
| `users.update.active` | Admin activates or deactivates a user | admin | `active` (bool) |
| `users.delete` | Admin deactivates a user (soft-delete) | admin | — |
| `users.password_change` | User changes their own password | self (local accounts) | — |
| `users.password_reset` | Admin resets another user's password | admin | `target_active` |

### Session and API Key Events

| Action | Trigger | Metadata |
|--------|---------|----------|
| `auth.session.revoke_all` | All refresh-token families revoked for a user | `trigger` (password_change / password_reset / user_deactivate), `revoked` (count), `target_id` |
| `api_keys.create` | User creates an API key | `key_name` |
| `api_keys.delete` | User deletes an API key | `key_name` |
| `api_keys.cascade_delete` | All API keys deleted for a user (password reset or deactivation) | `trigger`, `username`, `deleted` (count) |

---

## Event Schema

Each row in `audit_log` contains:

| Column | Type | Description |
|--------|------|-------------|
| `id` | bigint | Primary key |
| `occurred_at` | timestamptz | When the event was recorded |
| `actor_id` | bigint (nullable) | `users.id` of the initiating user; null for unauthenticated events (e.g. failed login before identity resolves) |
| `actor_label` | text | Denormalised email or username — readable after the actor is deleted or renamed |
| `target_type` | text | Entity type acted on: `user`, `api_key`, or `session` |
| `target_id` | text | Stringified ID of the target entity |
| `action` | text | Action string (see tables above) |
| `outcome` | text | `success` or `failure` |
| `ip` | text | Wire-level client IP from `RemoteAddr` (not `X-Forwarded-For` — audit always records what the server saw at the connection layer) |
| `user_agent` | text | `User-Agent` request header |
| `request_id` | text | Correlation ID from the `X-Request-ID` header (set by the request-ID middleware) |
| `metadata` | jsonb | Action-specific key-value pairs (see Metadata column in tables above) |

---

## Querying the Audit Log

There is no in-app audit viewer in AllureDeck v0.34. Audit events are written to the PostgreSQL `audit_log` table; query them directly from the database, or pipe them into your existing log/SIEM pipeline (`pg_dump`, logical replication, or a periodic export job).

Indexes on `occurred_at`, `(actor_id, occurred_at)`, and `(action, occurred_at)` keep typical investigation queries fast.

```sql
-- All failed login attempts in the last 24 hours
SELECT occurred_at, actor_label, ip, metadata
FROM audit_log
WHERE action = 'auth.login.failure'
  AND occurred_at > now() - interval '24 hours'
ORDER BY occurred_at DESC;

-- Role changes made by a specific admin
SELECT occurred_at, actor_label, target_id, metadata
FROM audit_log
WHERE action = 'users.update.role'
  AND actor_label = 'admin@example.com'
ORDER BY occurred_at DESC;

-- All events related to a specific user (as actor or target)
SELECT occurred_at, action, outcome, actor_label, target_id, metadata
FROM audit_log
WHERE actor_id = 12 OR (target_type = 'user' AND target_id = '12')
ORDER BY occurred_at DESC;
```

---

## Retention

Audit log rows are **not automatically pruned** — they accumulate indefinitely. For long-running deployments, implement a retention policy by scheduling a periodic deletion of old rows:

```sql
-- Delete audit records older than 1 year (run as a scheduled job)
DELETE FROM audit_log WHERE occurred_at < now() - interval '1 year';
```

Index on `occurred_at` is present for efficient time-range queries. The table also has indexes on `actor_id` and `action` for common investigation patterns.

---

## Using Audit Events for Security Investigations

**Detecting brute-force attacks:**

```sql
SELECT ip, actor_label, count(*) AS attempts, max(occurred_at) AS last_attempt
FROM audit_log
WHERE action = 'auth.login.failure'
  AND occurred_at > now() - interval '1 hour'
GROUP BY ip, actor_label
HAVING count(*) > 5
ORDER BY attempts DESC;
```

**Detecting refresh token reuse (potential session hijack):**

```sql
SELECT occurred_at, actor_label, ip, user_agent, metadata
FROM audit_log
WHERE action = 'auth.refresh.compromise'
ORDER BY occurred_at DESC
LIMIT 50;
```

The `auth.refresh.compromise` event fires when a refresh token from a previously-rotated family is presented. AllureDeck immediately revokes the entire family on detection. The `metadata` column contains the `family_id` for correlation with the `refresh_token_families` table.

**Auditing privilege escalation:**

```sql
SELECT occurred_at, actor_label, target_id, metadata->>'new_role' AS new_role
FROM audit_log
WHERE action = 'users.update.role'
ORDER BY occurred_at DESC;
```

**Tracking session revocations:**

The `auth.session.revoke_all` event captures the `trigger` (what caused the bulk revocation: `password_change`, `password_reset`, or `user_deactivate`) and `revoked` count. Cross-reference with `auth.login.success` events to see whether the affected user re-authenticated after revocation.
