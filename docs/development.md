# Local Development Guide

This guide covers setting up AllureDeck for local development, running services, and testing changes.

## Prerequisites

- **mise** — install from <https://mise.jdx.dev>, then run `mise install` at the repo root to provision the pinned toolchain (Go, Node.js, golangci-lint, helm, yq, swag)

The following tools are provisioned automatically by mise (no manual install needed):
- **Go 1.25.7**
- **Node.js / npm** (version pinned in `mise.toml`)
- **golangci-lint v2**

## Project Structure

```
alluredeck/
  api/                      # Go HTTP API backend
    cmd/api/                # entry point (main.go)
    internal/               # application packages
      config/               # configuration loading
      handlers/             # HTTP request handlers
      logging/              # Zap logger setup
      middleware/           # auth, CORS, CSRF, logging, rate limiting
      parser/               # Allure result parser
      runner/               # report generation orchestration (River job queue)
      security/             # JWT generation/validation, bcrypt
      store/                # store interfaces + PostgreSQL (pg/) impl
      storage/              # local filesystem & S3 (aws-sdk-go-v2)
      swagger/              # generated Swagger/OpenAPI docs
      testutil/             # shared test helpers / mocks
      version/              # build metadata (version, date, ref)
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
  mise.toml                 # unified build orchestration
  docs/                     # documentation
```

## Running Locally

### API (Go backend)

Build and start the API server on `http://localhost:8080`:

```bash
mise run api:run
```

**With custom configuration:**

Point `CONFIG_FILE` to a local YAML configuration file:

```bash
CONFIG_FILE=api/config.example.yaml mise run api:run
```

See [configuration.md](configuration.md) for environment variable reference.

### UI (React frontend)

Install dependencies (first time only):

```bash
mise run ui:install
```

Start the Vite dev server on `http://localhost:7474`:

```bash
mise run ui:dev
```

**With custom API URL:**

By default, the UI connects to the API at `http://localhost:8080/api/v1`. Override this:

```bash
VITE_API_URL=http://localhost:8080/api/v1 mise run ui:dev
```

### Full Stack (Docker Compose)

Start API and UI together:

```bash
mise run docker:up
```

Stop the full stack:

```bash
mise run docker:down
```

View logs:

```bash
mise run docker:logs
```

**Development stack (API only):**

```bash
mise run docker:up-dev    # start API container
mise run docker:down-dev  # stop
mise run docker:logs-dev  # follow logs
```

**With S3 storage (MinIO):**

```bash
mise run docker:up-s3     # includes MinIO for local S3 testing
mise run docker:down-s3
mise run docker:logs-s3
```

## mise Tasks

Run `mise tasks` for the full list. Key tasks:

### API Tasks

| Task | Description |
|------|-------------|
| `mise run api:build` | Compile binary to `api/bin/alluredeck-api` |
| `mise run api:run` | Build and run locally |
| `mise run api:test` | Run all tests (`go test ./...`) |
| `mise run api:test-race` | Run tests with race detector enabled |
| `mise run api:test-cover` | Run tests with coverage report (HTML output to `api/bin/coverage.html`) |
| `mise run api:check` | Full quality gate: fmt + vet + lint + test |
| `mise run api:lint` | Run golangci-lint |
| `mise run api:fmt` | Format code with golangci-lint fmt |
| `mise run api:vet` | Run go vet |
| `mise run api:tidy` | Tidy module dependencies (`go mod tidy`) |
| `mise run api:modernize` | Apply Go modernization patterns |
| `mise run api:swagger` | Regenerate Swagger/OpenAPI docs |
| `mise run api:clean` | Remove build artifacts |

### UI Tasks

| Task | Description |
|------|-------------|
| `mise run ui:install` | Install npm dependencies (`npm ci`) |
| `mise run ui:dev` | Start Vite dev server |
| `mise run ui:build` | Type-check + build to `ui/dist/` |
| `mise run ui:preview` | Preview production build locally |
| `mise run ui:typecheck` | Run TypeScript type checking |
| `mise run ui:test` | Run tests once (Vitest, CI mode) |
| `mise run ui:test-watch` | Run tests in watch mode |
| `mise run ui:coverage` | Generate test coverage report |
| `mise run ui:check` | Full quality gate: typecheck + lint + test |
| `mise run ui:lint` | Run ESLint |
| `mise run ui:format` | Format source files with Prettier |
| `mise run ui:clean` | Remove build artifacts and node_modules |

### Combined Tasks

| Task | Description |
|------|-------------|
| `mise run check` | Full quality gate (API + UI) |
| `mise run test` | All tests (API + UI) |
| `mise run clean` | Remove all build artifacts |

### Docker Tasks

| Task | Description |
|------|-------------|
| `mise run docker:build` | Build both Docker images |
| `mise run docker:build-api` | Build API image only |
| `mise run docker:build-ui` | Build UI image only |
| `mise run docker:up` | Start full stack (UI + API) |
| `mise run docker:down` | Stop full stack |
| `mise run docker:logs` | Follow full stack logs |
| `mise run docker:up-dev` | Start API-only dev stack |
| `mise run docker:down-dev` | Stop API-only dev |
| `mise run docker:logs-dev` | Follow API logs |
| `mise run docker:up-s3` | Start full stack with MinIO |
| `mise run docker:down-s3` | Stop S3 stack |
| `mise run docker:logs-s3` | Follow S3 stack logs |
| `mise run docker:clean` | Remove all built images |

