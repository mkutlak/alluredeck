# Local Development Guide

This guide covers setting up AllureDeck for local development, running services, and testing changes.

## Prerequisites

- **mise** — install from <https://mise.jdx.dev>, then run `mise install` at the repo root to provision the pinned toolchain (Go, Node.js, golangci-lint, helm, yq, swag, zizmor)
- **Docker** — needed for the `docker:*`, `docs:screenshots`, `e2e:test`, `e2e:report`, `e2e:update-snapshots`, and `e2e:clean` tasks, and to run PostgreSQL for local development. `e2e:install`, `e2e:test-local`, `e2e:test-headed`, and `e2e:test-ui` run locally without Docker

The following tools are provisioned automatically by mise (no manual install needed):
- **Go 1.26.5** (kept in sync with `api/go.mod`)
- **Node.js 26** / npm
- **golangci-lint v2**, **Helm 3**, **yq 4**, **swag**, **zizmor 1**

## Project Structure

```
alluredeck/
  api/                      # Go HTTP API backend
    cmd/
      api/                  # API server (main.go)
      mcp/                  # MCP server binary
      gcstorage/            # one-shot cleanup of orphaned report data (see storage.md)
      backfill/             # one-shot re-derivation of defect fingerprints
      mcp-eval/             # evaluation gate for the MCP server (calls the Anthropic API)
    internal/               # application packages
      bootstrap/            # startup wiring shared by the cmd/* binaries
      config/               # configuration loading
      failure/              # failure-evidence assembly for AI summaries
      handlers/             # HTTP request handlers
      llm/                  # LLM client for AI failure summaries
      logging/              # Zap logger setup
      mcp/                  # MCP tools, resources, rate limiting
      middleware/           # auth, CORS, CSRF, logging, rate limiting
      observability/        # OpenTelemetry traces + Prometheus metrics
      parser/               # Allure result parser
      runner/               # report generation orchestration (River job queue)
      security/             # JWT generation/validation, bcrypt, OIDC, encryption
      store/                # store interfaces + PostgreSQL (pg/) impl
      storage/              # local filesystem & S3 (aws-sdk-go-v2)
      swagger/              # generated OpenAPI docs (swag)
      testutil/             # shared test helpers / mocks
      triage/               # triage signals for failed tests
      version/              # build metadata (version, date, ref)
    static/                 # embedded Playwright trace viewer assets (static/trace)
    config.example.yaml     # commented configuration template
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
  e2e/                      # Playwright end-to-end tests (see e2e/README.md)
  docker/                   # Dockerfiles and compose configs
    Dockerfile.api
    Dockerfile.ui
    Dockerfile.mcp
    docker-compose.yml            # full stack: UI, API, MCP, PostgreSQL
    docker-compose-dev.yml        # API + PostgreSQL
    docker-compose-s3.yml         # full stack with MinIO
    docker-compose-preview.yml    # local tree against a remote database
    nginx.conf
    docker-entrypoint.sh
  charts/alluredeck/        # Helm chart
  scripts/                  # trace viewer fetch, Helm release, Allure result upload
  mise.toml                 # unified build orchestration
  docs/                     # documentation
```

## Running Locally

### API (Go backend)

The API needs PostgreSQL (`DATABASE_URL` is required). Start one — this publishes `localhost:5432`, or reuse the database of a running `mise run docker:up` stack:

```bash
docker compose -f docker/docker-compose.yml up -d postgres
```

Build and start the API server on `http://localhost:8080`:

```bash
export DATABASE_URL='postgresql://allure:allure@localhost:5432/allure?sslmode=disable'
export PROJECTS_PATH="$PWD/.data/projects"     # the default, /data/projects, is rarely writable on a dev machine
export CORS_ALLOWED_ORIGINS='http://localhost:7474'   # lets the Vite dev server call the API
export SECURITY_ENABLED=true                          # with it off, every role-checked endpoint returns 403
export ADMIN_USER=admin ADMIN_PASS=admin
export JWT_SECRET_KEY="$(openssl rand -hex 32)"
mkdir -p "$PROJECTS_PATH"
mise run api:run
```

