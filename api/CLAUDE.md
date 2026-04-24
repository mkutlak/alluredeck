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
| `handlers`   | HTTP handlers — see `ls internal/handlers/*.go` for full list |
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

## Lint
golangci-lint v2 — config at `api/.golangci.yml`. Run via `make api-lint`.

## API Docs
OpenAPI spec generated with `make api-swagger` (runs `swag init -g cmd/api/main.go`).
Output written to `internal/swagger/`. Served via Scalar at `/swagger/` when `SWAGGER_ENABLED=true`.
