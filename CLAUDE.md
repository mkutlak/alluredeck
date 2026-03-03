# Alluredeck

Monorepo for Allure Reports Dashboard — Go API backend + React frontend.

## Project Structure
```
alluredeck/
  api/              # Go HTTP API backend
    cmd/api/        # entry point
    internal/       # config, handlers, logging, middleware, runner, security, store, storage, version
    static/         # embedded static assets + swagger UI
    go.mod
  ui/               # React + TypeScript frontend
    src/
      api/          # axios clients & typed API functions
      components/   # shared UI components (shadcn-style)
      features/     # feature-scoped components and logic
      hooks/        # custom React hooks
      lib/          # utilities (cn, formatters, etc.)
      routes/       # React Router route definitions
      store/        # Zustand stores
      test/         # shared test helpers / mocks
      types/        # shared TypeScript types
    package.json
  docker/           # Dockerfiles and compose configs
    Dockerfile.api
    Dockerfile.ui
    docker-compose.yml
    docker-compose-dev.yml
    docker-compose-s3.yml
    nginx.conf
    docker-entrypoint.sh
  Makefile          # unified build orchestration
```

## Tech Stack

### API (Go backend)
- **Language**: Go 1.25 (module: `github.com/mkutlak/alluredeck/api`)
- **HTTP**: `net/http` stdlib only (no third-party router)
- **Auth**: JWT (`golang-jwt/jwt/v5`) + bcrypt passwords
- **Config**: env vars + optional YAML file (`go.yaml.in/yaml/v3`)
- **DB**: SQLite via `modernc.org/sqlite` (pure Go, CGO_ENABLED=0)
- **Logging**: Uber Zap (`go.uber.org/zap`) — JSON in prod, console in dev
- **Docs**: Swagger via `swaggo/swag` + `swaggo/http-swagger`
- **Lint**: golangci-lint v2

### UI (React frontend)
- **Runtime**: Node.js / npm
- **Framework**: React 19, React Router v7
- **State**: Zustand (global), TanStack Query v5 (server state)
- **UI**: Radix UI primitives + Tailwind CSS 4 + shadcn-style components
- **Charts**: Recharts
- **Build**: Vite 6
- **Test**: Vitest + Testing Library (jsdom)
- **Lint/Format**: ESLint 10 (flat config) + Prettier
- **Types**: TypeScript 5 (strict)

## Commands (use `make` from the repo root)
```
# API
make api-build        # compile binary → api/bin/
make api-run          # build + run locally
make api-test         # go test ./...
make api-check        # fmt + vet + lint + test (quality gate)
make api-lint         # golangci-lint run
make api-swagger      # regenerate Swagger docs

# UI
make ui-install       # npm ci
make ui-dev           # Vite dev server
make ui-build         # tsc + vite build → ui/dist/
make ui-test          # vitest run (CI)
make ui-check         # typecheck + lint + test (quality gate)
make ui-lint          # eslint

# Combined
make check            # full quality gate (API + UI)
make test             # all tests

# Docker
make docker-build     # build both images
make docker-up        # start full stack (UI + API)
make docker-down      # stop full stack
make docker-up-dev    # API-only dev stack
make docker-up-s3     # full stack with S3 (MinIO)

# Helm
make helm-lint        # lint Helm chart
make helm-template    # render templates (validate rendering)
make helm-package     # package chart into .tgz
```

## API Conventions
- No third-party dependencies without explicit approval
- Prefer stdlib; add libraries only when justified
- `CGO_ENABLED=0` for production builds (pure Go)
- Config via env vars; `CONFIG_FILE` env var points to YAML override
- Filesystem is source of truth for report data; SQLite is metadata cache
- All errors returned, not panicked; structured log messages to stderr
- Test files alongside source: `foo_test.go` next to `foo.go`

## UI Conventions
- Path alias `@/` maps to `ui/src/`
- Components use named exports (no default exports for components)
- Tailwind classes sorted by `prettier-plugin-tailwindcss`
- No `any` — use `unknown` and type guards instead
- API base URL configured via `VITE_API_URL` env var
- Test files alongside source or in `ui/src/test/`
- Use Testing Library queries (getByRole, etc.) — avoid `container.querySelector`

## Testing
- Write tests first (TDD) — make them fail, then implement
- Never update existing tests without explicit permission
- API: use `testing` stdlib; no third-party test frameworks
- UI: Mock API calls via `vi.mock('../api/...')` or MSW handlers