`SECURITY_ENABLED` defaults to `false`, and in that mode requests carry no identity, so every role-checked endpoint returns `403 Access denied: missing claims`; that is why the block above enables it. Log in to the UI as `admin` / `admin`. Check the API with `curl http://localhost:8080/ready`. Generating reports shells out to the `allure` CLI (npm package `allure`, major 3), which the Docker images bundle; install it locally if you need report generation outside Docker.

**With custom configuration:**

Point `CONFIG_FILE` to a local YAML configuration file. Use an absolute path — `api:run` executes from `api/`, and a missing file is silently ignored:

```bash
CONFIG_FILE="$PWD/api/config.example.yaml" mise run api:run
```

Environment variables (such as `DATABASE_URL` above) still override the file.

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

Without `VITE_API_URL`, the UI falls back to `http://localhost:5050` — the API port of the Docker Compose stacks, and without the `/api/v1` path. `mise run api:run` listens on port 8080, so point the UI at it explicitly (the value must include `/api/v1`):

```bash
VITE_API_URL=http://localhost:8080/api/v1 mise run ui:dev
```

To develop the UI against the full Docker Compose stack instead, stop its UI container first so Vite can take port 7474 — the only origin that stack's API allows (`CORS_ALLOWED_ORIGINS` defaults to `http://localhost:7474`). If Vite starts on another port, the API rejects its requests:

