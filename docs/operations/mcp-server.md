# MCP Server Operations

The Model Context Protocol (MCP) server enables AI clients (Claude Code, Cursor, Claude Desktop) to query test-failure data and propose mutations against AllureDeck. See [modelcontextprotocol.io](https://modelcontextprotocol.io) for the MCP specification.

---

## Protocol version

The server implements MCP **2026-07-28** and advertises it by default.

Older clients are downgraded automatically — the SDK negotiates back to `2025-11-25` and earlier, so no client needs updating in step with the server. What changes at 2026-07-28:

| Feature | Effect here |
|---------|-------------|
| Stateless transport | No `initialize` handshake and no `Mcp-Session-Id`. Every request is self-describing, so any replica can serve any request. Controlled by `MCP_STATELESS` (default `true`). |
| Request cancellation | A client aborting a call now cancels the in-flight database query rather than orphaning it. Matters most for `diagnose_failure`. |
| Cacheable lists | `tools/list` carries a 1-hour `ttlMs`, resource lists 5 minutes, inlined attachment content 24 hours (`private` scope). Clients re-fetch less. |
| Confirmed writes | The three `propose_*` tools ask the user to confirm before writing. See [Write confirmations](#write-confirmations). |
| `Mcp-Name` header | Used for per-tool rate-limit pricing without parsing the request body. |
| Request body cap | Bodies above 4 MiB are rejected by the SDK (OOM protection). |

Deprecated features (roots, sampling, logging, and the legacy HTTP+SSE transport) are not used by this server, so the 12-month deprecation window has no impact on AllureDeck deployments.

Because the transport is stateless, `mcp.replicaCount` may now be raised above 1. Leave it at 1 if you set `MCP_STATELESS=false`.

---

## Prerequisites

- AllureDeck v0.34.1 or later
- PostgreSQL with migration 0041 applied (`defect_proposals`, `known_issue_proposals`, `flaky_proposals` tables)
- For proposals: an API key with `allow_mcp_writes` enabled, owned by a
  registered user account (see [Token Issuance](#token-issuance))

---

## Enabling MCP

### Environment Variables

On the **cmd/mcp** deployment:

```bash
ENABLE_MCP_SERVER=true
MCP_ALLOWED_ORIGINS=https://your.host
MCP_RATE_LIMIT_PER_MIN=60
MCP_RATE_LIMIT_BURST=10
MCP_STATELESS=true          # 2026-07-28 stateless transport (default)
MCP_TOOL_COSTS=""           # per-tool rate-limit pricing override
```

`EXTERNAL_URL` is required whenever `ENABLE_MCP_SERVER=true` — the server signs absolute attachment download URLs and builds proposal review links from it. The Helm chart derives it automatically (see `alluredeck.externalURL`).

On the **cmd/api** deployment (exposes the admin proposals route):

```bash
ENABLE_MCP_SERVER=true
```

Leave `MCP_RATE_LIMIT_PER_MIN` and `MCP_RATE_LIMIT_BURST` unset to use defaults (60 req/min, burst 10).

### Docker Compose

```yaml
services:
  mcp:
    build:
      context: .
      dockerfile: docker/Dockerfile.mcp
    ports:
      - "8081:8081"
    environment:
      ENABLE_MCP_SERVER: "true"
      MCP_ALLOWED_ORIGINS: "http://localhost:5050"
      LOG_LEVEL: "info"
      DATABASE_URL: "postgres://user:pass@postgres:5432/alluredeck"
```

### Helm

```bash
helm upgrade --install alluredeck charts/alluredeck \
  --set mcp.enabled=true \
  --set mcp.image.tag=v0.13.0 \
  --set mcp.config.allowedOrigins="https://your.host"
```

### Verification

```bash
kubectl logs -f deploy/alluredeck-mcp | grep "MCP server listening"
```

If disabled, you will see `MCP server disabled via feature flag`.

---

## Token Issuance

### Personal Tokens

1. Log in to the AllureDeck UI
2. Navigate to **Settings → API Keys**
3. Click **Create API Key**
4. Toggle **Allow MCP writes** if proposals are needed
5. Copy the token — it is shown once and cannot be retrieved again

The key inherits your own username and role; there is no role selector, so log
in as an **editor** or **admin** if it needs to propose. Tokens have the form
`ald_` followed by 64 hex characters.

### Machine Tokens (CI)

**A key always belongs to whoever creates it.** The request body carries no
username or role — both are taken from the caller's own credentials. There is
no way to mint a key on behalf of another account, so to get a key owned by a
CI identity you authenticate as that identity and create it.

```bash
curl -X POST http://localhost:8080/api/v1/api-keys \
  -H "Authorization: Bearer <token-of-the-owning-account>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ci-mcp",
    "allow_mcp_writes": true,
    "expires_at": "2027-01-01T00:00:00Z",
    "project_ids": [12, 34]
  }'
```

| Field | Meaning |
|-------|---------|
| `name` | Label shown in the UI. Required, max 64 characters. |
| `allow_mcp_writes` | Required for the `propose_*` tools. Defaults to false. |
| `expires_at` | Optional RFC3339 timestamp; must be in the future. Omit for a key that never expires. |
| `project_ids` | Optional allow-list, max 100 entries. Omit for instance-wide access. |

The key inherits the creator's **role**, so create it while authenticated as an
editor or admin if it needs to propose. The response contains the full token
once and never again; the format is `ald_<hex>`, and it works against both the
REST API and the MCP endpoint. Each account may hold at most **5 keys**.

An existing API key can create further keys, so CI can rotate its own
credentials without a human logging in.

### Using a dedicated bot account

Not required — a key created by any registered user works. A dedicated account
is still worth it for CI: revoking it leaves human keys untouched, the audit
log attributes proposals to `mcp-bot@…` rather than to a person, and CI does
not consume part of someone's 5-key budget.

Create the account through **Settings → Users** (or your OIDC provider), give it
the **editor** role, then sign in as it and create the key. It must be an
account that can actually authenticate — a `users` row alone is not enough,
because the key is minted from that account's session.

> **Keys owned by config-file users cannot propose.** If you sign in with
> `ADMIN_USERNAME`/`VIEWER_USER` from the environment rather than with a
> registered account, keys you create are owned by a literal such as `admin`,
> which has no `users` row. Read-only tools work; the `propose_*` tools refuse
> with an explanation, because a proposal has to reference a real user. Use a
> registered account for anything that writes.

---

## Configuring a Client

### Claude Code

**Option A — CLI (recommended)**

```bash
claude mcp add --transport http alluredeck https://your.host/mcp \
  --header "Authorization: Bearer ald_<your-key>"
```

This registers the server in the current directory's local scope. To register it globally (available in every directory), add `--scope user`:

```bash
claude mcp add --transport http --scope user alluredeck https://your.host/mcp \
  --header "Authorization: Bearer ald_<your-key>"
```

After adding, type `/mcp` inside Claude Code to see server status and the full tool list.

**Option B — project `.mcp.json` (team-shareable, committed to source control)**

Create `.mcp.json` at the repository root:

```json
{
  "mcpServers": {
    "alluredeck": {
      "type": "http",
      "url": "https://your.host/mcp",
      "headers": { "Authorization": "Bearer ald_<your-key>" }
    }
  }
}
```

Every team member who opens the project in Claude Code picks up this configuration automatically.

### Cursor

Create or edit `~/.cursor/mcp.json` (user-wide) or `.cursor/mcp.json` (project-scoped):

```json
{
  "mcpServers": {
    "alluredeck": {
      "type": "http",
      "url": "https://your.host/mcp",
      "headers": { "Authorization": "Bearer ald_<your-key>" }
    }
  }
}
```

Restart Cursor after editing.

### Claude Desktop — Experimental (requires mcp-proxy)

Claude Desktop's `claude_desktop_config.json` expects stdio-based servers. The AllureDeck MCP server is streamable HTTP only, so it is not natively supported.

Use [mcp-proxy](https://github.com/sparfenyuk/mcp-proxy) as a stdio↔HTTP bridge. Install it first (`pip install mcp-proxy` or see the mcp-proxy README), then add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "alluredeck": {
      "command": "mcp-proxy",
      "args": ["--transport=streamablehttp", "https://your.host/mcp"],
      "env": { "API_ACCESS_TOKEN": "ald_<your-key>" }
    }
  }
}
```

`mcp-proxy` reads `API_ACCESS_TOKEN` and forwards it as `Authorization: Bearer` to the upstream server.

Restart Claude Desktop after editing.

---

## Quick start in Claude Code

**Install**

```bash
claude mcp add --transport http alluredeck https://your.host/mcp \
  --header "Authorization: Bearer ald_<your-key>"
```

**Example prompts and the tools they invoke**

1. **"Show recently failed tests on `main` for the alluredeck project"**
   - Claude calls `list_recent_builds` to find the latest builds on the `main` branch for the named project.
   - Then calls `list_failing_tests` on the relevant build(s) to return the failing test names, durations, and status codes.
   - Output: a summary table of failed tests grouped by build, with counts and first-seen timestamps.

2. **"Why did test `<name>` fail in build `<N>`?"**
   - Claude calls `find_test_by_name` to locate the test record, then `get_test_failure` to retrieve the error message, stack trace, and attached log snippets.
   - Optionally calls `get_test_history` or `compare_builds` to show whether the failure is new or recurring.
   - Output: the failure message and stack trace, plus a trend summary if history is available.

3. **"Diagnose build `<N>` and tell me what changed since it last passed"**
   - Claude calls `diagnose_failure` once for the build; for each failing test it returns the error message, failed-step path, triage signals, and a `last_good` pointer to the most recent build where that test passed (with `builds_since`, branch-scoped when available).
   - Passing `include_last_good_diff: true` adds a `last_good_diff` per failure — the test's own `passed → failed` transition plus a bounded sample of co-regressions between the last-good build and this one — so the agent can bisect what changed.
   - No LLM runs server-side; the agent writes the plain-language hypothesis from this evidence (see "No server-side LLM tools" under Known Limitations).
   - Output: a per-test diagnosis grounded in the last-known-good baseline.

4. **"Mark this failure as a known flake"**
   - Claude calls `propose_mark_flaky` with the test identifier.
   - The tool does not apply the change directly — it creates a proposal and returns a `review_url`.
   - A human must approve or reject the proposal at `/admin/proposals` in the AllureDeck UI.
   - Output: confirmation that the proposal was created and the `review_url` to action it.

---

## Write confirmations

The three `propose_*` tools ask the user to confirm before writing anything. The prompt states exactly what will be recorded; for `propose_known_issue` it also reports how many of the last 1000 failure messages the regex matches, and warns when that exceeds half the sample — an over-broad pattern is the failure mode this gate exists to catch.

Nothing is written until the user accepts: no proposal row, no `audit_log` entry. Declining is reported as a successful call that wrote nothing, so the agent does not treat it as a retryable error.

**Clients that cannot prompt are unaffected.** The confirmation is gated on the client advertising elicitation support. A headless caller — a CI pipeline using an API key — writes directly, exactly as before this feature existed. Confirmation is also skipped entirely when `MCP_SIGNING_KEY` is unset, since the round trip could not then be authenticated.

Human approval in `/admin/proposals` is unchanged and remains the gate that actually applies a proposal. The confirmation is an additional, earlier gate that keeps unintended proposals out of the review queue in the first place.

### Security note

The confirmation carries an opaque `RequestState` through the client and back. Because the server is stateless and the client controls that round trip, the value is HMAC-SHA256 signed with `MCP_SIGNING_KEY` and bound to the tool, the calling user, and a hash of the arguments, with a 10-minute expiry. A caller therefore cannot forge an approval, reuse one tool's approval for another, or approve a narrow change and then retry with a broader one.

Rotating `MCP_SIGNING_KEY` invalidates any confirmation in flight (the user simply gets asked again) and any unexpired attachment download URL.

---

## Audit Log

Every MCP call that mutates the database inserts an `audit_log` row with one of these actions:

- `mcp.propose_defect_classify` — proposed a defect classification
- `mcp.propose_known_issue` — proposed a known-issue pattern
- `mcp.propose_flaky` — proposed marking a test as flaky
- `mcp.proposal_approve` — admin approved a proposal (logged by cmd/api)
- `mcp.proposal_reject` — admin rejected a proposal (logged by cmd/api)

### Query Recent Audit Entries

```sql
SELECT
  actor_user_id,
  action,
  target_type,
  target_id,
  outcome,
  created_at
FROM audit_log
WHERE action LIKE 'mcp.%'
ORDER BY created_at DESC
LIMIT 50;
```

### Export MCP Activity Report

```sql
SELECT
  a.actor_user_id,
  u.email,
  a.action,
  a.outcome,
  COUNT(*) as count,
  MAX(a.created_at) as latest
FROM audit_log a
LEFT JOIN users u ON a.actor_user_id = u.id
WHERE a.action LIKE 'mcp.%'
GROUP BY a.actor_user_id, u.email, a.action, a.outcome
ORDER BY latest DESC;
```

---

## Common Failure Modes

| Symptom | Cause | Fix |
|---------|-------|-----|
| 401 Unauthorized on every MCP call | Bearer token invalid or expired | Check `api_keys.last_used` in Postgres; re-issue token if needed |
| 403 Forbidden with "origin not allowed" | Client's Origin header not in `MCP_ALLOWED_ORIGINS` | Update `MCP_ALLOWED_ORIGINS` env var; comma-separated list |
| Missing-Origin requests blocked (non-browser clients) | Feature disabled | Leave `MCP_ALLOWED_ORIGINS` empty to allow missing-Origin requests |
| 429 Too Many Requests on every call | Per-API-key rate limit exceeded | Increase `MCP_RATE_LIMIT_PER_MIN` or contact admin to raise burst quota |
| 429 only on `diagnose_failure` / `get_test_history` | Expensive tools cost more than one request each | Raise `MCP_RATE_LIMIT_BURST`, or reprice the tool via `MCP_TOOL_COSTS` (e.g. `diagnose_failure=2`) |
| `propose_*` returns a prompt instead of a proposal | Working as intended — the client supports confirmations | Accept the prompt; the proposal is created on the retry leg |
| "confirmation state rejected" on a `propose_*` retry | `MCP_SIGNING_KEY` changed, or the 10-minute window elapsed | Re-run the tool; the user will be asked again |
| `propose_*` refuses with "owned by a configuration-file user" | The key was created while signed in as an env-config user (`ADMIN_USERNAME` etc.), which has no `users` row to attribute the proposal to | Re-issue the key while signed in as a registered account — see [Using a dedicated bot account](#using-a-dedicated-bot-account) |
| Session errors after scaling to >1 replica | `MCP_STATELESS=false` with multiple replicas | Set `MCP_STATELESS=true`, or return `mcp.replicaCount` to 1 |
| Tool returns "history_id required" | Client passed empty `history_id` parameter | history_id is mandatory due to a partial-index caveat in migration 0015; do not omit |
| Attachment fetch returns a signed URL | Binary attachment >2MB | This is expected behavior; follow the signed URL within 10 minutes |
| `GET /api/v1/proposals` returns 404 | Feature flag off on cmd/api | Set `ENABLE_MCP_SERVER=true` on the cmd/api deployment and redeploy |

---

## Rolling Back

To disable MCP temporarily:

```bash
kubectl set env deploy/alluredeck-mcp ENABLE_MCP_SERVER=false
kubectl rollout status deploy/alluredeck-mcp
```

The mcp binary exits cleanly. Database schema (proposal tables, audit_log CHECK constraint) remains intact and causes no harm.

For full removal:

```bash
helm upgrade alluredeck charts/alluredeck --set mcp.enabled=false
helm status alluredeck
```

The MCP deployment and service are deleted. The database schema does not need rollback.

---

## Known Limitations (v1)

- No OAuth 2.1 authorization server — tokens are still issued manually (API keys or UI login). The server does publish RFC 9728 protected-resource metadata at `/.well-known/oauth-protected-resource`, and 401 responses carry a `WWW-Authenticate` header pointing at it, so clients can discover the resource identifier and that bearer tokens go in the header. Dynamic client registration and CIMD are not supported.
- No server-side LLM tools — the MCP tools are read-only data tools and never call an LLM. (The optional in-product **AI failure summary** is a separate, opt-in UI/REST feature — see [Configuration → AI Failure Summaries](../configuration.md#ai-failure-summaries-llm) — not an MCP tool.)
- Mutations are proposal-only — humans approve via the admin UI (`/admin/proposals`)
- Origin-based CORS (DNS rebinding defense) — browsers with disallowed Origins receive 403
