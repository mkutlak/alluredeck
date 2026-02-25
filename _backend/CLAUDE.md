# allure-docker-service

Go HTTP API backend for the Allure Docker Service.

## Tech Stack
- **Language**: Go 1.25 (module: `github.com/mkutlak/allure-docker-service/allure-docker-api`)
- **HTTP**: `net/http` stdlib only (no third-party router)
- **Auth**: JWT (`golang-jwt/jwt/v5`) + bcrypt passwords
- **Config**: env vars + optional YAML file (`go.yaml.in/yaml/v3`)
- **DB**: SQLite via `modernc.org/sqlite` (pure Go, CGO_ENABLED=0)
- **Docs**: Swagger via `swaggo/swag` + `swaggo/http-swagger`
- **Lint**: golangci-lint v2

## Commands (use `make` from `allure-docker-service/`)
```
make build         # compile binary → allure-docker-api/bin/
make build-static  # CGO_ENABLED=0 static binary (matches Docker)
make run           # build + run local binary
make test          # go test ./...
make test-race     # go test -race ./...
make test-cover    # coverage report → bin/coverage.html
make check         # fmt + vet + lint + test (quality gate)
make fmt           # golangci-lint fmt
make lint          # golangci-lint run
make vet           # go vet
make tidy          # go mod tidy
make modernize     # apply Go modernization patterns
make swagger       # regenerate Swagger docs (requires swag)
make docker-build  # build Docker image
make docker-up     # docker compose up -d
make docker-down   # docker compose down
make docker-logs   # follow compose logs
make docker-shell  # exec sh in running container
make docker-clean  # remove built image
make clean         # rm allure-docker-api/bin/
```

## Project Structure
```
allure-docker-api/
  cmd/api/main.go      # entry point, wires all dependencies
  internal/
    config/            # env + YAML config loading
    handlers/          # HTTP handlers (allure, auth, system)
    middleware/        # auth + CORS middleware
    runner/            # allure CLI runner, filesystem ops, watcher
    security/          # JWT manager, password hashing
    store/             # SQLite metadata store
    swagger/           # generated Swagger docs
  static/              # embedded static assets + swagger UI
  templates/           # HTML templates
```

## Testing
- Write tests first (TDD) — make them fail, then implement
- Test files alongside source: `foo_test.go` next to `foo.go`
- Use `testing` stdlib; no third-party test frameworks
- Never update existing tests without explicit permission

## Conventions
- No third-party dependencies without explicit approval
- Prefer stdlib; add libraries only when justified
- `CGO_ENABLED=0` for production builds (pure Go)
- Config via env vars; `CONFIG_FILE` env var points to YAML override
- Filesystem is source of truth for report data; SQLite is metadata cache
- All errors returned, not panicked; structured log messages to stderr
