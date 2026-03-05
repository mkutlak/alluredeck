# Deployment

AllureDeck can be deployed via Docker Compose (quickest), Helm on Kubernetes, or run locally for development. All methods use the same container images.

## Docker Images

Multi-architecture images (linux/amd64 and linux/arm64) are published to GitHub Container Registry on every release:

| Image | Registry |
|-------|----------|
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
cd alluredeck/alluredeck
docker compose -f docker/docker-compose.yml up -d
```

| Service | URL |
|---------|-----|
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
|---------|-----|
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
|---------|-----|
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

The Helm chart deploys AllureDeck to any Kubernetes cluster. See [helm-chart.md](helm-chart.md) for the full values reference.

```bash
helm repo add alluredeck https://mkutlak.github.io/alluredeck
helm repo update
helm install alluredeck alluredeck/alluredeck
```

Access without Ingress:
```bash
kubectl port-forward svc/alluredeck-ui 7474:8080
# open http://localhost:7474
```

With Ingress (single domain, path-based routing):
```bash
helm install alluredeck alluredeck/alluredeck \
  --set ingress.enabled=true \
  --set ingress.host=alluredeck.example.com
```

This serves UI at `/` and API at `/api` on the same domain, eliminating CORS issues.

For TLS, S3, StatefulSet mode, and other Kubernetes-specific configuration see [helm-chart.md](helm-chart.md).

## Local Development

To run AllureDeck locally without Docker, see [development.md](development.md).

## Related

- [configuration.md](configuration.md) — all environment variables
- [storage.md](storage.md) — S3/MinIO storage configuration
- [security.md](security.md) — authentication and security
- [helm-chart.md](helm-chart.md) — full Helm chart reference
- [development.md](development.md) — local dev setup
