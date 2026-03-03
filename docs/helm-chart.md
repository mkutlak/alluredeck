# Helm Chart Reference

AllureDeck provides an official Helm chart for Kubernetes deployment. The chart deploys the Go API backend and React UI frontend, along with all required Kubernetes resources.

## Prerequisites

- Kubernetes 1.24+
- Helm 3.x
- A StorageClass that supports `ReadWriteOnce` (for local storage mode)

## Installation

### Add the Helm Repository

```bash
helm repo add alluredeck https://mkutlak.github.io/alluredeck
helm repo update
```

### Install

```bash
# With default values (local storage, security enabled, auto-generated credentials)
helm install alluredeck alluredeck/alluredeck

# With a custom values file
helm install alluredeck alluredeck/alluredeck -f my-values.yaml

# With inline overrides
helm install alluredeck alluredeck/alluredeck \
  --set api.security.password=mysecretpassword \
  --set ui.ingress.enabled=true \
  --set ui.ingress.host=alluredeck.example.com
```

### Access (without Ingress)

```bash
# UI
kubectl port-forward svc/alluredeck-ui 7474:8080
# open http://localhost:7474

# API
kubectl port-forward svc/alluredeck-api 5050:8080
# open http://localhost:5050
```

### Upgrade

```bash
helm upgrade alluredeck alluredeck/alluredeck -f my-values.yaml
```

## Smart Defaults

The chart includes several intelligent defaults that reduce manual configuration:

- **Auto-generated credentials** — On first install, if `api.security.password`, `api.security.viewerPassword`, or `api.security.jwtSecretKey` are empty, random values are generated (32-char passwords, 64-char JWT key). On upgrade, existing secret values are preserved via Helm `lookup`.
- **Auto-computed API URL** — If `ui.config.apiUrl` is empty, the UI is configured to use the internal cluster service name automatically.
- **Auto-computed CORS** — If `api.config.corsAllowedOrigins` is empty and `ui.ingress.host` is set, CORS is automatically configured to allow the UI ingress host.

## Values Reference

### Global

| Key | Default | Description |
|-----|---------|-------------|
| `nameOverride` | `""` | Override the chart name |
| `fullnameOverride` | `""` | Override the full release name |
| `global.imagePullSecrets` | `[]` | Image pull secrets for private registries |
| `global.storageClassName` | `""` | Default StorageClass for all PVCs |

### API Image

| Key | Default | Description |
|-----|---------|-------------|
| `api.image.repository` | `ghcr.io/mkutlak/alluredeck-api` | API container image |
| `api.image.tag` | `""` | Image tag (defaults to chart `appVersion`) |
| `api.image.pullPolicy` | `IfNotPresent` | Image pull policy |

### API Configuration

These values are rendered into a `config.yaml` file mounted at `/app/alluredeck/config.yaml`.

| Key | Default | Description |
|-----|---------|-------------|
| `api.config.devMode` | `"false"` | Enable dev mode (console logging) |
| `api.config.logLevel` | `"info"` | Log level: `debug`, `info`, `warn`, `error` |
| `api.config.storageType` | `"local"` | Storage backend: `"local"` or `"s3"` |
| `api.config.checkResultsEverySeconds` | `"NONE"` | Auto-scan interval; `"NONE"` to disable |
| `api.config.keepHistory` | `"true"` | Retain report history |
| `api.config.keepHistoryLatest` | `"20"` | Max historical builds per project |
| `api.config.apiResponseLessVerbose` | `"false"` | Minimal JSON responses |
| `api.config.trustXForwardedFor` | `"true"` | Trust `X-Forwarded-For` (enabled for ingress) |
| `api.config.makeViewerEndpointsPublic` | `"false"` | Skip auth for viewer-role endpoints |
| `api.config.corsAllowedOrigins` | `[]` | Allowed CORS origins (auto-computed if empty) |
| `api.config.goMemLimit` | `"768MiB"` | Go GC memory limit (~80% of memory limit) |
| `api.config.staticContentProjects` | `"/data/projects"` | Projects data directory |
| `api.config.databasePath` | `"/data/db/alluredeck.db"` | SQLite database path |

