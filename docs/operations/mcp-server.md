# MCP Server Operations

The Model Context Protocol (MCP) server enables AI clients (Claude Code, Cursor, Claude Desktop) to query test-failure data and propose mutations against AllureDeck. See [modelcontextprotocol.io](https://modelcontextprotocol.io) for the MCP specification.

---

## Prerequisites

- AllureDeck v0.13.0 or later (TODO: confirm after Phase 4 merge)
- PostgreSQL with migration 0041 applied (`defect_proposals`, `known_issue_proposals`, `flaky_proposals` tables)
- An API key store row with `allow_mcp_writes` configured if proposals are needed

---

## Enabling MCP

### Environment Variables

On the **cmd/mcp** deployment:

```bash
ENABLE_MCP_SERVER=true
MCP_ALLOWED_ORIGINS=https://your.host
MCP_RATE_LIMIT_PER_MIN=60
MCP_RATE_LIMIT_BURST=10
```

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

1. User logs in to the AllureDeck UI
2. Navigate to **Settings → API Keys**
3. Click **Create API Key**
4. Choose role (typically **editor** to allow proposals)
5. Toggle **Allow MCP writes** if proposals are needed
6. Copy the token (format: `ald_<base64>`)

### Machine Tokens (CI)

An admin creates an API key via the admin API endpoint:

```bash
curl -X POST http://localhost:8080/api/v1/admin/api-keys \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "ci-mcp",
    "role": "editor",
    "allow_mcp_writes": true
  }'
```

Token format is always `ald_<base64>`, compatible with the REST API.

---

## Configuring a Client

### Claude Code

Add to `~/.claude/mcp.json`:

```json
{
  "mcp-servers": {
    "alluredeck": {
      "command": "npx",
      "args": ["@modelcontextprotocol/server-stdio"],
      "env": {
        "MCP_ALLOWED_ORIGINS": "https://your.host"
      }
    }
  },
  "mcpServers": {
    "alluredeck": {
      "url": "https://your.host/mcp",
      "auth": {
        "type": "bearer",
        "token": "ald_<base64>"
      }
    }
  }
}
```

### Cursor

In Cursor settings, go to **Features → MCP Servers** and add:

```json
{
  "name": "AllureDeck",
  "url": "https://your.host/mcp",
  "auth": {
    "type": "bearer",
    "token": "ald_<base64>"
  }
}
```

### Claude Desktop

Edit `~/.config/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "alluredeck": {
      "command": "curl",
      "args": [
        "-H", "Authorization: Bearer ald_<base64>",
        "https://your.host/mcp"
      ]
    }
  }
}
```

Restart Claude Desktop after editing.

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

- No OAuth 2.1 discovery endpoint — manual token entry only
- No server-side LLM tools — read-only data tools only
- Mutations are proposal-only — humans approve via the admin UI (`/admin/proposals`)
- Origin-based CORS (DNS rebinding defense) — browsers with disallowed Origins receive 403
