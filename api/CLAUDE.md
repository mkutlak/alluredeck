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

## Store interface assertions
- Every type implementing a `store` interface — production or test — must carry a
  `var _ store.X = (*T)(nil)` compile-time assertion directly below its type
  declaration. An interface change then breaks immediately and locally at the
  offending type rather than silently surfacing later in an unrelated package.
- Shared mock stores live in `internal/testutil` (`Mock*` function-field doubles,
  `Mem*` stateful in-memory stores). Reuse these instead of hand-writing mocks.
- A local mock in a `_test.go` file is acceptable only when it has genuine
  bespoke behavior the shared mock cannot provide — argument capture, a
  decorator wrapping another store, or an intentional partial panic-stub. Local
  mocks still carry the `var _` assertion; decorators assert the interface they
  wrap.

## Lint
golangci-lint v2 — config at `api/.golangci.yml`. Run via `mise run api:lint`.

## API Docs
OpenAPI spec generated with `mise run api:swagger` (runs `swag init -g cmd/api/main.go`).
Output written to `internal/swagger/`. Served via Scalar at `/swagger/` when `SWAGGER_ENABLED=true`.