### API S3 Settings

Used when `api.config.storageType: "s3"`.

| Key | Default | Description |
|-----|---------|-------------|
| `api.s3.endpoint` | `""` | S3/MinIO endpoint URL |
| `api.s3.bucket` | `""` | Bucket name |
| `api.s3.region` | `"us-east-1"` | AWS region |
| `api.s3.useSSL` | `"false"` | Enable TLS for S3 connections |
| `api.s3.pathStyle` | `"false"` | Path-style URLs (set `"true"` for MinIO) |
| `api.s3.concurrency` | `"10"` | Max parallel S3 operations |
| `api.s3.existingSecret` | `""` | Pre-created Secret with `S3_ACCESS_KEY`, `S3_SECRET_KEY` keys |

### API Security

| Key | Default | Description |
|-----|---------|-------------|
| `api.security.enabled` | `"true"` | Enable authentication |
| `api.security.user` | `"admin"` | Admin username |
| `api.security.password` | `""` | Admin password (random if empty on install) |
| `api.security.viewerUser` | `"viewer"` | Viewer username |
| `api.security.viewerPassword` | `""` | Viewer password (random if empty on install) |
| `api.security.jwtSecretKey` | `""` | JWT signing key (random 64-char if empty on install) |
| `api.security.jwtAccessTokenExpires` | `"900"` | Access token TTL in seconds |
| `api.security.jwtRefreshTokenExpires` | `"2592000"` | Refresh token TTL in seconds |
| `api.existingSecret` | `""` | Use a pre-created Secret instead (see below) |

### API Persistence

Two PVCs are created when `storageType=local`:

| Key | Default | Description |
|-----|---------|-------------|
| `api.persistence.projects.enabled` | `true` | Create PVC for project data (local storage only) |
| `api.persistence.projects.existingClaim` | `""` | Use an existing PVC |
| `api.persistence.projects.storageClassName` | `""` | StorageClass (falls back to `global.storageClassName`) |
| `api.persistence.projects.accessMode` | `ReadWriteOnce` | PVC access mode |
| `api.persistence.projects.size` | `10Gi` | PVC size |
| `api.persistence.database.enabled` | `true` | Create PVC for SQLite database |
| `api.persistence.database.existingClaim` | `""` | Use an existing PVC |
| `api.persistence.database.size` | `1Gi` | PVC size |

### API Resources & Scaling

| Key | Default | Description |
|-----|---------|-------------|
| `api.replicaCount` | `1` | Number of API replicas (stateful — use 1 for local storage) |
| `api.resources.requests.cpu` | `100m` | CPU request |
| `api.resources.requests.memory` | `256Mi` | Memory request |
| `api.resources.limits.memory` | `1Gi` | Memory limit |
| `api.terminationGracePeriodSeconds` | `35` | Graceful shutdown timeout |

### API Ingress

| Key | Default | Description |
|-----|---------|-------------|
| `api.ingress.enabled` | `false` | Enable Ingress for the API |
| `api.ingress.ingressClassName` | `""` | IngressClass name |
| `api.ingress.annotations` | `{}` | Ingress annotations (e.g. `cert-manager.io/cluster-issuer`) |
| `api.ingress.host` | `""` | Hostname (e.g. `api.alluredeck.example.com`) |
| `api.ingress.path` | `/` | URL path |
| `api.ingress.pathType` | `Prefix` | Path type |
| `api.ingress.tls` | `[]` | TLS configuration |

### UI

| Key | Default | Description |
|-----|---------|-------------|
| `ui.image.repository` | `ghcr.io/mkutlak/alluredeck-ui` | UI container image |
| `ui.image.tag` | `""` | Image tag (defaults to chart `appVersion`) |
| `ui.config.apiUrl` | `""` | API base URL (auto-computed if empty) |
| `ui.config.appTitle` | `"AllureDeck"` | Browser tab title and brand text |
| `ui.replicaCount` | `1` | Number of UI replicas |
| `ui.resources.requests.cpu` | `50m` | CPU request |
| `ui.resources.requests.memory` | `64Mi` | Memory request |
| `ui.resources.limits.memory` | `128Mi` | Memory limit |