```bash
docker compose -f docker/docker-compose.yml stop alluredeck-ui
VITE_API_URL=http://localhost:5050/api/v1 mise run ui:dev
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

**Development stack (API + PostgreSQL):**

```bash
mise run docker:up-dev    # start API container
mise run docker:down-dev  # stop
mise run docker:logs-dev  # follow logs
```

This stack runs with `SECURITY_ENABLED=0`, so role-checked endpoints return 403, and it sets no `CORS_ALLOWED_ORIGINS`; use `docker:up` for anything that exercises the UI or authentication.

**With S3 storage (MinIO):**

```bash
mise run docker:up-s3     # includes MinIO for local S3 testing
mise run docker:down-s3
mise run docker:logs-s3
```

**Preview against a remote database:**

```bash
export PREVIEW_DATABASE_URL='postgresql://USER:PASS@localhost:15432/alluredeck?sslmode=disable'
mise run docker:up-preview
mise run docker:down-preview
```

See [deployment.md](deployment.md#preview-stack) for what the preview stack does and does not touch.

## mise Tasks

Run `mise tasks` for the full list. Key tasks:

### API Tasks

| Task | Description |
|------|-------------|
| `mise run api:build` | Compile binary to `api/bin/alluredeck-api` |
| `mise run api:build-static` | Static binary matching the Docker build (`CGO_ENABLED=0`, version injected) |
| `mise run api:run` | Build and run locally |
| `mise run api:test` | Run all tests with the race detector (`go test -race ./...`) |
| `mise run api:test-race` | Alias for `api:test` |
| `mise run api:test-integration` | Integration tests (`-tags=integration`); requires `TEST_POSTGRES_URL` |
| `mise run api:test-leak` | Goroutine-leak detection tests (`GOEXPERIMENT=goroutineleakprofile`) |
| `mise run api:test-cover` | Run tests with coverage report (HTML output to `api/bin/coverage.html`) |
| `mise run api:check` | Full quality gate: fmt + vet + lint + test |
| `mise run api:lint` | Run golangci-lint |
| `mise run api:fmt` | Format code with golangci-lint fmt |
| `mise run api:vet` | Run go vet (including `integration` and `leakprofile` build tags) |
| `mise run api:tidy` | Tidy module dependencies (`go mod tidy`) |
| `mise run api:modernize` | Apply Go modernization patterns |
| `mise run api:swagger` | Regenerate the OpenAPI spec in `api/internal/swagger/` (served by Scalar at `/swagger/`) |
| `mise run api:clean` | Remove build artifacts |

### UI Tasks

| Task | Description |
|------|-------------|
| `mise run ui:install` | Install npm dependencies (`npm ci`) |
| `mise run ui:upgrade` | Upgrade all UI dependencies to the latest versions (`npm-check-updates`, then `npm install` and `npm audit`) |
| `mise run ui:dev` | Start Vite dev server on port 7474 |
| `mise run ui:build` | Type-check + build to `ui/dist/` |
| `mise run ui:preview` | Preview production build locally |
| `mise run ui:typecheck` | Run TypeScript type checking |
| `mise run ui:test` | Run tests once (Vitest, CI mode) |
| `mise run ui:test-watch` | Run tests in watch mode |
| `mise run ui:coverage` | Run tests with a coverage report |
| `mise run ui:test-allure` | Run tests and write Allure results to `ui/allure-results` |
| `mise run ui:upload-allure-results` | Upload `ui/allure-results` to a running AllureDeck (`scripts/upload-allure-results.sh`) |
| `mise run ui:dogfood` | `ui:test-allure` followed by the upload, even if tests fail |
| `mise run ui:check` | Full quality gate: typecheck + lint + coverage (runs `ui:coverage`, not `ui:test`) |
| `mise run ui:lint` | Run ESLint |
| `mise run ui:format` | Format source files with Prettier |
| `mise run ui:clean` | Remove build artifacts, coverage, Allure results, and node_modules |

### Combined Tasks

| Task | Description |
|------|-------------|
| `mise run check` | Full quality gate (API + UI) |
| `mise run test` | All tests (API + UI) |
| `mise run clean` | Remove all build artifacts |

### Docker Tasks

| Task | Description |
|------|-------------|
| `mise run docker:build` | Build the API, UI, and MCP images |
| `mise run docker:build-api` | Build API image only |
| `mise run docker:build-ui` | Build UI image only |
| `mise run docker:build-mcp` | Build MCP image only |
| `mise run docker:up` | Start full stack (UI, API, MCP, PostgreSQL) |
| `mise run docker:down` | Stop full stack |
| `mise run docker:logs` | Follow full stack logs |
| `mise run docker:up-dev` | Start API + PostgreSQL dev stack |
| `mise run docker:down-dev` | Stop dev stack |
| `mise run docker:logs-dev` | Follow API logs |
| `mise run docker:up-s3` | Start full stack with MinIO |
| `mise run docker:down-s3` | Stop S3 stack |
| `mise run docker:logs-s3` | Follow S3 stack logs |
| `mise run docker:up-preview` | Start the preview stack against a remote database (requires `PREVIEW_DATABASE_URL`) |
| `mise run docker:down-preview` | Stop the preview stack |
| `mise run docker:clean` | Remove all built images |

### E2E Tasks

Playwright end-to-end tests live in `e2e/`; see [e2e/README.md](../e2e/README.md) for selectors and conventions.

| Task | Description |
|------|-------------|
| `mise run e2e:install` | Install E2E dependencies and the Playwright Chromium browser |
| `mise run e2e:test` | Run the tests in a Playwright Docker container against `http://localhost:7474` / `:5050` (start the stack with `mise run docker:up` first), then upload the report |
| `mise run e2e:test-local` | Run the tests with a local Playwright install (needs `e2e:install`) and upload the report |
| `mise run e2e:test-headed` | Run in headed mode (local only) |
| `mise run e2e:test-ui` | Open Playwright UI mode (local only) |
| `mise run e2e:report` | Serve the last HTML report on port 9323 |
| `mise run e2e:update-snapshots` | Update visual-regression baselines (in Docker) |
| `mise run e2e:clean` | Remove test artifacts; snapshot baselines are kept |

### Docs Tasks

| Task | Description |
|------|-------------|
| `mise run docs:screenshots` | Regenerate `docs/screenshots` with seeded demo data in an isolated Docker stack (compose project `alluredeck-docs`); requires the default stack to be stopped |

`docs:screenshots` refuses to start if ports 7474 or 5050 are in use, so run `mise run docker:down` first. It builds and starts its own UI, API, and PostgreSQL containers, seeds demo data, captures the screenshots, and tears the stack (and its volumes) down on exit.

### Helm Tasks

