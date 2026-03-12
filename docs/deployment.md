# Deployment and Security

AllureDeck can be deployed via Docker Compose (quickest), Helm on Kubernetes, or run locally for development. All methods use the same container images.

## Docker Images

Multi-architecture images (linux/amd64 and linux/arm64) are published to GitHub Container Registry on every release:

| Image | Registry |
| ----- | -------- |
| API | `ghcr.io/mkutlak/alluredeck-api` |
| UI | `ghcr.io/mkutlak/alluredeck-ui` |

Tag scheme:
- `latest` — tracks the latest release
- `1`, `1.2`, `1.2.3` — semver tags
- `sha-<commit>` — immutable commit SHA tag

## Docker Compose

All compose files live in the `docker/` directory.

### Full Stack (UI + API)

The default setup runs the full AllureDeck stack with local filesystem storage and security enabled.

```bash
git clone https://github.com/mkutlak/alluredeck.git
cd alluredeck
docker compose -f docker/docker-compose.yml up -d
```

| Service | URL |
| ------- | --- |
| AllureDeck UI | http://localhost:7474 |
| AllureDeck API | http://localhost:5050 |

Default credentials: `admin` / `admin` (change via `ADMIN_USER` / `ADMIN_PASS` env vars).

Report data is persisted in a Docker volume named `allure-projects`. To customize, set env vars before running:

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
| MinIO Console | http://localhost:9001 |

MinIO credentials: `minioadmin` / `minioadmin`. AllureDeck credentials: `admin` / `admin`.

The `minio-init` service waits for MinIO to become healthy, then creates the bucket. The API waits for `minio-init` to complete before starting. For production S3, see [storage.md](storage.md#aws-s3).

### API-Only (Development)

Runs only the Go API backend — useful for backend development or testing with a separately running UI dev server.

```bash
docker compose -f docker/docker-compose-dev.yml up -d
```

| Service | URL |
| ------- | --- |
| AllureDeck API | http://localhost:5050 (configurable via `ALLUREDECK_API_PORT`) |

Key differences from the full stack:
- Security disabled by default (`SECURITY_ENABLED=0`)
- Mounts `../.data/alluredeck/allure-results` from the host for easy access
- Forced to `linux/amd64` platform

### Make Targets

```bash
make docker-up        # start full stack (UI + API)
make docker-down      # stop full stack
make docker-up-s3     # full stack with MinIO
make docker-down-s3   # stop MinIO stack
make docker-up-dev    # API-only dev stack
make docker-down-dev  # stop dev stack
make docker-build     # build both Docker images
```

## Helm (Kubernetes)

The Helm chart deploys AllureDeck to any Kubernetes cluster. See the [Helm Chart README](../charts/alluredeck/README.md) for the full values reference.

```bash
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck
```

Access without Ingress:

```bash
kubectl port-forward svc/alluredeck-ui 7474:8080
# open http://localhost:7474
```

With Ingress (single domain, path-based routing):

```bash
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck \
  --set ingress.enabled=true \
  --set ingress.host=alluredeck.example.com
```

This serves UI at `/` and API at `/api` on the same domain, eliminating CORS issues.

For TLS, S3, StatefulSet mode, and other Kubernetes-specific configuration see the [Helm Chart README](../charts/alluredeck/README.md).

## Local Development

To run AllureDeck locally without Docker, see [development.md](development.md).

---

## Security

AllureDeck's security features are disabled by default for ease of local development. Set `SECURITY_ENABLED=true` (or `SECURITY_ENABLED=1`) to enable authentication and authorization.

### Authentication (JWT)

AllureDeck uses JWT (JSON Web Tokens) for stateless authentication.

#### Token types

- **Access token** — short-lived (default 15 min / 900s), contains username and role claim. Delivered as `jwt` cookie or `Authorization: Bearer` header.
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
| Access token TTL | `JWT_ACCESS_TOKEN_EXPIRES` | `900` (15 min) |
| Refresh token TTL | `JWT_REFRESH_TOKEN_EXPIRES` | `2592000` (30 days) |

### Authorization (RBAC)

Two roles are supported:

| Role | Level | Capabilities |
| ---- | ----- | ------------ |
| `admin` | 2 | Full access: create/delete projects, generate reports, manage all data |
| `viewer` | 1 | Read-only: view projects, reports, analytics, timeline |

The role is embedded in the JWT access token claims and enforced per endpoint by `RequireRole` middleware.

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

Per-IP token bucket rate limiting protects the API from abuse:

- Each IP gets an independent token bucket
- Returns `429 Too Many Requests` with `Retry-After: 1` header when the bucket is empty
- Stale IP entries are cleaned up in the background

#### Behind a reverse proxy

If AllureDeck runs behind nginx, a load balancer, or another reverse proxy, set `TRUST_FORWARDED_FOR=true` so the rate limiter uses the real client IP from the `X-Forwarded-For` header rather than the proxy's IP.

### Security Headers

The following headers are added to every API response:

| Header | Value |
| ------ | ----- |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Content-Security-Policy` | `default-src 'self'` |

### TLS

Enable TLS on the API server with `TLS=true`. Requires two additional env vars:

- `TLS_CERT_FILE` — path to the TLS certificate file
- `TLS_KEY_FILE` — path to the TLS private key file

In Kubernetes deployments, TLS is typically terminated at the ingress layer rather than on the application server. See the [Helm Chart README](../charts/alluredeck/README.md#ingress-routing) for ingress TLS configuration.

### Production Security Checklist

Before deploying to production:

- [ ] Set `SECURITY_ENABLED=true`
- [ ] Set `JWT_SECRET_KEY` to a strong random value (min 32 chars): `openssl rand -hex 32`
- [ ] Change the default `admin` / `admin` credentials
- [ ] Set a strong viewer password (or disable the viewer account)
- [ ] Set `CORS_ALLOWED_ORIGINS` to your UI domain (e.g. `https://alluredeck.example.com`)
- [ ] Set `TRUST_FORWARDED_FOR=true` if running behind a reverse proxy
- [ ] Use HTTPS — either via `TLS=true` on the API or at the ingress/load balancer layer
- [ ] In Kubernetes: use `api.existingSecret` to supply credentials from a secrets manager, not plain Helm values

## Related

- [configuration.md](configuration.md) — all environment variables
- [storage.md](storage.md) — S3/MinIO storage configuration
- [Helm Chart README](../charts/alluredeck/README.md) — full Helm chart reference
- [development.md](development.md) — local dev setup
