## API Conventions
- No third-party dependencies without explicit approval
- Prefer stdlib; add libraries only when justified
- `CGO_ENABLED=0` for production builds (pure Go)
- Config via env vars; `CONFIG_FILE` env var points to YAML override
- PostgreSQL is the primary database (pgx/v5 + goose migrations)
- All errors returned, not panicked; structured log messages to stderr
- Test files alongside source: `foo_test.go` next to `foo.go`

## Internal Packages

| Package      | Description |
|--------------|-------------|
| `config`     | App configuration loaded from env vars and optional YAML |
| `handlers`   | HTTP handlers — enumerated below |
| `logging`    | Zap logger setup (JSON prod / console dev) |
| `middleware` | HTTP middleware (auth, CORS, logging, rate limiting, CSRF) |
| `parser`     | Allure result parser — reads JSON results, populates enrichment tables |
| `runner`     | Report generation orchestration (JobQueuer, RiverJobManager, allure runner, playwright runner) |
| `security`   | JWT generation/validation, bcrypt password hashing, refresh token rotation |
| `store`      | Store interfaces + PostgreSQL (`pg/`) implementation |
| `storage`    | File storage abstraction — local filesystem or S3 (aws-sdk-go-v2) |
| `swagger`    | Generated Swagger/OpenAPI docs (swaggo/swag output) |
| `testutil`   | Shared test helpers (mock stores, test HTTP helpers) |
| `version`    | Build metadata (version, date, ref injected at link time) |

### Handlers

Each handler file lives in `internal/handlers/`. Related helpers (`errors.go`, `pagination.go`, `render.go`, `response.go`, `types.go`, `namespace.go`, `project_id.go`) are shared across handlers.

| File | Surface |
|------|---------|
| `auth.go`, `oidc.go` | Local login, logout, refresh, session, OIDC authorization code + PKCE flow |
| `api_keys.go` | Per-user API key CRUD (SHA-256 hashed, `ald_` prefix) |
| `project_handler.go`, `project_parent.go` | Project CRUD, rename, parent/child relationships, children listing |
| `report_handler.go` | Report history, generation, deletion, per-report widget endpoints |
| `result_upload_handler.go` | Allure result file upload (`POST /results`) |
| `playwright.go` | Playwright HTML report upload and serving |
| `allure_branches.go`, `allure_stability.go`, `allure_summary.go`, `allure_timeline.go`, `allure_test_history.go` | Per-report Allure widgets served from DB enrichment tables |
| `analytics_handler.go`, `low_performing_handler.go` | Trend charts, error breakdowns, suite stats, labels, low-performing tests |
| `known_issues.go` | Known-issue tagging with regex/substring matching |
| `defect_handler.go` | Defect fingerprinting, grouping, bulk classification |
| `webhook_handler.go` | Webhook CRUD, test-send, delivery history |
| `pipeline_handler.go` | Parent-project CI pipeline aggregation by commit SHA |
| `compare_handler.go` | Build-to-build diff (regressed / fixed / added / removed) |
| `attachment.go` | Test result attachment browsing |
| `dashboard_handler.go` | Cross-project health summary cards |
| `project_timeline_handler.go` | Multi-build timeline aggregation |
| `search_handler.go` | Global full-text search (PostgreSQL FTS) |
| `preferences.go` | Per-user UI preferences (pagination, view mode, branch filter) |
| `admin.go`, `system.go` | Admin job queue, pending-results management, health/readiness |

## Key Dependencies
- **DB**: `github.com/jackc/pgx/v5 v5.9.1` (PostgreSQL driver), `github.com/pressly/goose/v3 v3.27.0` (migrations)
- **Job queue**: `github.com/riverqueue/river v0.34` (PostgreSQL-backed; `riverdriver/riverpgxv5` driver)
- **Storage**: `github.com/aws/aws-sdk-go-v2 v1.41` + S3 transfer manager
- **Auth**: `github.com/golang-jwt/jwt/v5 v5.3.1`, `golang.org/x/crypto` (bcrypt); OIDC SSO via `github.com/coreos/go-oidc/v3 v3.18` + `golang.org/x/oauth2`
- **Config**: `github.com/kelseyhightower/envconfig`, `go.yaml.in/yaml/v3`
- **Logging**: `go.uber.org/zap v1.27`
- **Swagger**: `github.com/swaggo/swag v1.16`, `github.com/swaggo/http-swagger/v2`

## Lint
golangci-lint v2 — config at `api/.golangci.yml`. Run via `make api-lint`.

## Swagger
Docs generated with `make api-swagger` (runs `swag init -g cmd/api/main.go`).
Output written to `internal/swagger/`. Served at `/swagger/index.html` when `SWAGGER_ENABLED=true`.
