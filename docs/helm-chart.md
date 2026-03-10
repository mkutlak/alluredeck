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
  --set ingress.enabled=true \
  --set ingress.host=alluredeck.example.com
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
- **Auto-computed API URL** — When `ingress.enabled` is true and `ingress.host` is set, the UI automatically uses a relative `/api/v1` path (same-origin, no CORS needed). Otherwise falls back to the internal cluster service URL.
- **Auto-computed CORS** — If `api.config.corsAllowedOrigins` is empty and `ingress.host` is set, CORS is automatically configured to allow the ingress host origin.

## Values Reference

### Global

| Key | Default | Description |
|-----|---------|-------------|
| `nameOverride` | `""` | Override the chart name |
| `fullnameOverride` | `""` | Override the full release name |
| `global.imagePullSecrets` | `[]` | Image pull secrets for private registries |
| `global.storageClassName` | `""` | Default StorageClass for all PVCs |

### Ingress

A single unified Ingress resource with path-based routing serves both the UI and API on the same domain. This eliminates CORS issues by keeping UI and API on the same origin.

| Key | Default | Description |
|-----|---------|-------------|
| `ingress.enabled` | `false` | Enable the unified Ingress |
| `ingress.ingressClassName` | `""` | IngressClass name (e.g. `nginx`) |
| `ingress.annotations` | `{}` | Ingress annotations (e.g. `cert-manager.io/cluster-issuer`) |
| `ingress.host` | `""` | Hostname (e.g. `alluredeck.example.com`) |
| `ingress.tls` | `[]` | TLS configuration |
| `ingress.api.path` | `/api` | Path routed to the API service |
| `ingress.api.pathType` | `Prefix` | API path type |
| `ingress.ui.path` | `/` | Path routed to the UI service |
| `ingress.ui.pathType` | `Prefix` | UI path type |

With this setup, a single domain serves:
- `https://alluredeck.example.com/` → UI
- `https://alluredeck.example.com/api/` → API

### API Image

| Key | Default | Description |
|-----|---------|-------------|
| `api.image.repository` | `ghcr.io/mkutlak/alluredeck-api` | API container image |
| `api.image.tag` | `""` | Image tag (defaults to chart `appVersion`) |
| `api.image.pullPolicy` | `IfNotPresent` | Image pull policy |

### API Workload Kind

| Key | Default | Description |
|-----|---------|-------------|
| `api.kind` | `Deployment` | Workload type: `Deployment` or `StatefulSet` |

- **Deployment** (default): Uses standalone PVCs. Fine for S3 storage or ephemeral/dev usage.
- **StatefulSet**: Uses `volumeClaimTemplates` for stable PVC binding. Recommended for local storage in production. Automatically creates a headless Service for stable pod DNS.

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
| `api.config.databaseURL` | `""` | PostgreSQL connection string (e.g. `postgres://alluredeck:pass@db:5432/alluredeck?sslmode=disable`) |

### API S3 Settings

Used when `api.config.storageType: "s3"`.

| Key | Default | Description |
|-----|---------|-------------|
| `api.s3.endpoint` | `""` | S3/MinIO endpoint URL |
| `api.s3.bucket` | `""` | Bucket name |
| `api.s3.region` | `"us-east-1"` | AWS region |
| `api.s3.tlsInsecureSkipVerify` | `"false"` | Skip TLS certificate verification (e.g. self-signed certs) |
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
| `api.security.existingSecret` | `""` | Use a pre-created Secret instead (see below) |

### API Persistence

Two PVCs are managed for the API. In `Deployment` mode they are standalone PVCs; in `StatefulSet` mode they are `volumeClaimTemplates`.

| Key | Default | Description |
|-----|---------|-------------|
| `api.persistence.projects.enabled` | `true` | Create PVC for project data (local storage only) |
| `api.persistence.projects.existingClaim` | `""` | Use an existing PVC (Deployment mode only) |
| `api.persistence.projects.storageClassName` | `""` | StorageClass (falls back to `global.storageClassName`) |
| `api.persistence.projects.accessMode` | `ReadWriteOnce` | PVC access mode |
| `api.persistence.projects.size` | `10Gi` | PVC size |
| `api.persistence.database.enabled` | `true` | Create PVC for PostgreSQL data |
| `api.persistence.database.existingClaim` | `""` | Use an existing PVC (Deployment mode only) |
| `api.persistence.database.size` | `1Gi` | PVC size |

### API Resources & Scaling

| Key | Default | Description |
|-----|---------|-------------|
| `api.replicaCount` | `1` | Number of API replicas (stateful — use 1 for local storage) |
| `api.resources.requests.cpu` | `100m` | CPU request |
| `api.resources.requests.memory` | `256Mi` | Memory request |
| `api.resources.limits.memory` | `1Gi` | Memory limit |
| `api.terminationGracePeriodSeconds` | `35` | Graceful shutdown timeout |

