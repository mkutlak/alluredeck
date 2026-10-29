# Deployment and Security

AllureDeck can be deployed via Docker Compose (quickest), Helm on Kubernetes, or run locally for development. All methods use the same container images.

## Docker Images

Multi-architecture images (linux/amd64 and linux/arm64) are published to GitHub Container Registry on every release:

| Image | Registry |
| ----- | -------- |
| API | `ghcr.io/mkutlak/alluredeck-api` |
| UI | `ghcr.io/mkutlak/alluredeck-ui` |
| MCP server | `ghcr.io/mkutlak/alluredeck-mcp` |

Tag scheme:
- `latest` — tracks the latest release
- `1`, `1.2`, `1.2.3` — semver tags
- `sha-<commit>` — immutable commit SHA tag

## Docker Compose

All compose files live in the `docker/` directory.

### Full Stack

The default setup runs the full AllureDeck stack — UI, API, MCP server, and PostgreSQL 18 — with local filesystem storage and security enabled.

```bash
git clone https://github.com/mkutlak/alluredeck.git
cd alluredeck
docker compose -f docker/docker-compose.yml up -d
```

| Service | URL |
| ------- | --- |
| AllureDeck UI | http://localhost:7474 |
| AllureDeck API | http://localhost:5050 |
| AllureDeck MCP server | http://localhost:8081 (see [operations/mcp-server.md](operations/mcp-server.md)) |
| PostgreSQL | `localhost:5432` (user `allure`, password `allure`, database `allure`) |

Default credentials: `admin` / `admin` and read-only `viewer` / `viewer` (change via `ADMIN_USER` / `ADMIN_PASS` / `VIEWER_USER` / `VIEWER_PASS` env vars). The MCP signing key defaults to the placeholder `dev-only-replace-in-prod`; set `MCP_SIGNING_KEY` for anything beyond local use.

Report data is persisted in the `allure-projects` Docker volume and the database in `postgres_data`. To customize, set env vars before running:

```bash
ADMIN_USER=myuser ADMIN_PASS=mypassword \
JWT_SECRET_KEY=$(openssl rand -hex 32) \
docker compose -f docker/docker-compose.yml up -d
```

### Full Stack with S3 / MinIO

Uses MinIO as an S3-compatible backend. An init container automatically creates the `allure-reports` bucket.

```bash
docker compose -f docker/docker-compose-s3.yml up -d
```

| Service | URL |
| ------- | --- |
| AllureDeck UI | http://localhost:7474 |
| AllureDeck API | http://localhost:5050 |
| AllureDeck MCP server | http://localhost:8081 |
| MinIO Console | http://localhost:9001 |

MinIO credentials: `minioadmin` / `minioadmin`. AllureDeck credentials: `admin` / `admin` (this stack defines no viewer account).

