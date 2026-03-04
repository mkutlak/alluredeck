# Configuration

AllureDeck reads configuration from three sources with the following precedence (highest to lowest):

1. **Environment variables** — `PORT`, `SECURITY_ENABLED`, `JWT_SECRET_KEY`, etc.
2. **YAML configuration file** — specified by `CONFIG_FILE` env var (default: `/app/alluredeck/config.yaml`)
3. **Built-in defaults** — hardcoded fallback values

This means environment variables always override the YAML file, and the YAML file overrides defaults.

## Configuration File

### Location and Loading

The API server looks for a YAML configuration file at the path specified by the `CONFIG_FILE` environment variable (default: `/app/alluredeck/config.yaml`).

- **Missing file** — silently ignored; the API starts with environment variables and built-in defaults
- **Malformed file** — causes a startup error with a descriptive message
- **Copy to start** — use `api/config.example.yaml` in the repository as a template

### Sensitive Values

Passwords, API keys, and secrets should **always** be set via environment variables, never in the YAML file. This ensures they don't end up in version control or container logs.

```bash
# Good: sensitive values via env vars
export ADMIN_USER="admin"
export ADMIN_PASS="strong-password"
export JWT_SECRET_KEY="<generated-via-openssl>"

# Bad: hardcoding in YAML (do not do this)
# admin_user: "admin"
# admin_pass: "strong-password"
```

## Server Configuration

| Environment Variable | YAML Key | Default | Description |
|----------------------|----------|---------|-------------|
| `PORT` | `port` | `8080` | TCP port the API server listens on |
| `DEV_MODE` | `dev_mode` | `false` | Enable development mode: console logging (not JSON), relaxed request validation, verbose output |
| `TLS` | `tls` | `false` | Enable TLS/HTTPS. Requires `TLS_CERT_FILE` and `TLS_KEY_FILE` environment variables |
| `LOG_LEVEL` | `log_level` | `info` | Logging level: `debug`, `info`, `warn`, `error` |
| `CONFIG_FILE` | *(n/a)* | `/app/alluredeck/config.yaml` | Path to the YAML configuration file (environment variable only) |
| `GOMEMLIMIT` | *(n/a)* | *(not set)* | Go runtime memory limit (e.g., `1GiB`). Set to ~80% of your container memory limit to prevent OOM kills |

### Example

```bash
# Run API on port 3000 in development mode
export PORT="3000"
export DEV_MODE="true"
./api/bin/api
```

## Security Configuration

### Authentication & Authorization

| Environment Variable | YAML Key | Default | Description |
|----------------------|----------|---------|-------------|
| `SECURITY_ENABLED` | `security_enabled` | `false` | Enable HTTP Basic and JWT authentication, and role-based access control |
| `ADMIN_USER` | `admin_user` | *(empty)* | Admin username (used when `SECURITY_ENABLED=true`) |
| `ADMIN_PASS` | `admin_pass` | *(empty)* | Admin password (used when `SECURITY_ENABLED=true`) |
| `VIEWER_USER` | `viewer_user` | *(empty)* | Read-only viewer username |
| `VIEWER_PASS` | `viewer_pass` | *(empty)* | Read-only viewer password |
| `JWT_SECRET_KEY` | `jwt_secret_key` | `super-secret-key-for-dev` | HMAC-SHA256 signing key for JWT tokens. **Must be changed in production.** |
| `JWT_ACCESS_TOKEN_EXPIRES` | `jwt_access_token_expires` | `900` | Access token lifetime in seconds (default: 15 minutes) |
| `JWT_REFRESH_TOKEN_EXPIRES` | `jwt_refresh_token_expires` | `2592000` | Refresh token lifetime in seconds (default: 30 days) |

### Advanced Security

| Environment Variable | YAML Key | Default | Description |
|----------------------|----------|---------|-------------|
| `MAKE_VIEWER_ENDPOINTS_PUBLIC` | `make_viewer_endpoints_public` | `false` | When `true`, viewer-role endpoints require no authentication. Useful for public dashboards |
| `TRUST_FORWARDED_FOR` | `trust_forwarded_for` | `false` | Trust `X-Forwarded-For` header for client IP. **Set to `true` when running behind a reverse proxy** (nginx, ALB, etc.) to ensure correct rate limiting |

### Production Security Requirements

When `SECURITY_ENABLED=true`, the API enforces the following:

- **JWT Secret** — if `JWT_SECRET_KEY` is still the default value (`super-secret-key-for-dev`), the API refuses to start
- **Admin Credentials** — must be set (both `ADMIN_USER` and `ADMIN_PASS`)

Generate a strong JWT secret:

```bash
openssl rand -hex 32
# Example output: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6
```

For details on roles, token types, CSRF protection, and the production security checklist, see [security.md](security.md).

## Storage Configuration (Local Filesystem)

