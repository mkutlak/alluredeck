# AllureDeck Helm Chart

[![Release Helm Chart](https://github.com/mkutlak/alluredeck/actions/workflows/release-chart.yml/badge.svg)](https://github.com/mkutlak/alluredeck/actions/workflows/release-chart.yml)
![Version: 0.4.0](https://img.shields.io/badge/Version-0.4.0-informational?style=flat-square)
![AppVersion: 0.5.0](https://img.shields.io/badge/AppVersion-0.5.0-informational?style=flat-square)
![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square)

A Helm chart for [AllureDeck](https://github.com/mkutlak/alluredeck) — an Allure Reports Dashboard that provides a centralized UI for viewing and managing Allure test reports.

The chart deploys two components:

- **API** — Go backend that stores, parses, and serves Allure test results
- **UI** — React frontend served by Nginx

## Prerequisites

- Kubernetes 1.26+
- Helm 3.10+
- **PostgreSQL 14+** database (external — see [PostgreSQL](#postgresql) section)

## Installation

Minimal install (requires a running PostgreSQL instance):

```bash
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck \
  --set api.config.databaseURL="postgres://user:password@postgres-host:5432/alluredeck?sslmode=require"
```

With a custom values file:

```bash
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck -f values.yaml
```

To pin a specific chart version:

```bash
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck --version 0.4.0
```

## PostgreSQL

AllureDeck requires PostgreSQL as its primary database. The chart does **not** deploy a PostgreSQL instance — you must provide an external database.

### Connection string

The API expects a standard PostgreSQL connection string:

```text
postgres://user:password@host:5432/dbname?sslmode=require
```

### Providing the connection string

**Option A — via `api.config.databaseURL`** (simple, but less secure):

```yaml
api:
  config:
    databaseURL: "postgres://alluredeck:secret@pg.example.com:5432/alluredeck?sslmode=require"
```

> **Note:** When set this way, the connection string appears in both the ConfigMap (plain text) and the Secret (base64-encoded). For production, prefer Option B.

**Option B — via `existingSecret`** (recommended for production):

Create a Kubernetes Secret that contains a `databaseURL` key alongside the other required credential keys:

```bash
kubectl create secret generic alluredeck-credentials \
  --namespace alluredeck \
  --from-literal=adminUser='admin' \
  --from-literal=adminPassword='<strong-password>' \
  --from-literal=viewerUser='viewer' \
  --from-literal=viewerPassword='<strong-password>' \
  --from-literal=jwtSecretKey='<64-char-random-string>' \
  --from-literal=jwtAccessTokenExpires='900' \
  --from-literal=jwtRefreshTokenExpires='2592000' \
  --from-literal=databaseURL='postgres://alluredeck:secret@pg.example.com:5432/alluredeck?sslmode=require'
```

Then reference it:

```yaml
api:
  security:
    existingSecret: alluredeck-credentials
```

This keeps the connection string out of the ConfigMap entirely.

### Schema migrations

The API runs [goose](https://github.com/pressly/goose) migrations automatically on startup. No manual schema setup is required — just point the chart at an empty database and the API will create all tables on first boot.

### Common PostgreSQL deployment options

| Option | Best for |
| ------ | -------- |
| **Managed service** (AWS RDS, Google Cloud SQL, Azure Database for PostgreSQL) | Production — automated backups, HA, patching |
| **In-cluster operator** ([CloudNativePG](https://cloudnative-pg.io/), [Zalando Postgres Operator](https://github.com/zalando/postgres-operator)) | Production on-prem or when managed services are unavailable |
| **docker-compose** | Local development and testing |

### Example: CloudNativePG

Deploy a PostgreSQL cluster with CloudNativePG, then reference the auto-created secret:

```yaml
# cnpg-cluster.yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: alluredeck-pg
spec:
  instances: 3
  storage:
    size: 10Gi
  bootstrap:
    initdb:
      database: alluredeck
      owner: alluredeck
```

```bash
kubectl apply -f cnpg-cluster.yaml
```

Then configure AllureDeck to use the CNPG connection:

```yaml
api:
  config:
    databaseURL: "postgres://alluredeck:$(PG_PASSWORD)@alluredeck-pg-rw:5432/alluredeck?sslmode=require"
```

Or inject the password from the CNPG secret using `extraEnvFrom`:

```yaml
api:
  extraEnvFrom:
    - secretRef:
        name: alluredeck-pg-app  # CNPG auto-created secret
```

### Example: AWS RDS

```yaml
api:
  config:
    databaseURL: "postgres://alluredeck:secret@alluredeck.xxxx.us-east-1.rds.amazonaws.com:5432/alluredeck?sslmode=require"
```

For IAM authentication, configure a service account with the appropriate RDS IAM role and use an init container or sidecar to handle token-based authentication.

## Configuration

### API

| Key | Description | Default |
| --- | ----------- | ------- |
| `api.image.repository` | API container image | `ghcr.io/mkutlak/alluredeck-api` |
| `api.image.tag` | Image tag | `""` (appVersion) |
| `api.config.devMode` | Enable dev mode | `"false"` |
| `api.config.logLevel` | Log level | `"info"` |
| `api.config.storageType` | File storage backend (`local` or `s3`) | `"local"` |
| `api.config.databaseURL` | PostgreSQL connection string | `""` |
| `api.config.keepHistory` | Enable test history tracking | `"true"` |
| `api.config.keepHistoryLatest` | Number of history entries to keep | `"20"` |
| `api.config.maxUploadSizeMb` | Max upload size in MB | `"100"` |
| `api.config.goMemLimit` | Go memory limit (set to ~80% of memory limit) | `"768MiB"` |
| `api.config.swaggerEnabled` | Enable Swagger UI | `"false"` |
| `api.config.makeViewerEndpointsPublic` | Allow unauthenticated read access | `"false"` |
| `api.config.corsAllowedOrigins` | CORS origins (auto-computed from ingress if empty) | `[]` |
| `api.config.checkResultsEverySeconds` | Polling interval for results | `"NONE"` |
| `api.config.trustXForwardedFor` | Trust X-Forwarded-For header | `"true"` |
| `api.kind` | Workload kind (`Deployment` or `StatefulSet`) | `Deployment` |
| `api.replicaCount` | Number of API replicas | `1` |
| `api.resources.requests.cpu` | CPU request | `100m` |
| `api.resources.requests.memory` | Memory request | `256Mi` |
| `api.resources.limits.memory` | Memory limit | `1Gi` |

### S3 / MinIO

| Key | Description | Default |
| --- | ----------- | ------- |
| `api.s3.endpoint` | S3 endpoint URL | `""` |
| `api.s3.bucket` | Bucket name | `""` |
| `api.s3.region` | AWS region | `"us-east-1"` |
| `api.s3.pathStyle` | Use path-style addressing (required for MinIO) | `"false"` |
| `api.s3.tlsInsecureSkipVerify` | Skip TLS cert verification | `"false"` |
| `api.s3.concurrency` | Upload concurrency | `"10"` |
| `api.s3.existingSecret` | Secret with `S3_ACCESS_KEY` and `S3_SECRET_KEY` | `""` |

### Security

| Key | Description | Default |
| --- | ----------- | ------- |
| `api.security.enabled` | Enable authentication | `"true"` |
| `api.security.user` | Admin username | `"admin"` |
| `api.security.password` | Admin password (random if empty) | `""` |
| `api.security.viewerUser` | Viewer username | `"viewer"` |
| `api.security.viewerPassword` | Viewer password (random if empty) | `""` |
| `api.security.jwtSecretKey` | JWT HMAC signing key (random if empty) | `""` |
| `api.security.jwtAccessTokenExpires` | Access token TTL in seconds | `"900"` |
| `api.security.jwtRefreshTokenExpires` | Refresh token TTL in seconds | `"2592000"` |
| `api.security.existingSecret` | Use a pre-created Secret | `""` |

### UI

| Key | Description | Default |
| --- | ----------- | ------- |
| `ui.image.repository` | UI container image | `ghcr.io/mkutlak/alluredeck-ui` |
| `ui.image.tag` | Image tag | `""` (appVersion) |
| `ui.config.apiUrl` | API base URL (auto-computed if empty) | `""` |
| `ui.config.appTitle` | Application title | `"AllureDeck"` |
| `ui.replicaCount` | Number of UI replicas | `1` |
| `ui.resources.requests.cpu` | CPU request | `50m` |
| `ui.resources.requests.memory` | Memory request | `64Mi` |
| `ui.resources.limits.memory` | Memory limit | `128Mi` |

### Ingress

| Key | Description | Default |
| --- | ----------- | ------- |
| `ingress.enabled` | Enable Ingress | `false` |
| `ingress.ingressClassName` | Ingress class | `""` |
| `ingress.host` | Hostname | `""` |
| `ingress.tls` | TLS configuration | `[]` |
| `ingress.api.path` | API path | `/api` |
| `ingress.swagger.enabled` | Expose Swagger UI via Ingress | `false` |
| `ingress.swagger.path` | Swagger path | `/swagger` |
| `ingress.ui.path` | UI path | `/` |

## Storage

AllureDeck supports two file storage backends for test results:

### Local filesystem (default)

```yaml
api:
  config:
    storageType: "local"
  persistence:
    projects:
      enabled: true
      size: 10Gi
```

For stable PVC binding with local storage, use a StatefulSet:

```yaml
api:
  kind: StatefulSet
```

### S3 / MinIO storage

```yaml
api:
  config:
    storageType: "s3"
  s3:
    endpoint: "http://minio.minio.svc:9000"
    bucket: "allure-reports"
    pathStyle: "true"  # required for MinIO
    existingSecret: "minio-credentials"  # contains S3_ACCESS_KEY, S3_SECRET_KEY
```

On EKS with IRSA (no static credentials needed):

```yaml
api:
  config:
    storageType: "s3"
  s3:
    bucket: "allure-reports"
    region: "us-east-1"
  serviceAccount:
    create: true
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/alluredeck-api
```

## Security and secrets

Credentials (admin password, viewer password, JWT signing key) are **auto-generated** on first install if not explicitly provided. The chart uses Helm `lookup` to preserve generated values across upgrades.

For production, pre-create a Secret with all required keys and reference it:

```yaml
api:
  security:
    existingSecret: alluredeck-credentials
```

Required Secret keys:

| Key | Description |
| --- | ----------- |
| `adminUser` | Admin username |
| `adminPassword` | Admin password |
| `viewerUser` | Viewer username |
| `viewerPassword` | Viewer password |
| `jwtSecretKey` | HMAC signing key for JWTs |
| `jwtAccessTokenExpires` | Access token TTL in seconds |
| `jwtRefreshTokenExpires` | Refresh token TTL in seconds |
| `databaseURL` | PostgreSQL connection string |

## Ingress routing

The chart creates a single unified Ingress with path-based routing:

| Path | Backend |
| ---- | ------- |
| `/` | UI |
| `/api` | API |
| `/swagger` | API (if `ingress.swagger.enabled`) |

Example with TLS:

```yaml
ingress:
  enabled: true
  ingressClassName: nginx
  host: alluredeck.example.com
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "100m"
    cert-manager.io/cluster-issuer: letsencrypt-prod
  tls:
    - secretName: alluredeck-tls
      hosts:
        - alluredeck.example.com
  swagger:
    enabled: true
```

> **Tip:** Set `nginx.ingress.kubernetes.io/proxy-body-size` to match or exceed `api.config.maxUploadSizeMb` to allow large test result uploads.

## Network policy

When `networkPolicy.enabled=true`, the chart creates NetworkPolicy resources that restrict ingress traffic:

- **API pods** — accept traffic from UI pods and (optionally) the ingress controller
- **UI pods** — accept traffic on the service port

Egress is unrestricted by default. On clusters with default-deny egress policies, add your own egress rules (e.g., to allow the API to reach PostgreSQL on port 5432) via `extraResources`.

Configure the ingress controller selector to match your setup:

```yaml
networkPolicy:
  enabled: true
  ingressController:
    podSelector:
      app.kubernetes.io/name: ingress-nginx  # adjust for traefik, AWS ALB, etc.
    namespaceSelector: {}
```

## Extra resources

Deploy arbitrary Kubernetes resources alongside the chart using `extraResources`:

```yaml
extraResources:
  - apiVersion: secrets-store.csi.x-k8s.io/v1
    kind: SecretProviderClass
    metadata:
      name: alluredeck-secrets
    spec:
      provider: aws
      parameters:
        objects: |
          - objectName: "alluredeck/prod"
            objectType: "secretsmanager"
```

## Helm tests

The chart includes connection tests for both the API and UI services:

```bash
helm test alluredeck
```

This verifies that both services are reachable within the cluster.

## Useful commands

```bash
# Lint the chart
make helm-lint

# Render templates locally (validate without installing)
make helm-template

# Package chart into a .tgz archive
make helm-package

# Bump chart version and commit
make helm-release BUMP=patch   # or minor, major
```
