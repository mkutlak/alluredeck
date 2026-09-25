<!-- Last reviewed: 2026-04-23. Quarterly or after major dependency upgrades. -->

# Alluredeck

Monorepo for Allure Reports Dashboard — Go API (`api/`), React UI (`ui/`), Helm chart (`charts/`), E2E tests (`e2e/`).

## Subdirectory Rules

- See `api/CLAUDE.md` — Go backend conventions, store interface assertions
- See `ui/CLAUDE.md` — React/TypeScript conventions, URL resolution rules
- See `charts/CLAUDE.md` — Helm chart conventions

## Essential Commands

Run `mise tasks` for the full task list. The toolchain (Go, Node, golangci-lint, helm, yq, swag) is provisioned with `mise install`. Quality gates:

```
mise run api:check      # fmt + vet + lint + test
mise run ui:check       # typecheck + lint + test
mise run check          # full quality gate (API + UI)
```

## Agent Routing

- Go changes → `executor` (sonnet); Go review → `code-reviewer` (opus)
- React/UI changes → `executor` (sonnet); design → `designer` (sonnet)
- Helm/infra → `executor` (sonnet); architecture → `architect` (opus)
- Bug investigation → `debugger` (sonnet) first, then `executor`
- SDK/framework docs → `document-specialist` with Context7 MCP

## Testing

- Write tests first (TDD) — make them fail, then implement
- Only update existing tests with explicit permission
- API: use `testing` stdlib; no third-party test frameworks
- UI: Mock API calls via `vi.mock('../api/...')` or MSW handlers