The `minio-init` service waits for MinIO to become healthy, then creates the bucket. The API waits for `minio-init` to complete before starting. For production S3, see [storage.md](storage.md#aws-s3).

### API-Only (Development)

Runs the Go API backend and PostgreSQL — useful for backend development or testing with a separately running UI dev server.

```bash
docker compose -f docker/docker-compose-dev.yml up -d
```

| Service | URL |
| ------- | --- |
| AllureDeck API | http://localhost:5050 (configurable via `ALLUREDECK_API_PORT`) |

Key differences from the full stack:
- Security disabled (`SECURITY_ENABLED=0`). With security off, role-checked endpoints answer `403 Access denied: missing claims`, so only the public endpoints (`/health`, `/ready`, `/api/v1/version`, `/api/v1/config`) are usable until you edit the file to enable security (which also needs a non-default `JWT_SECRET_KEY`)
- No UI and no MCP server; PostgreSQL is not published to the host
- No `CORS_ALLOWED_ORIGINS`, so a browser UI served from another origin cannot call it
- Mounts `../.data/alluredeck/allure-results` from the host for easy access
- Forced to `linux/amd64` platform

For UI development, prefer `mise run docker:up` or `mise run api:run` with `CORS_ALLOWED_ORIGINS=http://localhost:7474` (see [development.md](development.md#running-locally)).

### Preview Stack

`docker/docker-compose-preview.yml` runs your local checkout's UI and API against a **remote** database — for example a live environment reached through `kubectl port-forward` — so you can check a change against real data.

```bash
export PREVIEW_DATABASE_URL='postgresql://USER:PASS@localhost:15432/alluredeck?sslmode=disable'
mise run docker:up-preview     # UI on http://localhost:7475, API on port 5051
mise run docker:down-preview
```

The stack is read-only by design:

- `RUN_MIGRATIONS=false` — the remote schema stays authoritative
- `BACKGROUND_JOBS_ENABLED=false` — the preview never claims upload, report, or webhook jobs and never applies retention deletes
- Report files live in the remote environment's storage, so report iframes return 404 locally; the runs feed, dashboards, and analytics work
- Log in with `admin` / `admin` (override with `PREVIEW_ADMIN_USER` / `PREVIEW_ADMIN_PASS`); accounts stored in the remote database also work
- The API uses the host network so it can reach a `127.0.0.1`-bound port-forward

### mise Tasks

```bash
mise run docker:up        # start full stack (UI + API)
mise run docker:down      # stop full stack
mise run docker:up-s3     # full stack with MinIO
mise run docker:down-s3   # stop MinIO stack
mise run docker:up-dev    # API-only dev stack
mise run docker:down-dev  # stop dev stack
mise run docker:build     # build the API, UI, and MCP images
mise run docker:build-mcp # build the MCP image only
mise run docker:up-preview   # preview stack against a remote DB (needs PREVIEW_DATABASE_URL)
mise run docker:down-preview # stop the preview stack
```

## Helm (Kubernetes)

The Helm chart deploys AllureDeck to any Kubernetes cluster. See the [Helm Chart README](../charts/alluredeck/README.md) for the full values reference.

Current versions: app **v0.50.1**, chart **0.26.3**.

```bash
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck --version 0.26.3
```

Access without Ingress:

```bash
kubectl port-forward svc/alluredeck-ui 7474:8080
# open http://localhost:7474
```

With Ingress (single domain, path-based routing):

```bash
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck \
  --version 0.26.3 \
  --set ingress.enabled=true \
  --set ingress.host=alluredeck.example.com
```

This serves everything on one domain, eliminating CORS issues. The chart hard-codes these paths:

| Path | Backend | Condition |
| ---- | ------- | --------- |
| `/api` | API | always |
| `/trace` | API (Playwright trace viewer) | always |
| `/swagger` | API (Scalar API reference) | `ingress.swagger.enabled=true` (also needs `api.config.swaggerEnabled="true"`) |
| `/mcp` | MCP server | `mcp.enabled=true` |
| `/attachments` | API (signed attachment downloads for MCP) | `mcp.enabled=true` |
| `/` | UI | always |

Add more rules with `ingress.extraPaths`. To run the MCP server on Helm, set `mcp.enabled=true` and `api.config.mcpEnabled="true"`; see [operations/mcp-server.md](operations/mcp-server.md).

### Migrations, autoscaling, and disruption budgets

- **Migrations** — by default every API replica migrates on boot, serialized by a Postgres advisory lock. `migrationJob.enabled=true` instead runs migrations in a `pre-install,pre-upgrade` hook Job (`alluredeck-api --migrate-only`), and the API pods start with `RUN_MIGRATIONS=false`. Caveats:
  - **First install:** Helm runs the hook before it creates the chart's API ConfigMap and Secret, which the Job mounts, so the Job cannot start on `helm install`. Install with `migrationJob.enabled=false`, then enable it for upgrades.
  - **MCP pods are not covered:** the chart sets `run_migrations: false` only in the API ConfigMap, so the MCP pods keep the default and still migrate on boot.
  - **OIDC:** the Job does not receive `OIDC_CLIENT_SECRET` or `OIDC_STATE_COOKIE_SECRET`, so with `api.oidc.enabled=true` it fails configuration validation. Provide them to the Job through `api.extraEnvFrom` (applied to the Job too) or keep the Job disabled.
  - **Backups:** take a database backup before every upgrade when you use the Job. Migration `0031_surrogate_pk` cannot be rolled back, so reverting means restoring the backup and pinning the previous `api.image.tag`.
- **Autoscaling** — `autoscaling.enabled=true` creates HorizontalPodAutoscalers. The API one renders only with `api.config.storageType: s3`; local (ReadWriteOnce) storage must stay at one replica.
- **Disruption budgets** — `podDisruptionBudget.enabled=true` creates PDBs, but only for workloads that run more than one replica.

### Health probes

| Endpoint | Purpose |
| -------- | ------- |
| `GET /health` (alias `/healthz`) | Liveness. Returns `200` as soon as the listener is bound, before slow startup work finishes; no dependency checks |
| `GET /ready` (alias `/readyz`) | Readiness. Returns `503` until startup bootstrap finishes, then probes the database, storage backend, and job queue and reports each as `ok`, `error`, or `skipped` |

The chart uses `/health` for the liveness and startup probes and `/ready` for the readiness probe.

For TLS, S3, StatefulSet mode, and other Kubernetes-specific configuration see the [Helm Chart README](../charts/alluredeck/README.md).

## Local Development

To run AllureDeck locally without Docker, see [development.md](development.md).

---

## Security

The API itself starts with `SECURITY_ENABLED=false`. In that mode requests are never authenticated and carry no identity, but role checks still run, so every role-checked endpoint (projects, uploads, users, admin) returns `403 Access denied: missing claims`; only public endpoints such as `/health`, `/ready`, `/api/v1/version`, and `/api/v1/config` answer, plus viewer-level reads when `MAKE_VIEWER_ENDPOINTS_PUBLIC=true`. The full-stack Compose files and the Helm chart set `SECURITY_ENABLED=true`; set it yourself (`true` or `1`) for any other deployment you want to use.

### Authentication (JWT)

AllureDeck uses JWT (JSON Web Tokens) for stateless authentication.

#### Token types

- **Access token** — short-lived (default 1 hour / 3600s), contains username and role claim. Delivered as `jwt` cookie or `Authorization: Bearer` header.
- **Refresh token** — long-lived (default 30 days / 2592000s). Delivered as `refresh_jwt` cookie. Used to obtain new access tokens without re-login.

#### Signing

- Algorithm: HMAC-SHA256 (`HS256`)
- Key: `JWT_SECRET_KEY` env var

**Production requirement:** The API refuses to start if `SECURITY_ENABLED=true` and `JWT_SECRET_KEY` is still the default value (`super-secret-key-for-dev`). Generate a strong key:

```bash
openssl rand -hex 32
```

#### Token revocation

- Every token carries a unique **JTI** (JWT ID).
- Revoked JTIs are persisted to a `jwt_blacklist` table in PostgreSQL and survive restarts.
- A background goroutine periodically prunes expired blacklist entries.

#### Token lifetimes

Configure via env vars or YAML:

| Setting | Variable | Default |
| ------- | -------- | ------- |
| Access token TTL | `JWT_ACCESS_TOKEN_EXPIRES` | `3600` (1 hour) |
| Refresh token TTL | `JWT_REFRESH_TOKEN_EXPIRES` | `2592000` (30 days) |

### Authorization (RBAC)

Three roles are supported:

| Role | Level | Capabilities |
| ---- | ----- | ------------ |
| `admin` | 3 | Full access: create/delete projects, clean results/history, manage users, system settings, API keys |
| `editor` | 2 | Upload results, generate reports, manage known issues, set default branches |
| `viewer` | 1 | Read-only: view projects, reports, analytics, timeline |

The role is embedded in the JWT access token claims and enforced per endpoint by `RequireRole` middleware. See [Authentication](authentication.md) for OIDC SSO and role mapping details.

#### Public viewer endpoints

Set `MAKE_VIEWER_ENDPOINTS_PUBLIC=true` to skip authentication for viewer-role endpoints. This is useful for public dashboards where you want reports accessible without login.

### CSRF Protection

AllureDeck uses the **double-submit cookie** pattern for CSRF protection.

- Active only when `SECURITY_ENABLED=true`
- On login, a `csrf_token` cookie is set
- Mutating requests (POST, PUT, DELETE, PATCH) must include `X-CSRF-Token` header matching the cookie value
- Comparison uses `crypto/subtle.ConstantTimeCompare` to prevent timing attacks
- Exempt: `GET`, `HEAD`, `OPTIONS` methods and the `/login` endpoint

### Rate Limiting

Per-IP token bucket rate limiting protects the credential and token endpoints. It does **not** apply to the rest of the API.

| Limiter | Rate and burst | Routes |
| ------- | -------------- | ------ |
| Shared | 5 requests/s, burst 10 | `POST /api/v1/login`, `POST /api/v1/auth/refresh`, `POST /api/v1/users/me/password`, `GET /api/v1/projects/{project_id}/builds/{build_id}/tests/{history_id}/failure-summary`, `GET /api/v1/auth/oidc/login`, `GET /api/v1/auth/oidc/callback` (the OIDC routes exist only when `OIDC_ENABLED=true`) |
| User provisioning | 5 requests/min, burst 5 | `POST /api/v1/users`, `POST /api/v1/users/{id}/password` |

- Each IP gets an independent token bucket
- Returns `429 Too Many Requests` with `Retry-After: 1` header when the bucket is empty
- Stale IP entries are cleaned up in the background

#### Behind a reverse proxy

If AllureDeck runs behind nginx, a load balancer, or another reverse proxy, set `TRUST_FORWARDED_FOR=true` so the rate limiter uses the real client IP (the first `X-Forwarded-For` entry) rather than the proxy's IP. Enable it only when a trusted proxy sets that header, otherwise clients can spoof it.

### Security Headers

The following headers are added to every API response:

| Header | Value |
| ------ | ----- |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Content-Security-Policy` | `default-src 'self'` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=(), usb=()` |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` (only when `TLS=true`) |

Allure report pages and the trace viewer replace the CSP with a permissive one and use `frame-ancestors` (built from `CORS_ALLOWED_ORIGINS`) instead of `X-Frame-Options`, so the UI can embed them. When `CORS_ALLOWED_ORIGINS` is empty, `frame-ancestors` falls back to `*` and any site can embed reports — set `CORS_ALLOWED_ORIGINS` in production.

### TLS

The API server speaks plain HTTP only; it has no certificate or key settings. Terminate TLS at an ingress controller, load balancer, or reverse proxy in front of it.

Set `TLS=true` to tell AllureDeck that clients reach it over HTTPS. This:

- marks the `jwt`, `refresh_jwt`, and `csrf_token` cookies `Secure`
- adds the `Strict-Transport-Security` header

Without it the cookies are not restricted to HTTPS. In Kubernetes, configure TLS on the ingress (`ingress.tls`); see the [Helm Chart README](../charts/alluredeck/README.md#ingress-routing).

### Production Security Checklist

Before deploying to production:

- [ ] Set `SECURITY_ENABLED=true`
- [ ] Set `JWT_SECRET_KEY` to a strong random value (min 32 chars): `openssl rand -hex 32`
- [ ] Change the default `admin` / `admin` credentials
- [ ] Set a strong viewer password (or disable the viewer account)
- [ ] Set `CORS_ALLOWED_ORIGINS` to your UI domain (e.g. `https://alluredeck.example.com`)
- [ ] Set `TRUST_FORWARDED_FOR=true` if running behind a reverse proxy
- [ ] Serve HTTPS at the ingress, load balancer, or reverse proxy, and set `TLS=true` so cookies are `Secure` and HSTS is sent
- [ ] In Kubernetes: use `api.existingSecret` to supply credentials from a secrets manager, not plain Helm values
- [ ] If using OIDC SSO: set `OIDC_STATE_COOKIE_SECRET` to 16, 24, or 32 random bytes and store `OIDC_CLIENT_SECRET` in a Kubernetes Secret (see [Authentication](authentication.md))
- [ ] Use [API keys](features.md#api-keys-for-cicd) instead of username/password for CI/CD pipelines

## Related

- [authentication.md](authentication.md) — OIDC SSO, API keys, role mapping
- [configuration.md](configuration.md) — all environment variables
- [storage.md](storage.md) — S3/MinIO storage configuration
- [Helm Chart README](../charts/alluredeck/README.md) — full Helm chart reference
- [development.md](development.md) — local dev setup
