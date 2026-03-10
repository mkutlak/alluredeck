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
| `handlers`   | HTTP handlers (allure, auth, admin, analytics, known issues, etc.) |
| `logging`    | Zap logger setup (JSON prod / console dev) |
| `middleware` | HTTP middleware (auth, CORS, logging, rate limiting) |
| `parser`     | Allure result parser — reads JSON results, populates enrichment tables |
| `runner`     | Report generation orchestration (JobQueuer, RiverJobManager, allure runner) |
| `security`   | JWT generation/validation, bcrypt password hashing |
| `store`      | Store interfaces + PostgreSQL (`pg/`) implementation |
| `storage`    | File storage abstraction — local filesystem or S3 (aws-sdk-go-v2) |
| `swagger`    | Generated Swagger/OpenAPI docs (swaggo/swag output) |
| `testutil`   | Shared test helpers (mock stores, test HTTP helpers) |
| `version`    | Build metadata (version, date, ref injected at link time) |

## Key Dependencies
- **DB**: `github.com/jackc/pgx/v5` (PostgreSQL driver), `github.com/pressly/goose/v3` (migrations)
- **Job queue**: `github.com/riverqueue/river v0.31` (PostgreSQL-backed; riverpgxv5 driver)
- **Storage**: `github.com/aws/aws-sdk-go-v2` + S3 transfer manager
- **Auth**: `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto` (bcrypt)
- **Config**: `github.com/kelseyhightower/envconfig`, `go.yaml.in/yaml/v3`
- **Logging**: `go.uber.org/zap v1.27`
- **Swagger**: `github.com/swaggo/swag v1.16`, `github.com/swaggo/http-swagger/v2`

## Lint
golangci-lint v2 — config at `api/.golangci.yml`. Run via `make api-lint`.

## Swagger
Docs generated with `make api-swagger` (runs `swag init -g cmd/api/main.go`).
Output written to `internal/swagger/`. Served at `/swagger/index.html` when `SWAGGER_ENABLED=true`.
