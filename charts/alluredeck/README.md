# AllureDeck Helm Chart

[![Release Helm Chart](https://github.com/mkutlak/alluredeck/actions/workflows/release-chart.yml/badge.svg)](https://github.com/mkutlak/alluredeck/actions/workflows/release-chart.yml)
![Version: 0.16.0](https://img.shields.io/badge/Version-0.16.0-informational?style=flat-square)
![AppVersion: 0.31.0](https://img.shields.io/badge/AppVersion-0.31.0-informational?style=flat-square)
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
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck --version 0.13.0
```

**Upgrade an existing release** (preserves auto-generated credentials and previous values):

```bash
helm upgrade --install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck \
  --version 0.13.0 \
  -f values.yaml
```

Use `--reuse-values` to preserve prior `--set` flags without re-specifying them; use `-f values.yaml` when your values file is the source of truth. The chart uses Helm `lookup` to preserve any auto-generated admin / viewer / JWT credentials across upgrades, so you don't need to re-supply them.

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
  --from-literal=jwtAccessTokenExpires='3600' \
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

### Global

`global.*` values are cluster-wide defaults inherited by any sub-chart key that doesn't set its own value (most commonly PVC `storageClassName`).

| Key | Description | Default |
| --- | ----------- | ------- |
| `global.imagePullSecrets` | List of image pull secret names injected into every Deployment / StatefulSet | `[]` |
| `global.storageClassName` | Default storage class used by `api.persistence.projects` when it doesn't set its own | `""` |

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
| `api.config.keepHistoryLatest` | Number of history entries to keep (0 = unlimited) | `"100"` |
| `api.config.keepHistoryMaxAgeDays` | Delete reports older than N days (0 = disabled) | `"0"` |
| `api.config.maxUploadSizeMb` | Max upload size in MB | `"100"` |
| `api.config.apiResponseLessVerbose` | Return minimal JSON responses (omit verbose fields) | `"false"` |
| `api.config.staticContentProjects` | Root directory for static project content (embedded report overlay file server) | `"/data/projects"` |
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

> **`api.kind` tip:** use `StatefulSet` when `storageType=local` so the API pod re-binds to the same PVC on restart. Use `Deployment` for S3/MinIO storage or ephemeral dev environments where no PVC is needed.

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
| `api.security.jwtAccessTokenExpires` | Access token TTL in seconds | `"3600"` |
| `api.security.jwtRefreshTokenExpires` | Refresh token TTL in seconds | `"2592000"` |
| `api.security.existingSecret` | Use a pre-created Secret | `""` |

### OIDC SSO

OpenID Connect SSO is **disabled by default**. When enabled, users can sign in via any OIDC-compliant identity provider (Azure AD / Entra ID, Keycloak, Okta, Google Workspace). Local authentication (`admin` / `viewer`) continues to work alongside SSO as a break-glass fallback.

| Key | Description | Default |
| --- | ----------- | ------- |
| `api.oidc.enabled` | Enable OIDC SSO | `false` |
| `api.oidc.issuerUrl` | IdP discovery URL (e.g. `https://login.microsoftonline.com/{tenant}/v2.0`) | `""` |
| `api.oidc.clientId` | OIDC client ID | `""` |
| `api.oidc.clientSecret` | Confidential client secret (stored in Secret — prefer `existingSecret` in prod) | `""` |
| `api.oidc.redirectUrl` | Callback URL; must match `https://<host>/api/v1/auth/oidc/callback` | `""` |
| `api.oidc.scopes` | Comma-separated OIDC scopes | `"openid,profile,email"` |
| `api.oidc.groupsClaim` | Claim name containing group memberships | `"groups"` |
| `api.oidc.adminGroups` | Comma-separated group IDs that map to the `admin` role | `""` |
| `api.oidc.editorGroups` | Comma-separated group IDs that map to the `editor` role | `""` |
| `api.oidc.defaultRole` | Role for users not matching any group (`admin`, `editor`, or `viewer`) | `"viewer"` |
| `api.oidc.stateCookieSecret` | AES-GCM key (exactly 16, 24, or 32 bytes) for state-cookie encryption; empty = auto-generated | `""` |
| `api.oidc.postLoginRedirect` | Frontend URL after successful SSO login | `"/"` |
| `api.oidc.endSessionUrl` | Optional RP-initiated logout URL | `""` |

**Role mapping:** AllureDeck has three roles — `admin` (full control), `editor` (upload, generate reports, manage known issues), `viewer` (read-only). On each OIDC login the API reads the claim named by `groupsClaim` from the ID token and matches each group against `adminGroups` → `editorGroups` → `defaultRole` (first match wins).

**Example — Azure AD (Entra ID):**

```yaml
api:
  oidc:
    enabled: true
    issuerUrl: "https://login.microsoftonline.com/{tenant-id}/v2.0"
    clientId: "{app-registration-client-id}"
    redirectUrl: "https://alluredeck.example.com/api/v1/auth/oidc/callback"
    groupsClaim: "groups"
    adminGroups: "{admin-group-object-id}"
    editorGroups: "{editor-group-object-id}"
    defaultRole: "viewer"
  security:
    existingSecret: alluredeck-credentials   # provides oidcClientSecret + oidcStateCookieSecret
```

See [docs/authentication.md](../../docs/authentication.md) for Keycloak, Okta, Google Workspace walk-throughs and the full OIDC feature reference (PKCE, JIT provisioning, state cookie encryption, JWT blacklist).

**IRSA on EKS for OIDC:** Use the same `api.serviceAccount.annotations` pattern as S3. If your OIDC client secret is stored in AWS Secrets Manager, mount it via the AWS Secrets Store CSI driver (see [Extra resources](#extra-resources)) and reference it from `api.security.existingSecret`.

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

| Key | Description | Required when |
| --- | ----------- | ------------- |
| `adminUser` | Admin username | always |
| `adminPassword` | Admin password | always |
| `viewerUser` | Viewer username | always |
| `viewerPassword` | Viewer password | always |
| `jwtSecretKey` | HMAC signing key for JWTs | always |
| `jwtAccessTokenExpires` | Access token TTL in seconds | always |
| `jwtRefreshTokenExpires` | Refresh token TTL in seconds | always |
| `databaseURL` | PostgreSQL connection string | always |
| `oidcClientSecret` | OIDC confidential client secret | `api.oidc.enabled=true` |
| `oidcStateCookieSecret` | AES-GCM key for state cookie encryption (exactly 16/24/32 bytes) | `api.oidc.enabled=true` |
| `S3_ACCESS_KEY` | S3 access key ID | `api.s3.existingSecret` used and not on IRSA |
| `S3_SECRET_KEY` | S3 secret access key | `api.s3.existingSecret` used and not on IRSA |

**Tip:** you can put all of these keys in a single `Secret` and reference it from each of `api.security.existingSecret`, `api.s3.existingSecret`, and `api.oidc.existingSecret` (whichever the chart wires up for each section) — that way one Secret resource holds every credential the chart needs.

**3-role RBAC:** the chart wires three roles — `admin`, `editor`, and `viewer`. With local auth only, the `admin` account (from `adminUser`/`adminPassword`) has role `admin` and `viewerUser`/`viewerPassword` has role `viewer`; there's no local "editor" account. With OIDC enabled, group-based role mapping via `api.oidc.adminGroups` / `api.oidc.editorGroups` / `api.oidc.defaultRole` promotes SSO users into any of the three roles. See the [OIDC SSO](#oidc-sso) section above.

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

### Extra ingress paths

The chart's Ingress has hardcoded `/api` → API, `/trace` → API, and `/` → UI routes. If you need to expose additional services under the same host (e.g. a Grafana dashboard on `/grafana`), use `ingress.extraPaths` to inject extra rules **before** the catch-all UI rule:

```yaml
ingress:
  enabled: true
  ingressClassName: nginx
  host: alluredeck.example.com
  extraPaths:
    - path: /grafana
      pathType: Prefix
      serviceName: grafana
      servicePortName: http
```

Each extra path becomes an additional backend on the same Ingress resource, inheriting all host-level TLS and annotations.

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
