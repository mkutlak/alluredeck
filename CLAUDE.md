<!-- Last reviewed: 2026-04-23. Quarterly or after major dependency upgrades. -->

# Alluredeck

Monorepo for Allure Reports Dashboard — Go API (`api/`), React UI (`ui/`), Helm chart (`charts/`), E2E tests (`e2e/`).

## Subdirectory Rules

- See @api/CLAUDE.md — Go backend conventions, internal packages, handlers
- See @ui/CLAUDE.md — React/TypeScript conventions, URL resolution rules
- See @charts/CLAUDE.md — Helm chart conventions, values.yaml structure

## Tech Stack

### API (Go backend)
- Go (see `go.mod` for version) — `net/http` stdlib only, no third-party router
- PostgreSQL (pgx/v5 + goose migrations), River job queue (riverpgxv5)
- Local filesystem or S3 storage (aws-sdk-go-v2)
- Allure 2/3 reports (auto-detected) + Playwright HTML reports with trace viewer at `/trace/`
- JWT auth + optional OIDC SSO; Uber Zap logging; Swagger docs via swaggo
- golangci-lint v2; `CGO_ENABLED=0` for production builds

### UI (React frontend)
- React 19, React Router v7, Zustand v5, TanStack Query v5
- Radix UI + Tailwind CSS 4 + shadcn-style components; Recharts 3
- Vite, Vitest + Testing Library; ESLint (flat config) + Prettier
- TypeScript strict mode; see `ui/package.json` for versions

### E2E
- Playwright + TypeScript in Dockerized runner; see `e2e/README.md` for selector conventions

## Essential Commands

Run `make help` for the full target list. Key commands:

```
# Quality gates
make api-check          # fmt + vet + lint + test
make ui-check           # typecheck + lint + test
make check              # full quality gate (API + UI)

# Dev
make api-run            # build + run API locally
make ui-dev             # Vite dev server (port 7474)
make docker-up          # start full stack (UI + API + Postgres)
make docker-up-dev      # start API-only dev stack
make docker-up-s3       # start full stack with S3 (MinIO)

# Test
make api-test-race      # go test -race ./...
make ui-test            # vitest run
make ui-coverage        # vitest with coverage report
make e2e-test           # Playwright tests in Docker + upload report

# Build & Release
make docker-build       # build both Docker images
make helm-release       # bump chart version (BUMP=patch|minor|major)
make api-swagger        # regenerate Swagger docs
```

## Agent Routing

- Go changes → `executor` (sonnet); Go review → `code-reviewer` (opus)
- React/UI changes → `executor` (sonnet); design → `designer` (sonnet)
- Helm/infra → `executor` (sonnet); architecture → `architect` (opus)
- Bug investigation → `debugger` (sonnet) first, then `executor`
- SDK/framework docs → `document-specialist` with Context7 MCP

## Development Instructions

- Delegate specialized or tool-heavy work to the most appropriate agent.
- Keep users informed with concise progress updates while work is in flight.
- Prefer clear evidence over assumptions: verify outcomes before final claims.
- Choose the lightest-weight path that preserves quality (direct action, or agent).
- Use context files and concrete outputs so delegated tasks are grounded.
- Consult official documentation before implementing with SDKs, frameworks, or APIs - context7 MCP.

## Testing

- Write tests first (TDD) — make them fail, then implement
- Only update existing tests with explicit permission
- API: use `testing` stdlib; no third-party test frameworks
- UI: Mock API calls via `vi.mock('../api/...')` or MSW handlers
- E2E: use `getByTestId()` for structure, `getByRole()`/`getByText()` only for asserting user-visible copy