UI Ingress keys follow the same pattern as `api.ingress.*`.

### Service Accounts

| Key | Default | Description |
|-----|---------|-------------|
| `serviceAccount.api.create` | `true` | Create a ServiceAccount for the API |
| `serviceAccount.api.name` | `""` | ServiceAccount name (auto-generated if empty) |
| `serviceAccount.api.annotations` | `{}` | Annotations (use for IRSA: `eks.amazonaws.com/role-arn`) |
| `serviceAccount.ui.create` | `true` | Create a ServiceAccount for the UI |

### Network Policy

| Key | Default | Description |
|-----|---------|-------------|
| `networkPolicy.enabled` | `false` | Enable NetworkPolicies restricting API/UI ingress |
| `networkPolicy.ingressController.podSelector` | `{app.kubernetes.io/name: ingress-nginx}` | Ingress controller pod selector |
| `networkPolicy.ingressController.namespaceSelector` | `{}` | Ingress controller namespace selector |

## Secrets Management

### Auto-generated Secrets (default)

On first `helm install`, if credential values are empty, random values are generated:
- Admin password: 32 random alphanumeric characters
- Viewer password: 32 random alphanumeric characters
- JWT secret key: 64 random alphanumeric characters

On subsequent `helm upgrade`, existing values are read from the cluster via Helm `lookup` and preserved. Secrets are **not portable** across clusters or release names.

### Using an Existing Secret

To supply credentials from an external secrets manager (AWS Secrets Manager, Vault, etc.):

```bash
kubectl create secret generic alluredeck-credentials \
  --from-literal=adminUser=admin \
  --from-literal=adminPassword=mysecretpassword \
  --from-literal=viewerUser=viewer \
  --from-literal=viewerPassword=viewersecret \
  --from-literal=jwtSecretKey=$(openssl rand -hex 32) \
  --from-literal=jwtAccessTokenExpires=900 \
  --from-literal=jwtRefreshTokenExpires=2592000
```

Then reference it in your values:

```yaml
api:
  existingSecret: "alluredeck-credentials"
```

### Secret Rotation

After rotating credentials, trigger a rolling restart:

```bash
kubectl rollout restart deployment/alluredeck-api
```

## Example Values Files

### Minimal Production (local storage + ingress + TLS)

```yaml
api:
  security:
    password: ""         # auto-generated on install
    viewerPassword: ""   # auto-generated on install
    jwtSecretKey: ""     # auto-generated on install
  ingress:
    enabled: true
    ingressClassName: nginx
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod
    host: api.alluredeck.example.com
    tls:
      - secretName: alluredeck-api-tls
        hosts:
          - api.alluredeck.example.com

ui:
  ingress:
    enabled: true
    ingressClassName: nginx
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod
    host: alluredeck.example.com
    tls:
      - secretName: alluredeck-ui-tls
        hosts:
          - alluredeck.example.com
```

### S3 Storage with IRSA on EKS

```yaml
serviceAccount:
  api:
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/alluredeck-api-s3

api:
  config:
    storageType: "s3"
  s3:
    endpoint: "https://s3.amazonaws.com"
    bucket: "my-allure-reports"
    region: "eu-west-1"
    useSSL: "true"
    pathStyle: "false"
  persistence:
    projects:
      enabled: false  # not needed for S3 storage
```

### MinIO in Cluster

```yaml
api:
  config:
    storageType: "s3"
  s3:
    endpoint: "http://minio.minio.svc:9000"
    bucket: "allure-reports"
    region: "us-east-1"
    useSSL: "false"
    pathStyle: "true"
    existingSecret: "minio-credentials"
  persistence:
    projects:
      enabled: false
```

## Related

- [configuration.md](configuration.md) — full environment variable reference
- [security.md](security.md) — authentication and security configuration
- [storage.md](storage.md) — S3/MinIO storage setup
- [deployment.md](deployment.md) — all deployment methods