| Task | Description |
|------|-------------|
| `mise run helm:lint` | Lint the Helm chart |
| `mise run helm:template` | Render templates (validates output) |
| `mise run helm:package` | Package chart as `.tgz` archive |
| `mise run helm:release` | Bump version and commit (`mise run helm:release patch\|minor\|major`) |

### GitHub Actions Tasks

| Task | Description |
|------|-------------|
| `mise run gha:lint` | Security-lint the workflows and `dependabot.yml` with zizmor (set `GH_TOKEN` for online audits) |

### Utility Tasks

| Task | Description |
|------|-------------|
| `mise run fetch-trace-viewer` | Download the Playwright trace viewer assets into `api/static/trace/` for embedding (the Docker build does this itself) |

## Testing

### API Tests

Tests live alongside source files: `foo_test.go` next to `foo.go`. Uses only Go's stdlib `testing` package.

**Run all tests:**

```bash
mise run api:test
```

**Run with race detector** (`api:test` already enables it; `api:test-race` is an alias):

```bash
mise run api:test-race
```

**Integration tests** need a PostgreSQL database:

```bash
TEST_POSTGRES_URL='postgresql://allure:allure@localhost:5432/allure?sslmode=disable' mise run api:test-integration
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

- **Language**: Go 1.26 (module: `github.com/mkutlak/alluredeck/api`)
- **HTTP**: `net/http` stdlib (no third-party router)
- **Auth**: `golang-jwt/jwt/v5` + bcrypt passwords; optional OIDC SSO via `coreos/go-oidc/v3` + `golang.org/x/oauth2`
- **Config**: environment variables + optional YAML (`go.yaml.in/yaml/v3`, `kelseyhightower/envconfig`)
- **Database**: PostgreSQL via `jackc/pgx/v5` + goose v3 migrations
- **Job queue**: River v0.49 (PostgreSQL-backed; `riverdriver/riverpgxv5` driver)
- **Storage**: local filesystem or S3 (`aws-sdk-go-v2`)
- **Report formats**: Allure 2 and Allure 3 (auto-detected); Playwright HTML reports with embedded trace viewer served at `/trace/`
- **Logging**: Uber Zap (`go.uber.org/zap`) — JSON in production, console in development
- **Observability**: OpenTelemetry traces + Prometheus metrics (see [observability.md](observability.md))
- **Documentation**: OpenAPI via `swaggo/swag`, rendered with Scalar at `/swagger/`
- **MCP server**: separate `cmd/mcp` binary (see [operations/mcp-server.md](operations/mcp-server.md))
- **Linting**: golangci-lint v2

### UI (React frontend)

- **Framework**: React 19 + React Router v8
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
- **Type Checking**: TypeScript in strict mode. `ui/package.json` installs two compilers: `typescript` is aliased to the TypeScript 6 compatibility shim (`@typescript/typescript6`), which keeps `typescript-eslint` working (it has no TypeScript 7 support yet), and `typescript7` is the native TypeScript 7 compiler. Both packages ship a `tsc` binary and `typecheck`/`build` call `tsc`, so check which one your install resolves with `cd ui && npx tsc --version` (an installed tree can resolve `node_modules/.bin/tsc` to the 6.x package pulled in by the shim)
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
- **API Configuration**: Base URL configured via `VITE_API_URL` env var (code default `http://localhost:5050`; the value must include `/api/v1`).
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

- Check that `DATABASE_URL` is set and PostgreSQL is reachable — the API exits at startup without a valid one
- Check that port 8080 is not in use: `lsof -i :8080`
- Verify `go.mod` is not corrupted: `cd api && go mod verify`
- Check environment variables: `printenv | grep CONFIG`

### Every API call returns 403 "Access denied: missing claims"

The API is running with `SECURITY_ENABLED=false`, which attaches no identity to requests while role checks still run. Start it with `SECURITY_ENABLED=true`, `ADMIN_USER`, `ADMIN_PASS`, and a non-default `JWT_SECRET_KEY` (see [Running Locally](#running-locally)), or use `mise run docker:up`.

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
- [observability.md](observability.md) — traces, metrics, health endpoints
- [e2e/README.md](../e2e/README.md) — end-to-end test conventions
