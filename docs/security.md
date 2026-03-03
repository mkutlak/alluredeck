# Security

AllureDeck's security features are disabled by default for ease of local development. Set `SECURITY_ENABLED=true` (or `SECURITY_ENABLED=1`) to enable authentication and authorization.

## Authentication (JWT)

AllureDeck uses JWT (JSON Web Tokens) for stateless authentication.

### Token Types

- **Access token** — short-lived (default 15 min / 900s), contains username and role claim. Delivered as `jwt` cookie or `Authorization: Bearer` header.
- **Refresh token** — long-lived (default 30 days / 2592000s). Delivered as `refresh_jwt` cookie. Used to obtain new access tokens without re-login.

### Signing

- Algorithm: HMAC-SHA256 (`HS256`)
- Key: `JWT_SECRET_KEY` env var

**Production requirement:** The API refuses to start if `SECURITY_ENABLED=true` and `JWT_SECRET_KEY` is still the default value (`super-secret-key-for-dev`). Generate a strong key:

```bash
openssl rand -hex 32
```

### Token Revocation

- Every token carries a unique **JTI** (JWT ID).
- Revoked JTIs are persisted to a `jwt_blacklist` table in SQLite and survive restarts.
- A background goroutine periodically prunes expired blacklist entries.

### Token Lifetimes

Configure via env vars or YAML:

| Setting | Variable | Default |
|---------|----------|---------|
| Access token TTL | `JWT_ACCESS_TOKEN_EXPIRES` | `900` (15 min) |
| Refresh token TTL | `JWT_REFRESH_TOKEN_EXPIRES` | `2592000` (30 days) |

## Authorization (RBAC)

Two roles are supported:

| Role | Level | Capabilities |
|------|-------|--------------|
| `admin` | 2 | Full access: create/delete projects, generate reports, manage all data |
| `viewer` | 1 | Read-only: view projects, reports, analytics, timeline |

The role is embedded in the JWT access token claims and enforced per endpoint by `RequireRole` middleware.

### Public Viewer Endpoints

Set `MAKE_VIEWER_ENDPOINTS_PUBLIC=true` to skip authentication for viewer-role endpoints. This is useful for public dashboards where you want reports accessible without login.

## CSRF Protection

AllureDeck uses the **double-submit cookie** pattern for CSRF protection.

- Active only when `SECURITY_ENABLED=true`
- On login, a `csrf_token` cookie is set
- Mutating requests (POST, PUT, DELETE, PATCH) must include `X-CSRF-Token` header matching the cookie value
- Comparison uses `crypto/subtle.ConstantTimeCompare` to prevent timing attacks
- Exempt: `GET`, `HEAD`, `OPTIONS` methods and the `/login` endpoint

## Rate Limiting

Per-IP token bucket rate limiting protects the API from abuse:

- Each IP gets an independent token bucket
- Returns `429 Too Many Requests` with `Retry-After: 1` header when the bucket is empty
- Stale IP entries are cleaned up in the background

### Behind a Reverse Proxy

If AllureDeck runs behind nginx, a load balancer, or another reverse proxy, set `TRUST_X_FORWARDED_FOR=true` so the rate limiter uses the real client IP from the `X-Forwarded-For` header rather than the proxy's IP.

## Security Headers

The following headers are added to every API response:

| Header | Value |
|--------|-------|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Content-Security-Policy` | `default-src 'self'` |

## TLS

Enable TLS on the API server with `TLS=true`. Requires two additional env vars:

- `TLS_CERT_FILE` — path to the TLS certificate file
- `TLS_KEY_FILE` — path to the TLS private key file

In Kubernetes deployments, TLS is typically terminated at the ingress layer rather than on the application server. See [helm-chart.md](helm-chart.md) for ingress TLS configuration.

## Production Security Checklist

Before deploying to production:

- [ ] Set `SECURITY_ENABLED=true`
- [ ] Set `JWT_SECRET_KEY` to a strong random value (min 32 chars): `openssl rand -hex 32`
- [ ] Change the default `admin` / `admin` credentials
- [ ] Set a strong viewer password (or disable the viewer account)
- [ ] Set `CORS_ALLOWED_ORIGINS` to your UI domain (e.g. `https://alluredeck.example.com`)
- [ ] Set `TRUST_X_FORWARDED_FOR=true` if running behind a reverse proxy
- [ ] Use HTTPS — either via `TLS=true` on the API or at the ingress/load balancer layer
- [ ] In Kubernetes: use `api.existingSecret` to supply credentials from a secrets manager, not plain Helm values

## Related

- [configuration.md](configuration.md) — full environment variable reference
- [helm-chart.md](helm-chart.md) — Kubernetes secrets management