### Helm Tasks

| Task | Description |
|------|-------------|
| `mise run helm:lint` | Lint the Helm chart |
| `mise run helm:template` | Render templates (validates output) |
| `mise run helm:package` | Package chart as `.tgz` archive |
| `mise run helm:release` | Bump version and commit (`mise run helm:release patch\|minor\|major`) |

## Testing

### API Tests

Tests live alongside source files: `foo_test.go` next to `foo.go`. Uses only Go's stdlib `testing` package.

**Run all tests:**

```bash
mise run api:test
```

**Run with race detector:**

```bash
mise run api:test-race
```

**Generate coverage report:**

```bash
mise run api:test-cover
# Opens api/bin/coverage.html in browser
```

### UI Tests

Uses Vitest + Testing Library. Test files live alongside source or in `ui/src/test/`.

**Run once (CI mode):**

```bash
mise run ui:test
```

**Watch mode:**

```bash
mise run ui:test-watch
```

**Coverage report:**

```bash
mise run ui:coverage
```

**Mock API calls:**

Use `vi.mock('../api/...')` for module mocking or MSW handlers for HTTP mocking. Always use Testing Library queries (`getByRole`, `getByText`, `getByLabelText`) — avoid `container.querySelector`.

## Tech Stack

### API (Go backend)

- **Language**: Go 1.25 (module: `github.com/mkutlak/alluredeck/api`)
- **HTTP**: `net/http` stdlib (no third-party router)
- **Auth**: `golang-jwt/jwt/v5` + bcrypt passwords; optional OIDC SSO via `coreos/go-oidc/v3` + `golang.org/x/oauth2`
- **Config**: environment variables + optional YAML (`go.yaml.in/yaml/v3`, `kelseyhightower/envconfig`)
- **Database**: PostgreSQL via `jackc/pgx/v5` + goose v3 migrations
- **Job queue**: River v0.34 (PostgreSQL-backed; `riverdriver/riverpgxv5` driver)
- **Storage**: local filesystem or S3 (`aws-sdk-go-v2`)
- **Report formats**: Allure 2 and Allure 3 (auto-detected); Playwright HTML reports with embedded trace viewer served at `/trace/`
- **Logging**: Uber Zap (`go.uber.org/zap`) — JSON in production, console in development
- **Documentation**: Swagger/OpenAPI via `swaggo/swag`
- **Linting**: golangci-lint v2

### UI (React frontend)

- **Framework**: React 19 + React Router v7
- **State Management**: Zustand v5 (global state), TanStack Query v5 (server state)
- **UI Components**: Radix UI primitives + Tailwind CSS 4 + shadcn-style components
- **Charts**: Recharts 3
- **Timeline**: D3 (`d3-selection`, `d3-scale`, `d3-axis`, `d3-brush`, `d3-zoom`) for the interactive Gantt chart
- **Command palette**: `cmdk` (Cmd+K / Ctrl+K global search)
- **Theme**: `next-themes` with Catppuccin Latte / Mocha palettes
- **Icons**: `lucide-react`
- **Markdown**: `marked` + `dompurify` for safe HTML rendering; `shiki` for syntax highlighting
- **Build Tool**: Vite 8
- **Testing**: Vitest 4 + Testing Library (jsdom); `allure-vitest` reporter for Allure integration
- **Type Checking**: TypeScript 6 (strict mode)
- **Linting**: ESLint 10 (flat config)
- **Formatting**: Prettier with `prettier-plugin-tailwindcss`

**Playwright trace viewer:** The Go binary embeds the Playwright trace viewer static assets at build time via `scripts/fetch-trace-viewer.sh`, which installs `playwright-core` via npm and copies `lib/vite/traceViewer` into `api/static/trace/`. The assets are served from `/trace/` at runtime — no separate runtime install required.

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
mise run api:run

# Terminal 2: UI
mise run ui:dev
```

2. Write tests first (TDD approach). For API, write `*_test.go` files. For UI, write tests in `*.test.ts` or `*.test.tsx`.

3. Make tests fail, then implement the feature.

4. Before committing, run the quality gates:

```bash
mise run check    # runs fmt + vet + lint + test for both API and UI
```

### Fixing a Bug

1. Write a failing test that reproduces the bug.
2. Implement the fix to make the test pass.
3. Run `mise run check` to ensure no regressions.

### Building for Production

Build both API and UI:

```bash
mise run docker:build     # builds Docker images
```

Or individually:

```bash
mise run api:build-static    # API binary (matches Docker)
mise run ui:build            # UI dist/
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

- Clear node_modules and reinstall: `mise run ui:clean && mise run ui:install`
- Verify Node.js version: `node --version`
- Check if port 7474 is in use: `lsof -i :7474`

### Tests fail locally but pass in CI

- Ensure you're using the same Go and Node.js versions as CI
- Clear build caches: `mise run clean`
- Run `mise run check` to catch all issues

### Docker containers won't build

- Ensure Docker daemon is running: `docker ps`
- Check Docker disk space: `docker system df`
- Clean up unused images: `docker system prune -a`

## Related Documentation

- [configuration.md](configuration.md) — environment variables and configuration reference
- [deployment.md](deployment.md) — Docker, Helm, and security guide
