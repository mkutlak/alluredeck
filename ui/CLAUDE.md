## UI Conventions
- Path alias `@/` maps to `ui/src/`
- Components use named exports (no default exports for components)
- Tailwind classes sorted by `prettier-plugin-tailwindcss`
- No `any` — use `unknown` and type guards instead
- API base URL configured via `VITE_API_URL` env var
- Test files alongside source or in `ui/src/test/`
- Use Testing Library queries (getByRole, etc.) — avoid `container.querySelector`

## Key Versions
- React 19, React Router v7, Zustand v5
- Vite 7, Vitest 4, TypeScript 5 (strict)
- ESLint 10 (flat config — `eslint.config.js`), Prettier 3
- TanStack Query v5, Recharts 3, Tailwind CSS 4
- Radix UI primitives, shadcn-style components

## Dev Server
Runs on port **7474** (`npm run dev` / `make ui-dev`).

## Coverage Thresholds
Configured in `vitest.config.ts`:
- Lines: 80%, Functions: 80%, Branches: 70%, Statements: 80%
