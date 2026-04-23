# UI

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
- Vite 8, Vitest 4, TypeScript 6 (strict)
- ESLint 10 (flat config — `eslint.config.js`), Prettier 3
- TanStack Query v5, Recharts 3, Tailwind CSS 4
- Radix UI primitives, shadcn-style components

## Key Libraries

- **Icons**: `lucide-react`
- **Command palette**: `cmdk` (Cmd+K global search)
- **Theme toggle**: `next-themes` (Catppuccin Latte / Mocha)
- **Markdown rendering**: `marked` + `dompurify` (safe HTML, used for known-issues notes and webhook payload previews)
- **Syntax highlighting**: `shiki`
- **Timeline Gantt chart**: `d3-selection`, `d3-scale`, `d3-axis`, `d3-brush`, `d3-zoom` (used in `features/timeline/`)
- **Class composition**: `clsx`, `class-variance-authority`, `tailwind-merge`
- **Allure reporting for tests**: `allure-vitest`

## Dev Server

Runs on port **7474** (`npm run dev` / `make ui-dev`).

## Coverage Thresholds

Configured in `vitest.config.ts`:

- Lines: 80%, Functions: 80%, Branches: 70%, Statements: 80%

## URL Conventions

- `/projects/:id` accepts either a numeric `project_id` or a slug string.
- **Navigation links (`<Link to>`, `<NavLink to>`, `navigate()`) MUST use numeric `project_id`, never slugs.** Slugs collide when two child projects share the same name under different parents; numeric IDs are always unique.
- UI display text should show `display_name` or hierarchical `parent/child` labels via `formatProjectLabel()` from `@/lib/projectLabel`.
- The route still accepts both formats for backward compatibility (bookmarks, typed URLs).
- Use `useProjectFromParam(param)` from `@/lib/resolveProject` to resolve the route param to a `ProjectEntry`.
- `resolveProjectFromParam(param, projects)` is the pure helper: `/^\d+$/` params match by `project_id`, others match by `slug`.
- Do NOT do ad-hoc `projects.find(p => p.slug === params.id)` — it silently fails for numeric ids.
- Import: `import { useProjectFromParam } from '@/lib/resolveProject'`
- The hook returns `{ project, isLoading, error }` and internally calls `useQuery(projectListOptions())`.
