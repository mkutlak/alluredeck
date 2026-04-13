# Storage Configuration

AllureDeck supports two storage backends: local filesystem (default) and S3-compatible object storage (AWS S3 or MinIO). Select via the `STORAGE_TYPE` environment variable.

The filesystem stores Allure report data. PostgreSQL is used for metadata and index storage.

## Local Storage (Default)

`STORAGE_TYPE=local` (default)

Allure project data is stored under `PROJECTS_PATH` (default: `/data/projects`), with this structure:

```
/data/projects/
  {project-id}/
    results/      # uploaded Allure result files
    reports/
      latest/     # most recently generated report
      {N}/        # historical report builds (variable dirs only)
```

Only "variable" subdirectories (`data/`, `widgets/`, `history/`) are stored per historical build — static assets are served from `latest/`, saving approximately 90% disk space per build.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `STORAGE_TYPE` | `local` | Storage backend type |
| `PROJECTS_PATH` | `/data/projects` | Project data root directory |
| `DATABASE_URL` | `postgres://alluredeck:alluredeck@localhost:5432/alluredeck?sslmode=disable` | PostgreSQL connection string |
| `KEEP_HISTORY` | `true` | Retain previous report builds |
| `KEEP_HISTORY_LATEST` | `100` | Max historical builds per project |
| `KEEP_HISTORY_MAX_AGE_DAYS` | `0` | Delete reports older than N days (0 = disabled) |
| `PENDING_RESULTS_MAX_AGE_DAYS` | `3` | Delete uploaded-but-never-generated result files older than N days. Cleaned up by the same daily background scheduler that prunes report history |

### Docker Compose

In `docker/docker-compose.yml`, a named Docker volume persists data across container restarts:

```yaml
volumes:
  - allure-projects:/data/projects
```

### Kubernetes / Helm

The Helm chart creates a persistent volume claim by default (when `storageType=local`):

- `projects` — 10Gi for report data (`ReadWriteOnce`)

See the [Helm Chart README](../charts/alluredeck/README.md#storage) for PVC configuration details.

## S3 / MinIO Storage

`STORAGE_TYPE=s3`

Reports are stored in an S3-compatible bucket. The API downloads results to a temporary directory before generating reports, then uploads the result back to S3.

### S3 Key Structure

```
projects/{project-id}/results/         # uploaded result files
projects/{project-id}/reports/latest/  # latest generated report
projects/{project-id}/reports/{N}/     # historical builds (variable dirs only)
```

### Required Settings

Startup fails if these are missing when `STORAGE_TYPE=s3`:

| Variable | Description |
|----------|-------------|
| `S3_ENDPOINT` | S3 or MinIO endpoint URL |
| `S3_BUCKET` | Target bucket name |

### All S3 Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `STORAGE_TYPE` | `local` | Must be set to `s3` |
| `S3_ENDPOINT` | *(required)* | Endpoint URL (e.g. `https://s3.amazonaws.com` or `http://minio:9000`) |
| `S3_BUCKET` | *(required)* | Bucket name |
| `S3_REGION` | `us-east-1` | AWS region |
| `S3_ACCESS_KEY` | *(empty)* | Access key ID (omit for IRSA) |
| `S3_SECRET_KEY` | *(empty)* | Secret access key (omit for IRSA) |
| `S3_TLS_INSECURESKIPVERIFY` | `false` | Skip TLS certificate verification (e.g. self-signed certs) |
| `S3_PATH_STYLE` | `false` | Path-style URLs — **required for MinIO** |
| `S3_CONCURRENCY` | `10` | Max parallel upload/download operations |

### MinIO (Local Development)

Use `docker/docker-compose-s3.yml` to spin up MinIO alongside AllureDeck:

```bash
docker compose -f docker/docker-compose-s3.yml up -d
```

Services:

- **MinIO API** — `http://localhost:9000`
- **MinIO Console** — `http://localhost:9001` (minioadmin / minioadmin)
- **AllureDeck API** — `http://localhost:5050`
- **AllureDeck UI** — `http://localhost:7474` (admin / admin)

The `minio-init` service automatically creates the `allure-reports` bucket.

#### MinIO Configuration

```
STORAGE_TYPE=s3
S3_ENDPOINT=http://minio:9000
S3_BUCKET=allure-reports
S3_REGION=us-east-1
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_PATH_STYLE=true     # required for MinIO
```

### AWS S3

```
STORAGE_TYPE=s3
S3_ENDPOINT=https://s3.amazonaws.com
S3_BUCKET=my-allure-reports
S3_REGION=eu-west-1
S3_ACCESS_KEY=AKIA...
S3_SECRET_KEY=...
S3_PATH_STYLE=false    # virtual-hosted style for AWS
```

### IRSA on EKS (IAM Roles for Service Accounts)

On Amazon EKS, skip static credentials and use IRSA instead — the AWS SDK automatically picks up credentials from the pod's service account.

1. Create an IAM role with S3 permissions for your bucket.
2. Annotate the API service account in your Helm values:

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
    # Do not set S3_ACCESS_KEY or S3_SECRET_KEY
```

3. Do not set `S3_ACCESS_KEY` or `S3_SECRET_KEY` — leave them empty.

### Helm S3 Configuration (Static Credentials)

```yaml
api:
  config:
    storageType: "s3"
  s3:
    endpoint: "http://minio.minio.svc:9000"
    bucket: "allure-reports"
    region: "us-east-1"
    pathStyle: "true"
    concurrency: "10"
    # Reference a pre-created Kubernetes Secret with keys: S3_ACCESS_KEY, S3_SECRET_KEY
    existingSecret: "alluredeck-s3-credentials"
```

## Related

- [configuration.md](configuration.md) — full environment variable reference
- [Helm Chart README](../charts/alluredeck/README.md) — Helm persistence and S3 values
- [deployment.md](deployment.md#security) — authentication and authorization
