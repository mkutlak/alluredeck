# Local Development Guide

This guide covers setting up AllureDeck for local development, running services, and testing changes.

## Prerequisites

- **Go 1.25.7** or later
- **Node.js / npm** (check `ui/package.json` for exact version constraints)
- **golangci-lint v2** (for API linting)
- **make** (for build orchestration)

## Project Structure

```
alluredeck/
  api/                      # Go HTTP API backend
    cmd/api/                # entry point (main.go)
    internal/               # application packages
      config/               # configuration loading
      handlers/             # HTTP request handlers
      logging/              # Zap logger setup
      middleware/           # auth, CSRF, CORS, rate limiting
      runner/               # Allure CLI execution
      security/             # JWT management
      store/                # SQLite metadata store
      storage/              # local & S3 storage backends
      version/              # version info
    static/                 # embedded static assets + Swagger UI
    go.mod
  ui/                       # React + TypeScript frontend
    src/
      api/                  # axios clients & typed API functions
      components/           # shared UI components (shadcn-style)
      features/             # feature-scoped components and logic
      hooks/                # custom React hooks
      lib/                  # utilities (cn, formatters, etc.)
      routes/               # React Router route definitions
      store/                # Zustand stores
      test/                 # shared test helpers / mocks
      types/                # shared TypeScript types
    package.json
  docker/                   # Dockerfiles and compose configs
    Dockerfile.api
    Dockerfile.ui
    docker-compose.yml
    docker-compose-dev.yml
    docker-compose-s3.yml
    nginx.conf
    docker-entrypoint.sh
  charts/alluredeck/        # Helm chart
  Makefile                  # unified build orchestration
  docs/                     # documentation
```

## Running Locally

### API (Go backend)

Build and start the API server on `http://localhost:8080`:

```bash
make api-run
```

**With custom configuration:**

Point `CONFIG_FILE` to a local YAML configuration file:

```bash
CONFIG_FILE=api/config.example.yaml make api-run
```

See [configuration.md](configuration.md) for environment variable reference.

### UI (React frontend)

Install dependencies (first time only):

```bash
make ui-install
```

Start the Vite dev server on `http://localhost:5173`:

```bash
make ui-dev
```

**With custom API URL:**

By default, the UI connects to the API at `http://localhost:8080/api/v1`. Override this:

```bash
VITE_API_URL=http://localhost:8080/api/v1 make ui-dev
```

### Full Stack (Docker Compose)

Start API and UI together:

```bash
make docker-up
```

Stop the full stack:

```bash
make docker-down
```

View logs:

```bash
make docker-logs
```

**Development stack (API only):**

```bash
make docker-up-dev    # start API container
make docker-down-dev  # stop
make docker-logs-dev  # follow logs
```

**With S3 storage (MinIO):**

```bash
make docker-up-s3     # includes MinIO for local S3 testing
make docker-down-s3
make docker-logs-s3
```

## Make Targets

Run `make help` for the full list. Key targets:

### API Targets

| Target | Description |
|--------|-------------|
| `make api-build` | Compile binary to `api/bin/alluredeck-api` |
| `make api-run` | Build and run locally |
| `make api-test` | Run all tests (`go test ./...`) |
| `make api-test-race` | Run tests with race detector enabled |
| `make api-test-cover` | Run tests with coverage report (HTML output to `api/bin/coverage.html`) |
| `make api-check` | Full quality gate: fmt + vet + lint + test |
| `make api-lint` | Run golangci-lint |
| `make api-fmt` | Format code with golangci-lint fmt |
| `make api-vet` | Run go vet |
| `make api-tidy` | Tidy module dependencies (`go mod tidy`) |
| `make api-modernize` | Apply Go modernization patterns |
| `make api-swagger` | Regenerate Swagger/OpenAPI docs |
| `make api-clean` | Remove build artifacts |

### UI Targets

| Target | Description |
|--------|-------------|
| `make ui-install` | Install npm dependencies (`npm ci`) |
| `make ui-dev` | Start Vite dev server |
| `make ui-build` | Type-check + build to `ui/dist/` |
| `make ui-preview` | Preview production build locally |
| `make ui-typecheck` | Run TypeScript type checking |
| `make ui-test` | Run tests once (Vitest, CI mode) |
| `make ui-test-watch` | Run tests in watch mode |
| `make ui-coverage` | Generate test coverage report |
| `make ui-check` | Full quality gate: typecheck + lint + test |
| `make ui-lint` | Run ESLint |
| `make ui-format` | Format source files with Prettier |
| `make ui-clean` | Remove build artifacts and node_modules |

### Combined Targets

| Target | Description |
|--------|-------------|
| `make check` | Full quality gate (API + UI) |
| `make test` | All tests (API + UI) |
| `make clean` | Remove all build artifacts |

### Docker Targets

| Target | Description |
|--------|-------------|
| `make docker-build` | Build both Docker images |
| `make docker-build-api` | Build API image only |
| `make docker-build-ui` | Build UI image only |
| `make docker-up` | Start full stack (UI + API) |
| `make docker-down` | Stop full stack |
| `make docker-logs` | Follow full stack logs |
| `make docker-up-dev` | Start API-only dev stack |
| `make docker-down-dev` | Stop API-only dev |
| `make docker-logs-dev` | Follow API logs |
| `make docker-up-s3` | Start full stack with MinIO |
| `make docker-down-s3` | Stop S3 stack |
| `make docker-logs-s3` | Follow S3 stack logs |
| `make docker-clean` | Remove all built images |

