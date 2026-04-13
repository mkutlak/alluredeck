# Alluredeck

Monorepo for Allure Reports Dashboard — Go API backend + React frontend.

## API Rules
- See @api/CLAUDE.md

## UI Rules
- See @ui/CLAUDE.md

## Helm Chart Rules
- See @charts/CLAUDE.md

## Project Structure
```text
alluredeck/
  api/              # Go HTTP API backend
    cmd/api/        # entry point
    internal/       # config, handlers, logging, middleware, parser, runner, security, store, storage, swagger, testutil, version
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
  charts/
    alluredeck/     # Helm chart (Chart.yaml, values.yaml, templates/, tests/)
  Makefile          # unified build orchestration
```

## Tech Stack

### API (Go backend)
- **Language**: Go 1.25 (module: `github.com/mkutlak/alluredeck/api`)
- **HTTP**: `net/http` stdlib only (no third-party router)
- **Auth**: JWT (`golang-jwt/jwt/v5`) + bcrypt passwords; optional OIDC SSO via `coreos/go-oidc/v3`
- **Config**: env vars + optional YAML file (`go.yaml.in/yaml/v3`, `kelseyhightower/envconfig`)
- **DB**: PostgreSQL (pgx/v5 + goose v3 migrations)
- **Job Queue**: River v0.34 (PostgreSQL-backed; `riverpgxv5` driver)
- **Storage**: Local filesystem or S3 (`aws-sdk-go-v2`)
- **Report formats**: Allure 2 and Allure 3 (auto-detected via `report_type`); Playwright HTML reports with embedded trace viewer at `/trace/`
- **Logging**: Uber Zap (`go.uber.org/zap`) — JSON in prod, console in dev
- **Docs**: Swagger via `swaggo/swag` + `swaggo/http-swagger`
- **Lint**: golangci-lint v2

### UI (React frontend)
- **Runtime**: Node.js / npm
- **Framework**: React 19, React Router v7
- **State**: Zustand v5 (global), TanStack Query v5 (server state)
- **UI**: Radix UI primitives + Tailwind CSS 4 + shadcn-style components
- **Charts**: Recharts 3
- **Build**: Vite 8
- **Test**: Vitest 4 + Testing Library (jsdom)
- **Lint/Format**: ESLint 10 (flat config) + Prettier
- **Types**: TypeScript 5 (strict)

## Commands (use `make` from the repo root)
```
# API
make api-build          # compile binary → api/bin/
make api-run            # build + run locally
make api-build-static   # static binary for Docker (CGO_ENABLED=0)
make api-test           # go test ./...
make api-test-race      # go test -race ./...
make api-test-cover     # go test with HTML coverage report → api/bin/coverage.html
make api-fmt            # golangci-lint fmt
make api-vet            # go vet
make api-lint           # golangci-lint run
make api-tidy           # go mod tidy
make api-modernize      # apply Go modernization patterns
make api-check          # fmt + vet + lint + test (quality gate)
make api-swagger        # regenerate Swagger docs
make api-clean          # remove api/bin/

# UI
make ui-install         # npm ci
make ui-dev             # Vite dev server (port 7474)
make ui-build           # tsc + vite build → ui/dist/
make ui-preview         # preview production build
make ui-typecheck       # tsc --noEmit
make ui-lint            # eslint
make ui-format          # prettier --write
make ui-test            # vitest run (CI)
make ui-test-watch      # vitest watch mode
make ui-coverage        # vitest with coverage report
make ui-check           # typecheck + lint + test (quality gate)
make ui-clean           # remove dist, coverage, node_modules

# Combined
make check              # full quality gate (API + UI)
make test               # all tests
make clean              # remove all build artifacts

# Docker
make docker-build-api   # build API Docker image
make docker-build-ui    # build UI Docker image
make docker-build       # build both images
make docker-up          # start full stack (UI + API)
make docker-down        # stop full stack
make docker-logs        # follow full stack logs
make docker-up-dev      # start API-only dev stack
make docker-down-dev    # stop API-only dev stack
make docker-logs-dev    # follow API-only dev logs
make docker-up-s3       # start full stack with S3 (MinIO)
make docker-down-s3     # stop S3 stack
make docker-logs-s3     # follow S3 stack logs
make docker-clean       # remove built Docker images

# Helm
make helm-lint          # lint Helm chart
make helm-template      # render templates (validate rendering)
make helm-package       # package chart into .tgz
make helm-release       # bump chart version (BUMP=patch|minor|major) and commit
```

## Development

- Delegate specialized or tool-heavy work to the most appropriate agent.
- Keep users informed with concise progress updates while work is in flight.
- Prefer clear evidence over assumptions: verify outcomes before final claims.
- Choose the lightest-weight path that preserves quality (direct action, or agent).
- Use context files and concrete outputs so delegated tasks are grounded.
- Consult official documentation before implementing with SDKs, frameworks, or APIs - context7 MCP.

## Testing

- Write tests first (TDD) — make them fail, then implement
- Never update existing tests without explicit permission
- API: use `testing` stdlib; no third-party test frameworks
- UI: Mock API calls via `vi.mock('../api/...')` or MSW handlers