### API Pod Customization

| Key | Default | Description |
|-----|---------|-------------|
| `api.strategy` | `{}` | Deployment update strategy (or StatefulSet `updateStrategy`) |
| `api.extraArgs` | `[]` | Additional CLI arguments for the API binary |
| `api.extraEnv` | `[]` | Additional environment variables |
| `api.extraEnvFrom` | `[]` | Additional env sources (ConfigMap/Secret refs) |
| `api.extraVolumes` | `[]` | Additional volumes |
| `api.extraVolumeMounts` | `[]` | Additional volume mounts |
| `api.initContainers` | `[]` | Init containers |
| `api.sidecars` | `[]` | Sidecar containers |
| `api.lifecycle` | `{}` | Container lifecycle hooks |
| `api.priorityClassName` | `""` | Pod priority class |
| `api.dnsPolicy` | `""` | DNS policy |
| `api.dnsConfig` | `{}` | DNS configuration |
| `api.nodeSelector` | `{}` | Node selector |
| `api.tolerations` | `[]` | Tolerations |
| `api.affinity` | `{}` | Affinity rules |
| `api.topologySpreadConstraints` | `[]` | Topology spread constraints |
| `api.podAnnotations` | `{}` | Additional pod annotations |
| `api.podLabels` | `{}` | Additional pod labels |

### UI

| Key | Default | Description |
|-----|---------|-------------|
| `ui.image.repository` | `ghcr.io/mkutlak/alluredeck-ui` | UI container image |
| `ui.image.tag` | `""` | Image tag (defaults to chart `appVersion`) |
| `ui.config.apiUrl` | `""` | API base URL (auto-computed if empty — see Smart Defaults) |
| `ui.config.appTitle` | `"AllureDeck"` | Browser tab title and brand text |
| `ui.replicaCount` | `1` | Number of UI replicas |
| `ui.resources.requests.cpu` | `50m` | CPU request |
| `ui.resources.requests.memory` | `64Mi` | Memory request |
| `ui.resources.limits.memory` | `128Mi` | Memory limit |

UI pod customization keys follow the same pattern as API (`ui.extraEnv`, `ui.extraVolumes`, `ui.strategy`, etc.).

### Service Accounts

| Key | Default | Description |
|-----|---------|-------------|
| `api.serviceAccount.create` | `false` | Create a ServiceAccount for the API |
| `api.serviceAccount.name` | `""` | ServiceAccount name (auto-generated if empty) |
| `api.serviceAccount.annotations` | `{}` | Annotations (use for IRSA: `eks.amazonaws.com/role-arn`) |
| `ui.serviceAccount.create` | `false` | Create a ServiceAccount for the UI |
| `ui.serviceAccount.name` | `""` | ServiceAccount name (auto-generated if empty) |
| `ui.serviceAccount.annotations` | `{}` | Annotations |

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
  security:
    existingSecret: "alluredeck-credentials"
```

### Secret Rotation

After rotating credentials, trigger a rolling restart:

```bash
# For Deployment mode (default)
kubectl rollout restart deployment/alluredeck-api

# For StatefulSet mode
kubectl rollout restart statefulset/alluredeck-api
```

## Example Values Files

### Minimal Production (unified ingress + TLS)

```yaml
ingress:
  enabled: true
  ingressClassName: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  host: alluredeck.example.com
  tls:
    - secretName: alluredeck-tls
      hosts:
        - alluredeck.example.com
```

This serves both UI and API on `alluredeck.example.com`:
- `https://alluredeck.example.com/` → UI
- `https://alluredeck.example.com/api/` → API

CORS is auto-configured and the UI automatically uses relative `/api/v1` paths (same-origin).

### StatefulSet with Local Storage (production)

```yaml
api:
  kind: StatefulSet
  persistence:
    projects:
      size: 50Gi
    database:
      size: 5Gi

ingress:
  enabled: true
  ingressClassName: nginx
  host: alluredeck.example.com
  tls:
    - secretName: alluredeck-tls
      hosts:
        - alluredeck.example.com
```

StatefulSet provides stable PVC binding — PVCs are managed via `volumeClaimTemplates` and are not deleted when the StatefulSet is scaled down. A headless Service is automatically created for stable pod DNS.

### S3 Storage with IRSA on EKS

```yaml
api:
  serviceAccount:
    create: true
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/alluredeck-api-s3
  config:
    storageType: "s3"
  s3:
    endpoint: "https://s3.amazonaws.com"
    bucket: "my-allure-reports"
    region: "eu-west-1"
    pathStyle: "false"
  persistence:
    projects:
      enabled: false  # not needed for S3 storage

ingress:
  enabled: true
  ingressClassName: nginx
  host: alluredeck.example.com
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