| Environment Variable | YAML Key | Default | Description |
|----------------------|----------|---------|-------------|
| `STORAGE_TYPE` | `storage_type` | `local` | Storage backend: `local` (filesystem) or `s3` (S3/MinIO) |
| `PROJECTS_PATH` | `projects_path` | `/data/projects` | Directory where Allure project results and reports are stored. Must be readable and writable |
| `DATABASE_PATH` | `database_path` | `/data/db/alluredeck.db` | Path to the SQLite metadata database file |
| `KEEP_HISTORY` | `keep_history` | `true` | Retain report history between builds. When `false`, only the latest report is kept |
| `KEEP_HISTORY_LATEST` | `keep_history_latest` | `20` | Maximum number of historical reports to keep per project (when `keep_history=true`) |

### Example

```bash
# Store projects in /data/allure-projects
export PROJECTS_PATH="/data/allure-projects"
export DATABASE_PATH="/data/allure.db"
export KEEP_HISTORY="true"
export KEEP_HISTORY_LATEST="50"
```

## Storage Configuration (S3 / MinIO)

Set `STORAGE_TYPE=s3` to use S3 or MinIO for object storage.

### Required Settings

| Environment Variable | YAML Key | Default | Description |
|----------------------|----------|---------|-------------|
| `S3_ENDPOINT` | `s3.endpoint` | *(empty)* | S3 or MinIO endpoint URL. **Required when `STORAGE_TYPE=s3`**. Examples: `https://s3.amazonaws.com`, `http://minio:9000` |
| `S3_BUCKET` | `s3.bucket` | *(empty)* | S3 bucket name. **Required when `STORAGE_TYPE=s3`** |

### Optional S3 Settings

| Environment Variable | YAML Key | Default | Description |
|----------------------|----------|---------|-------------|
| `S3_REGION` | `s3.region` | `us-east-1` | AWS region (e.g., `eu-west-1`, `us-west-2`) |
| `S3_ACCESS_KEY` | `s3.access_key` | *(empty)* | S3 access key ID. Leave empty if using IAM roles (IRSA on EKS, EC2 instance roles, etc.) |
| `S3_SECRET_KEY` | `s3.secret_key` | *(empty)* | S3 secret access key. Leave empty if using IAM roles. **Set via environment variable in production.** |
| `S3_USE_SSL` | `s3.use_ssl` | `false` | Enable TLS for S3 connections. Use `false` for MinIO over HTTP, `true` for AWS S3 HTTPS |
| `S3_PATH_STYLE` | `s3.path_style` | `false` | Use path-style S3 URLs. **Required for MinIO** (e.g., `http://minio:9000/bucket/key`). AWS S3 uses virtual-hosted-style by default |
| `S3_CONCURRENCY` | `s3.concurrency` | `10` | Maximum number of parallel S3 operations (uploads/downloads). Increase for high-throughput environments; decrease to reduce memory usage |

### Validation

Both `S3_ENDPOINT` and `S3_BUCKET` are validated at startup. If either is missing or unreachable when `STORAGE_TYPE=s3`, the API will exit with an error.

### Example: AWS S3

```bash
export STORAGE_TYPE="s3"
export S3_ENDPOINT="https://s3.amazonaws.com"
export S3_BUCKET="alluredeck-reports"
export S3_REGION="eu-west-1"
export S3_ACCESS_KEY="AKIAIOSFODNN7EXAMPLE"
export S3_SECRET_KEY="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
export S3_USE_SSL="true"
```

### Example: MinIO (Local)

```bash
export STORAGE_TYPE="s3"
export S3_ENDPOINT="http://minio:9000"
export S3_BUCKET="alluredeck"
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"
export S3_USE_SSL="false"
export S3_PATH_STYLE="true"
```

## Report Generation

| Environment Variable | YAML Key | Default | Description |
|----------------------|----------|---------|-------------|
| `CHECK_RESULTS_EVERY_SECONDS` | `check_results_every_secs` | `NONE` | Seconds between automatic result scans for new test data. Set to `NONE` to disable automatic scanning. Valid values: positive integers or `NONE` |
| `ALLURE_VERSION_FILE` | `allure_version_path` | `/app/version` | Path to a text file containing the Allure CLI version string (e.g., `2.25.0`). Used for report display |

### Example

```bash
# Scan for new results every 30 seconds
export CHECK_RESULTS_EVERY_SECONDS="30"

# Disable automatic scanning (manual uploads only)
export CHECK_RESULTS_EVERY_SECONDS="NONE"
```

## API Behavior

| Environment Variable | YAML Key | Default | Description |
|----------------------|----------|---------|-------------|
| `API_RESPONSE_LESS_VERBOSE` | `api_response_less_verbose` | `false` | Return minimal JSON responses (omit extra fields, arrays of objects). Useful for reducing payload size in bandwidth-constrained environments |
| `CORS_ALLOWED_ORIGINS` | `cors_allowed_origins` | *(empty list)* | Comma-separated list of allowed CORS origins (via env var) or YAML list. Empty = CORS disabled. Examples: `https://dashboard.example.com,https://ci.example.com` |

