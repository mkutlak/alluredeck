# allure-dashboard-ui

React + TypeScript frontend for the Allure Dashboard.

## Tech Stack
- **Runtime**: Node.js / npm
- **Framework**: React 18, React Router v6
- **State**: Zustand (global), TanStack Query v5 (server state)
- **UI**: Radix UI primitives + Tailwind CSS + shadcn-style components
- **Charts**: Recharts
- **Build**: Vite 6
- **Test**: Vitest + Testing Library (jsdom)
- **Lint/Format**: ESLint 9 (flat config) + Prettier
- **Types**: TypeScript 5 (strict)

## Commands (use `make`, not npm directly)
```
make install       # npm ci
make dev           # Vite dev server
make build         # tsc + vite build → dist/
make check         # typecheck + lint + test (quality gate)
make typecheck     # tsc --noEmit
make lint          # eslint
make format        # prettier --write
make test          # vitest run (CI)
make test-watch    # vitest watch
make coverage      # vitest + coverage report
make docker-build  # build Docker image
make docker-up     # docker compose up --build -d
make docker-down   # docker compose down
make docker-logs   # follow compose logs
make docker-shell  # exec sh in running container
make docker-clean  # remove built image
make clean         # rm dist/ coverage/ node_modules/
```

## Project Structure
```
src/
  api/        # axios clients & typed API functions
  components/ # shared UI components (shadcn-style)
  features/   # feature-scoped components and logic
  hooks/      # custom React hooks
  lib/        # utilities (cn, formatters, etc.)
  routes/     # React Router route definitions
  store/      # Zustand stores
  test/       # shared test helpers / mocks
  types/      # shared TypeScript types
```

## Testing
- Test files live alongside source or in `src/test/`
- Use Testing Library queries (getByRole, etc.) — avoid `container.querySelector`
- Mock API calls via `vi.mock('../api/...')` or MSW handlers

## Conventions
- Path alias `@/` maps to `src/`
- Components use named exports (no default exports for components)
- Tailwind classes sorted by `prettier-plugin-tailwindcss`
- No `any` — use `unknown` and type guards instead
- API base URL configured via `VITE_API_URL` env var