### Helm Targets

| Target | Description |
|--------|-------------|
| `make helm-lint` | Lint the Helm chart |
| `make helm-template` | Render templates (validates output) |
| `make helm-package` | Package chart as `.tgz` archive |
| `make helm-release` | Bump version and commit (use `BUMP=patch\|minor\|major`) |

## Testing

### API Tests

Tests live alongside source files: `foo_test.go` next to `foo.go`. Uses only Go's stdlib `testing` package.

**Run all tests:**

```bash
make api-test
```

**Run with race detector:**

```bash
make api-test-race
```

**Generate coverage report:**

```bash
make api-test-cover
# Opens api/bin/coverage.html in browser
```

### UI Tests

Uses Vitest + Testing Library. Test files live alongside source or in `ui/src/test/`.

**Run once (CI mode):**

```bash
make ui-test
```

**Watch mode:**

```bash
make ui-test-watch
```

**Coverage report:**

```bash
make ui-coverage
```

**Mock API calls:**

Use `vi.mock('../api/...')` for module mocking or MSW handlers for HTTP mocking. Always use Testing Library queries (`getByRole`, `getByText`, `getByLabelText`) — avoid `container.querySelector`.

## Tech Stack

### API (Go backend)

- **Language**: Go 1.25.7 (module: `github.com/mkutlak/alluredeck/api`)
- **HTTP**: `net/http` stdlib (no third-party router)
- **Auth**: `golang-jwt/jwt/v5` + bcrypt passwords
- **Config**: environment variables + optional YAML (`go.yaml.in/yaml/v3`)
- **Database**: SQLite via `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`)
- **Logging**: Uber Zap (`go.uber.org/zap`) — JSON in production, console in development
- **Documentation**: Swagger/OpenAPI via `swaggo/swag`
- **Linting**: golangci-lint v2

### UI (React frontend)

- **Framework**: React 19 + React Router v7
- **State Management**: Zustand (global state), TanStack Query v5 (server state)
- **UI Components**: Radix UI primitives + Tailwind CSS 4 + shadcn-style components
- **Charts**: Recharts
- **Build Tool**: Vite 6
- **Testing**: Vitest + Testing Library (jsdom)
- **Type Checking**: TypeScript 5 (strict mode)
- **Linting**: ESLint 10 (flat config)
- **Formatting**: Prettier with `prettier-plugin-tailwindcss`

## Code Conventions

### API

- **Dependencies**: No third-party dependencies without explicit approval. Prefer stdlib.
- **Builds**: Always `CGO_ENABLED=0` for production builds (pure Go, no C dependencies).
- **Errors**: All errors are returned; never panic. Structured log messages to stderr.
- **Testing**: Test files live alongside source (`foo_test.go` next to `foo.go`).
- **Configuration**: Via environment variables; `CONFIG_FILE` env var can point to a YAML override.

### UI

- **Path Aliases**: `@/` maps to `ui/src/`
- **Exports**: Named exports for all components (no default exports for components).
- **Tailwind**: Classes are sorted by `prettier-plugin-tailwindcss` on format.
- **Types**: No `any` — use `unknown` with type guards instead.
- **API Configuration**: Base URL configured via `VITE_API_URL` env var (defaults to `http://localhost:8080/api/v1`).
- **Testing**: Test files alongside source or in `ui/src/test/`. Use Testing Library queries; avoid `container.querySelector`.

## Common Workflows

### Developing a Feature

1. Start the API and UI dev servers in separate terminals:

```bash
# Terminal 1: API
make api-run

# Terminal 2: UI
make ui-dev
```

2. Write tests first (TDD approach). For API, write `*_test.go` files. For UI, write tests in `*.test.ts` or `*.test.tsx`.

3. Make tests fail, then implement the feature.

4. Before committing, run the quality gates:

```bash
make check    # runs fmt + vet + lint + test for both API and UI
```

### Fixing a Bug

1. Write a failing test that reproduces the bug.
2. Implement the fix to make the test pass.
3. Run `make check` to ensure no regressions.

### Building for Production

Build both API and UI:

```bash
make docker-build     # builds Docker images
```

Or individually:

```bash
make api-build-static    # API binary (matches Docker)
make ui-build            # UI dist/
```

### Updating Dependencies

**API dependencies:**

```bash
cd api
go get -u [package]
go mod tidy
```

**UI dependencies:**

```bash
cd ui
npm update [package]
npm ci    # reinstall to update package-lock.json
```

Always ask before downgrading versions or introducing new dependencies.

## Troubleshooting

### API won't start

- Check that port 8080 is not in use: `lsof -i :8080`
- Verify `go.mod` is not corrupted: `cd api && go mod verify`
- Check environment variables: `printenv | grep CONFIG`

### UI dev server won't start

- Clear node_modules and reinstall: `make ui-clean && make ui-install`
- Verify Node.js version: `node --version`
- Check if port 5173 is in use: `lsof -i :5173`

### Tests fail locally but pass in CI

- Ensure you're using the same Go and Node.js versions as CI
- Clear build caches: `make clean`
- Run `make check` to catch all issues

### Docker containers won't build

- Ensure Docker daemon is running: `docker ps`
- Check Docker disk space: `docker system df`
- Clean up unused images: `docker system prune -a`

## Related Documentation

- [configuration.md](configuration.md) — environment variables and configuration reference
- [deployment.md](deployment.md) — Docker and Helm deployment guide
- [security.md](security.md) — security considerations and best practices