### CORS Example

```bash
# Via environment variable (comma-separated)
export CORS_ALLOWED_ORIGINS="https://dashboard.example.com,https://ci.example.com"
```

Or in YAML:

```yaml
cors_allowed_origins:
  - "https://dashboard.example.com"
  - "https://ci.example.com"
```

## UI Environment Variables

These variables configure the React frontend. They are injected at container **runtime** (not build time) via `docker/docker-entrypoint.sh` into the `window.__env__` object. This means a single Docker image works with any API endpoint without rebuilding.

| Environment Variable | Default | Description |
|----------------------|---------|-------------|
| `VITE_API_URL` | `http://localhost:5050` | AllureDeck API base URL. Include the `/api/v1` path. Example: `https://api.example.com/api/v1` |
| `VITE_APP_TITLE` | `AllureDeck` | Browser tab title and top-bar brand text |
| `VITE_APP_VERSION` | `dev` | Version string displayed in the UI (e.g., from CI/CD pipeline) |

### Example (Docker)

```bash
docker run \
  -e VITE_API_URL="https://api.example.com/api/v1" \
  -e VITE_APP_TITLE="My Company - Test Reports" \
  -e VITE_APP_VERSION="1.2.3" \
  alluredeck-ui
```

## Example Configuration Files

### Minimal (Development)

Store locally, security disabled:

```yaml
# AllureDeck API - Example Configuration File
#
# Copy this file to config.yaml and adjust values as needed.
# Precedence (highest wins): environment variables > this file > built-in defaults.
#
# Set CONFIG_FILE=/path/to/config.yaml to load this file.
# Default location: /app/alluredeck/config.yaml
#
# Sensitive fields (passwords, secrets) should be set via environment variables
# rather than stored in this file.

# --- Server ---
port: "8080"
dev_mode: false
tls: false

# --- Security ---
security_enabled: false
admin_user: ""
admin_pass: ""
viewer_user: ""
viewer_pass: ""
jwt_secret_key: ""
jwt_access_token_expires: 900
jwt_refresh_token_expires: 2592000
make_viewer_endpoints_public: false
trust_forwarded_for: false

# --- Logging ---
log_level: "info"

# --- Storage ---
projects_path: "/data/projects"
database_path: "/data/db/alluredeck.db"
keep_history: true
keep_history_latest: 20

# --- Report generation ---
check_results_every_secs: "NONE"
allure_version_path: "/app/version"

# --- UI / API behaviour ---
api_response_less_verbose: false

# --- Storage Backend ---
storage_type: "local"

# --- S3/MinIO Storage (optional) ---
s3:
  endpoint: ""
  bucket: ""
  region: "us-east-1"
  access_key: ""
  secret_key: ""
  use_ssl: false
  path_style: false
  concurrency: 10

# --- CORS ---
cors_allowed_origins: []
```

### Production (S3 + Security)

Secure deployment with S3 storage:

```yaml
# --- Server ---
port: "8080"
dev_mode: false
tls: false

# --- Security ---
security_enabled: true
# Set via env vars (ADMIN_USER, ADMIN_PASS, JWT_SECRET_KEY)
admin_user: ""
admin_pass: ""
viewer_user: ""
viewer_pass: ""
jwt_secret_key: ""
jwt_access_token_expires: 900
jwt_refresh_token_expires: 2592000
make_viewer_endpoints_public: false
trust_forwarded_for: true

# --- Logging ---
log_level: "warn"

# --- Storage ---
projects_path: "/data/projects"
database_path: "/data/db/alluredeck.db"
keep_history: true
keep_history_latest: 100

# --- Report generation ---
check_results_every_secs: "60"
allure_version_path: "/app/version"

# --- UI / API behaviour ---
api_response_less_verbose: true
cors_allowed_origins:
  - "https://alluredeck.example.com"

# --- Storage Backend ---
storage_type: "s3"

# --- S3/MinIO Storage ---
s3:
  endpoint: "https://s3.amazonaws.com"
  bucket: "alluredeck-reports-prod"
  region: "eu-west-1"
  # Access key and secret via env vars (S3_ACCESS_KEY, S3_SECRET_KEY)
  access_key: ""
  secret_key: ""
  use_ssl: true
  path_style: false
  concurrency: 20
```

## Related Documentation

- [security.md](security.md) — authentication, authorization, JWT tokens, CSRF protection, TLS, production security checklist
- [storage.md](storage.md) — S3/MinIO setup guide and troubleshooting
- [helm-chart.md](helm-chart.md) — Kubernetes/Helm configuration and secrets management
