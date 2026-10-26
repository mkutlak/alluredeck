# AllureDeck Helm Chart

[![Release Helm Chart](https://github.com/mkutlak/alluredeck/actions/workflows/release-chart.yml/badge.svg)](https://github.com/mkutlak/alluredeck/actions/workflows/release-chart.yml)
![Version: 0.26.1](https://img.shields.io/badge/Version-0.26.1-informational?style=flat-square)
![AppVersion: 0.50.0](https://img.shields.io/badge/AppVersion-0.50.0-informational?style=flat-square)
![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square)

A Helm chart for [AllureDeck](https://github.com/mkutlak/alluredeck) — an Allure Reports Dashboard that provides a centralized UI for viewing and managing Allure test reports.

The chart deploys three components:

- **API** — Go backend that stores, parses, and serves Allure test results
- **UI** — React frontend served by Nginx
- **MCP** — optional Model Context Protocol server for AI-assisted test analysis (disabled by default; see [MCP server](#mcp-server))

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
helm install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck --version 0.26.1
```

**Upgrade an existing release** (preserves auto-generated credentials and previous values):

```bash
helm upgrade --install alluredeck oci://ghcr.io/mkutlak/charts/alluredeck \
  --version 0.26.1 \
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

By default every API replica runs [goose](https://github.com/pressly/goose) and River migrations on startup (`RUN_MIGRATIONS=true`). No manual schema setup is required — point the chart at an empty database and the API creates all tables on first boot. Replicas that start together are serialised by a PostgreSQL advisory lock, so only one applies the migrations while the others wait (bounded by `MIGRATION_TIMEOUT`, default `5m`).

To apply migrations before the new API pods start instead, enable the pre-upgrade [Migration Job](#migration-job). API replicas then skip migrations on boot; MCP pods (if enabled) still run them at boot, serialised by the same advisory lock.

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

### General

| Key | Description | Default |
| --- | ----------- | ------- |
| `nameOverride` | Override the chart name used in resource names | `""` |
| `fullnameOverride` | Override the full resource name prefix | `""` |
| `externalUrl` | Public base URL of the deployment (for example `https://alluredeck.example.com`). Used to sign absolute attachment download URLs and to build proposal review links. Empty = derived from `ingress.host` only when `ingress.enabled=true` (`https` when `ingress.tls` is set, otherwise `http`); otherwise no external URL is set. With MCP enabled an empty external URL fails config validation and the pods crash-loop (the MCP pod, and the API when `api.config.mcpEnabled` is `"true"`) | `""` |

In this README `<fullname>` is the release name when it contains `alluredeck`, otherwise `<release>-alluredeck`; `fullnameOverride` replaces it. Component resources are named `<fullname>-api`, `<fullname>-ui`, and `<fullname>-mcp`.

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
| `api.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `api.config.devMode` | Enable dev mode | `"false"` |
| `api.config.logLevel` | Log level | `"info"` |
| `api.config.storageType` | File storage backend (`local` or `s3`) | `"local"` |
| `api.config.databaseURL` | PostgreSQL connection string | `""` |
| `api.config.keepHistory` | Enable test history tracking | `"true"` |
| `api.config.keepHistoryLatest` | Number of history entries to keep (0 = unlimited) | `"100"` |
| `api.config.keepHistoryMaxAgeDays` | Delete reports older than N days (0 = disabled); when set, also garbage-collects non-default branches whose newest build exceeds this age | `"0"` |
| `api.config.maxUploadSizeMb` | Max upload size in MB | `"100"` |
| `api.config.maxArchiveFileCount` | Max files allowed in a tar.gz archive upload | `"5000"` |
| `api.config.uploadWriteConcurrency` | Parallel storage writes during tar.gz extraction (Allure + Playwright); tune to MinIO/S3 capacity | `"32"` |
| `api.config.apiResponseLessVerbose` | Return minimal JSON responses (omit verbose fields) | `"false"` |
| `api.config.staticContentProjects` | Root directory for static project content (embedded report overlay file server) | `"/data/projects"` |
| `api.config.reportGenerationTimeout` | Maximum wall-clock time for one report generation job (download results, run the Allure CLI, upload the report). Accepts a Go duration (`"5m"`, `"1h"`) or integer seconds. Raise for very large result sets | `"5m"` |
| `api.config.mcpEnabled` | Sets `ENABLE_MCP_SERVER` on the API pod: registers the proposal routes and shows **Pending Proposals** in the UI. Set it together with `mcp.enabled`; requires `externalUrl` (or `ingress.enabled=true` with `ingress.host`) | `"false"` |
| `api.config.goMemLimit` | Go memory limit (set to ~80% of memory limit) | `"768MiB"` |
| `api.config.swaggerEnabled` | Serve the API reference at `/swagger/` | `"false"` |
| `api.config.makeViewerEndpointsPublic` | Allow unauthenticated read access | `"false"` |
| `api.config.corsAllowedOrigins` | CORS origins (auto-computed from ingress if empty) | `[]` |
| `api.config.checkResultsEverySeconds` | Polling interval for the local-storage result watcher; `"NONE"` disables it. No effect on retention, and the watcher is already off when `storageType=s3` | `"NONE"` |
| `api.config.trustXForwardedFor` | Trust X-Forwarded-For header | `"true"` |
| `api.kind` | Workload kind (`Deployment` or `StatefulSet`) | `Deployment` |
| `api.replicaCount` | Number of API replicas | `1` |
| `api.resources.requests.cpu` | CPU request | `100m` |
| `api.resources.requests.memory` | Memory request | `256Mi` |
| `api.resources.limits.memory` | Memory limit | `1Gi` |

### API and UI workload settings

`api.*` and `ui.*` share these pod-level keys. Unless noted, each exists under both prefixes.

| Key | Description | Default |
| --- | ----------- | ------- |
| `replicaCount`, `revisionHistoryLimit` | Replica count and retained ReplicaSet history | `1`, `3` |
| `resources` | Requests and limits (see the tables above for defaults) | per component |
| `podSecurityContext`, `securityContext` | Non-root, `readOnlyRootFilesystem`, all capabilities dropped | UID/GID `1000` (API), `101` (UI) |
| `strategy` | Deployment update strategy | `{}` |
| `service.type`, `service.port`, `service.annotations` | Service settings | `ClusterIP`, `8080`, `{}` |
| `livenessProbe`, `readinessProbe`, `startupProbe` | Probe definitions. API: `/health` for liveness and startup, `/ready` for readiness. UI: `/` | see `values.yaml` |
| `terminationGracePeriodSeconds` | Shutdown grace period (API only) | `35` |
| `extraArgs` | Extra container arguments (API only) | `[]` |
| `extraEnv`, `extraEnvFrom` | Extra environment variables and sources | `[]` |
| `extraVolumes`, `extraVolumeMounts`, `initContainers`, `sidecars`, `lifecycle` | Pod and container extensions | `[]` / `{}` |
| `nodeSelector`, `tolerations`, `affinity`, `topologySpreadConstraints` | Scheduling | empty |
| `priorityClassName`, `dnsPolicy`, `dnsConfig` | Pod settings | empty |
| `serviceAccount.create`, `serviceAccount.name`, `serviceAccount.annotations` | ServiceAccount (annotations carry the IRSA role ARN) | `false`, `""`, `{}` |
| `podAnnotations`, `podLabels` | Extra pod metadata | `{}` |
| `persistence.projects.*` | Project data volume, used when `storageType=local` (API only; see [Storage](#storage)) | enabled, `10Gi`, `ReadWriteOnce` |

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
| `ui.config.appVersion` | Version string shown in the UI sidebar footer | `""` (appVersion) |
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
| `ingress.annotations` | Ingress annotations (for example `proxy-body-size`, cert-manager issuer) | `{}` |
| `ingress.tls` | TLS configuration | `[]` |
| `ingress.swagger.enabled` | Route `/swagger` to the API | `false` |
| `ingress.extraPaths` | Extra paths inserted before the catch-all UI rule (see [Extra ingress paths](#extra-ingress-paths)) | `[]` |

The paths themselves are not configurable. See [Ingress routing](#ingress-routing).

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
| `mcpSigningKey` | HMAC key (`MCP_SIGNING_KEY`) that signs attachment download URLs and the state of write confirmations. It does not authenticate MCP requests | `mcp.enabled=true` and the Secret is shared with the API (i.e. `api.security.existingSecret` is set and `mcp.security.existingSecret` is not) |

**Tip:** you can put all of these keys in a single `Secret` and reference it from `api.security.existingSecret` and `api.s3.existingSecret` (and `mcp.security.existingSecret` for MCP) — that way one Secret resource holds every credential the chart needs. The OIDC keys (`oidcClientSecret`, `oidcStateCookieSecret`) are read from `api.security.existingSecret`; there is no separate OIDC Secret reference.

**3-role RBAC:** the chart wires three roles — `admin`, `editor`, and `viewer`. With local auth only, the `admin` account (from `adminUser`/`adminPassword`) has role `admin` and `viewerUser`/`viewerPassword` has role `viewer`; there's no local "editor" account. With OIDC enabled, group-based role mapping via `api.oidc.adminGroups` / `api.oidc.editorGroups` / `api.oidc.defaultRole` promotes SSO users into any of the three roles. See the [OIDC SSO](#oidc-sso) section above.

## Ingress routing

The chart creates a single unified Ingress with path-based routing:

| Path | Backend | Condition |
| ---- | ------- | --------- |
| `/` | UI | always |
| `/api` | API | always |
| `/trace` | API (embedded Playwright trace viewer) | always |
| `/swagger` | API | `ingress.swagger.enabled` |
| `/mcp` | MCP | `mcp.enabled` |
| `/attachments` | API (signed attachment downloads handed out by the MCP server) | `mcp.enabled` |

The API serves `/attachments` only when it has `MCP_SIGNING_KEY` set to the same value as the MCP server. The chart injects that variable into the MCP pod only, so share it with the API when you need signed downloads:

```yaml
api:
  extraEnv:
    - name: MCP_SIGNING_KEY
      valueFrom:
        secretKeyRef:
          name: alluredeck-mcp        # the Secret the MCP pod reads (<fullname>-mcp when chart-managed)
          key: mcpSigningKey
```

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

The chart's Ingress has hardcoded `/api` → API, `/trace` → API, and `/` → UI routes (plus `/swagger`, `/mcp`, and `/attachments` when enabled). If you need to expose additional services under the same host (e.g. a Grafana dashboard on `/grafana`), use `ingress.extraPaths` to inject extra rules **before** the catch-all UI rule:

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

## MCP server

The MCP (Model Context Protocol) server is an **optional** component that exposes a structured API for AI-assisted test analysis tooling. It is disabled by default. See [MCP Server Operations](../../docs/operations/mcp-server.md) for tools, client setup, and troubleshooting.

### Enabling the MCP server

```yaml
externalUrl: "https://alluredeck.example.com"   # or enable the Ingress with ingress.host
api:
  config:
    mcpEnabled: "true"    # proposal routes + Pending Proposals page in the API
mcp:
  enabled: true
```

The MCP server (and the API when `api.config.mcpEnabled` is `"true"`) fails config validation and crash-loops without an external URL. Set `externalUrl`, or enable the Ingress with `ingress.host`. When `ingress.enabled=true`, the Ingress adds `/mcp` (MCP service) and `/attachments` (API) — see [Ingress routing](#ingress-routing).

### Image

| Key | Description | Default |
| --- | ----------- | ------- |
| `mcp.image.repository` | MCP container image | `ghcr.io/mkutlak/alluredeck-mcp` |
| `mcp.image.tag` | Image tag | `""` (appVersion) |
| `mcp.image.pullPolicy` | Image pull policy | `Always` |

### MCP Secret

The chart resolves the MCP Secret name using this precedence:

1. **`mcp.security.existingSecret`** is set → MCP reads from that Secret (dedicated MCP secret).
2. **`api.security.existingSecret`** is set (but `mcp.security.existingSecret` is not) → MCP reads from the API's existing Secret. The chart does **not** generate a separate `<fullname>-mcp` Secret. The shared Secret must include `mcpSigningKey` in addition to `jwtSecretKey` and `databaseURL`.
3. **Neither is set** (fully chart-managed) → the chart auto-creates a Secret named `<fullname>-mcp` containing the following keys:

| Key | Description |
| --- | ----------- |
| `mcpSigningKey` | HMAC key that signs attachment download URLs and write-confirmation state (auto-generated if blank) |
| `jwtSecretKey` | JWT HMAC signing key; must equal the API's (taken from `api.security.jwtSecretKey`, random if blank — see the note below) |
| `databaseURL` | PostgreSQL connection string (taken from `api.config.databaseURL`) |

> **Set `api.security.jwtSecretKey` explicitly** (or use `api.security.existingSecret`). When it is blank, the API Secret and the chart-managed MCP Secret each generate their own random key, so tokens issued by the API fail validation on the MCP server.

The auto-generated Secret is annotated with `helm.sh/resource-policy: keep` so it survives `helm uninstall`. The signing key is preserved across upgrades via Helm `lookup`.

Whichever Secret that is, MCP reads `adminUser` and `viewerUser` from the API's Secret (`api.security.existingSecret`, or the chart-managed `<fullname>-api`). API keys created by those env users are stored under those names, and MCP refuses them as inactive unless it knows the names.

**Sharing the API's existing Secret with MCP** (recommended when you already use `api.security.existingSecret`):

Add `mcpSigningKey` to your existing Secret, then enable MCP — no extra configuration needed:

```bash
kubectl patch secret alluredeck-credentials \
  --namespace <namespace> \
  --type merge \
  -p '{"stringData":{"mcpSigningKey":"<64-char-random-string>"}}'
```

```yaml
api:
  security:
    existingSecret: alluredeck-credentials   # MCP will also read from this
mcp:
  enabled: true
  # mcp.security.existingSecret is NOT set — MCP falls back to the API secret above
```

**Giving MCP its own dedicated Secret** (for stricter secret isolation):

```bash
kubectl create secret generic my-mcp-secret \
  --namespace <namespace> \
  --from-literal=mcpSigningKey='<64-char-random-string>' \
  --from-literal=jwtSecretKey='<64-char-random-string>' \
  --from-literal=databaseURL='postgres://user:password@host:5432/alluredeck?sslmode=require'
```

```yaml
mcp:
  security:
    existingSecret: my-mcp-secret
```

### Signing-key rotation

After rotating the `mcpSigningKey` in the Secret, restart the MCP Deployment to pick up the new value:

```bash
kubectl rollout restart deployment/<fullname>-mcp -n <namespace>
```

Rotation invalidates unexpired attachment download URLs and any write confirmation in flight: the retry of a pending `propose_*` call fails with `confirmation state rejected`, and the client must call the tool again to be prompted anew. If you share the key with the API through `api.extraEnv`, restart the API too.

### MCP values reference

| Key | Description | Default |
| --- | ----------- | ------- |
| `mcp.enabled` | Enable the MCP server | `false` |
| `mcp.replicaCount` | Number of MCP replicas | `1` |
| `mcp.port` | Container port | `8081` |
| `mcp.config.enableMcpServer` | Enable MCP server endpoint (`ENABLE_MCP_SERVER`) | `true` |
| `mcp.config.allowedOrigins` | Allowed browser `Origin` values (`MCP_ALLOWED_ORIGINS`, comma-separated). Requests without an `Origin` header are always accepted; empty accepts every browser origin | `""` |
| `mcp.config.rateLimitPerMin` | Rate limit (requests per minute) | `60` |
| `mcp.config.rateLimitBurst` | Rate limit burst size | `10` |
| `mcp.config.poolMaxConns` | Max database connection pool size | `8` |
| `mcp.config.stateless` | MCP 2026-07-28 stateless transport (`MCP_STATELESS`). Set `false` only for a client that needs session-based transport, and then keep `mcp.replicaCount` at `1` | `true` |
| `mcp.config.toolCosts` | Per-tool rate-limit pricing as `tool=cost,tool=cost` (`MCP_TOOL_COSTS`), layered over the built-in defaults. Costs above `rateLimitBurst` are clamped to it | `""` |
| `mcp.probes.liveness`, `mcp.probes.readiness`, `mcp.probes.startup` | `GET /health` probes. Liveness and readiness take `path`, `port`, `initialDelaySeconds`, `periodSeconds`; startup takes `path`, `port`, `periodSeconds`, `failureThreshold` | `/health` on `8081`; liveness 10s delay / 20s period, readiness 5s / 10s, startup 5s period / 12 failures |
| `mcp.resources` | Resource requests/limits | `requests: {cpu: 50m, memory: 64Mi}, limits: {memory: 256Mi}` |
| `mcp.serviceAccount.create` | Create a dedicated ServiceAccount | `true` |
| `mcp.serviceAccount.name` | Override ServiceAccount name | `""` |
| `mcp.serviceAccount.annotations` | ServiceAccount annotations (e.g. IRSA) | `{}` |
| `mcp.serviceAccount.automountServiceAccountToken` | Mount the ServiceAccount token into the pod. The MCP server does not call the Kubernetes API; keep `false` unless a sidecar needs it | `false` |
| `mcp.serviceAccountName` | Deprecated — use `mcp.serviceAccount.name` | `""` |
| `mcp.security.signingKey` | Fixed MCP signing key (auto-generated if empty) | `""` |
| `mcp.security.existingSecret` | Use a pre-created Secret | `""` |
| `mcp.service.type` | Service type | `ClusterIP` |

The MCP pod reads `api.config.storageType` and, for S3, `api.s3.*` and the S3 credentials. With `storageType=s3` it uses the API's bucket and can inline attachment content. With `storageType=local` the chart mounts no project volume into the MCP pod, so it cannot read attachment files and returns signed download URLs instead. With `observability.enabled=true` it also exports traces using the `observability.*` values, and its process starts a metrics listener on `:9464`; the chart creates no port, Service, or ServiceMonitor for it.

## Autoscaling and disruption budgets

Both are **disabled by default**.

### Autoscaling

Set `autoscaling.enabled=true` to create HorizontalPodAutoscalers. The matching Deployment then omits `replicas`, so the HPA owns the replica count.

| Key | Description | Default |
| --- | ----------- | ------- |
| `autoscaling.enabled` | Create HPAs | `false` |
| `autoscaling.ui.minReplicas` / `maxReplicas` | UI replica bounds | `1` / `5` |
| `autoscaling.ui.targetCPUUtilizationPercentage` | Average CPU utilisation target | `80` |
| `autoscaling.api.minReplicas` / `maxReplicas` | API replica bounds | `1` / `5` |
| `autoscaling.api.targetCPUUtilizationPercentage` | Average CPU utilisation target | `80` |

- The UI HPA renders whenever autoscaling is enabled.
- The API HPA renders **only when `api.config.storageType=s3`**. Local storage uses a `ReadWriteOnce` volume that can bind to a single node, so the API never autoscales with it.
- The HPAs scale on CPU only.

### PodDisruptionBudget

| Key | Description | Default |
| --- | ----------- | ------- |
| `podDisruptionBudget.enabled` | Create PDBs | `false` |
| `podDisruptionBudget.ui.minAvailable` | Pods that must stay available during voluntary disruptions | `1` |
| `podDisruptionBudget.api.minAvailable` | Pods that must stay available during voluntary disruptions | `1` |

A PDB is created only when the workload runs more than one replica (`replicaCount > 1`, or an active HPA with `minReplicas > 1`). With the defaults (one replica, autoscaling off) no PDB object is rendered even when `podDisruptionBudget.enabled=true`, because `minAvailable: 1` on a single replica would block every node drain.

## Migration Job

By default each API replica runs database migrations on boot (see [Schema migrations](#schema-migrations)). With `migrationJob.enabled=true` the chart instead runs them in a Helm hook Job before the new pods start.

```yaml
migrationJob:
  enabled: true
```

| Key | Description | Default |
| --- | ----------- | ------- |
| `migrationJob.enabled` | Run a pre-upgrade migration Job | `false` |
| `migrationJob.migrationTimeout` | Deadline for the migration (`MIGRATION_TIMEOUT`): a Go duration such as `"10m"` or `"1h"` (a bare integer other than `"0"` is rejected). `"0"` disables the deadline, useful for very large migrations | `"10m"` |
| `migrationJob.backoffLimit` | Job retries on transient failure. Keep it low | `2` |
| `migrationJob.activeDeadlineSeconds` | Cap on total Job time including retries | `900` |

How it works:

- The Job is a `pre-install,pre-upgrade` hook (weight `-5`). It runs the API image as `alluredeck-api --migrate-only` with `RUN_MIGRATIONS=true`, applies the pending goose and River migrations under the schema-migration advisory lock, and exits without starting the HTTP server.
- The chart sets `run_migrations: false` (and `migration_timeout`) in the API ConfigMap, so API replicas start without migrating. The MCP ConfigMap does not set it: MCP pods still run migrations on boot, serialised by the advisory lock.
- The Job receives the database, JWT, and admin/viewer credentials but not `OIDC_CLIENT_SECRET` or `OIDC_STATE_COOKIE_SECRET`. With `api.oidc.enabled=true` it fails config validation unless you pass those two variables to it, for example through `api.extraEnvFrom` (the Job reads `api.extraEnv` and `api.extraEnvFrom`) with a Secret whose keys are named `OIDC_CLIENT_SECRET` and `OIDC_STATE_COOKIE_SECRET`.
- The Job is deleted after it succeeds and before the next one is created.
- Helm runs `pre-install` and `pre-upgrade` hooks before it creates or updates the release's other resources. The Job reads the API ConfigMap and Secret, which do not exist on a first install: leave `migrationJob.enabled=false` for the first install and enable it for later upgrades. On an upgrade the Job sees the previous release's ConfigMap and Secret.

**Rollback runbook** — read before enabling on a live cluster:

1. Take a database backup before every `helm upgrade`. Some migrations cannot be reversed (`0031_surrogate_pk.sql` deliberately fails on `down`), so a schema rollback means restoring the backup, not running `goose down`.
2. To roll back after a destructive migration: restore the database from the pre-upgrade backup, pin `api.image.tag` to the previous version, and run `helm upgrade`. The Job then applies the older migration set.
3. Keep at least one full backup per release.

## Observability

OpenTelemetry traces and a Prometheus metrics endpoint are **disabled by default**. When `observability.enabled=false` the chart injects no `OBSERVABILITY_*` / `OTEL_*` variables and creates no metrics Service or ServiceMonitor. For what is instrumented, metric names, and dashboards see [docs/observability.md](../../docs/observability.md).

| Key | Description | Default |
| --- | ----------- | ------- |
| `observability.enabled` | Master switch | `false` |
| `observability.serviceName` | `service.name` resource attribute on spans and metrics | `"alluredeck-api"` |
| `observability.environment` | `deployment.environment.name` attribute (for example `production`); omitted when empty | `""` |
| `observability.traces.endpoint` | OTLP collector `host:port` with no scheme (for example `otel-collector.observability.svc:4318`) | `""` |
| `observability.traces.protocol` | `http/protobuf` or `grpc` | `"http/protobuf"` |
| `observability.traces.sampleRatio` | Fraction of traces sampled, `1.0` = all. Do not use `0`: Helm's `default` renders it as `1` | `1.0` |
| `observability.traces.insecure` | Disable TLS to the collector (in-cluster or local collectors) | `false` |
| `observability.metrics.enabled` | Serve Prometheus metrics from the API on a dedicated port and create a `<fullname>-api-metrics` ClusterIP Service for scraping | `true` |
| `observability.metrics.port` | Metrics container and Service port | `9464` |
| `observability.metrics.path` | Scrape path | `"/metrics"` |
| `observability.serviceMonitor.enabled` | Create a Prometheus Operator `ServiceMonitor` (requires `metrics.enabled` and the ServiceMonitor CRD) | `false` |
| `observability.serviceMonitor.interval` / `scrapeTimeout` | Scrape interval and timeout | `"30s"` / `"10s"` |
| `observability.serviceMonitor.labels` | Extra labels, to match your Prometheus `serviceMonitorSelector` | `{}` |
| `observability.serviceMonitor.relabelings` / `metricRelabelings` | Relabel configs applied before and after scraping | `[]` |

Known limitation: `traces.endpoint` must not include a scheme, and with `http/protobuf` spans can be posted to `/` instead of `/v1/traces` because the endpoint reaches the exporter through an environment variable. See [Observability](../../docs/observability.md#observabilitytraces).

The chart exposes metrics for the API pod only: it declares the port and creates the Service and ServiceMonitor for the API. The MCP pod exports traces with the same `observability.*` trace settings; its own metrics listener (`:9464`) is not exposed.

```yaml
observability:
  enabled: true
  environment: production
  traces:
    endpoint: "otel-collector.observability.svc:4318"
  serviceMonitor:
    enabled: true
    labels:
      prometheus: kube-prometheus
```

## Verifying image provenance

AllureDeck images are built with SLSA provenance, SBOM attestations, and Sigstore signatures. You can verify any image before deploying:

```bash
# MCP server image
gh attestation verify oci://ghcr.io/mkutlak/alluredeck-mcp:<tag> --owner mkutlak

# API image
gh attestation verify oci://ghcr.io/mkutlak/alluredeck-api:<tag> --owner mkutlak

# UI image
gh attestation verify oci://ghcr.io/mkutlak/alluredeck-ui:<tag> --owner mkutlak
```

These commands validate that each image was built by a GitHub Actions workflow in the `mkutlak` organisation and has not been tampered with since publication. Replace `<tag>` with the specific image tag or digest you are deploying (e.g. the value of `Chart.appVersion`).

## Network policy

When `networkPolicy.enabled=true`, the chart creates NetworkPolicy resources that restrict ingress traffic:

- **API pods** — accept traffic from UI pods and (when `ingress.enabled=true`) the ingress controller
- **UI pods** — accept traffic on the service port
- **MCP pods** (when `mcp.enabled=true`) — accept traffic from the ingress controller when `ingress.enabled=true`

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
mise run helm:lint

# Render templates locally (validate without installing)
mise run helm:template

# Package chart into a .tgz archive
mise run helm:package

# Bump chart version and commit
mise run helm:release patch    # or: mise run helm:release minor / mise run helm:release major
```
