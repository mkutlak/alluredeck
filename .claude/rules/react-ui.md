---
paths:
  - "ui/src/**/*.ts"
  - "ui/src/**/*.tsx"
---

## React UI Rules

- Path alias `@/` maps to `ui/src/`
- Components use named exports (no default exports)
- No `any` — use `unknown` and type guards
- Tailwind classes sorted by `prettier-plugin-tailwindcss`
- Use Testing Library queries (getByRole, etc.) — avoid `container.querySelector`

### URL Navigation (critical)
- Navigation links (`<Link to>`, `<NavLink to>`, `navigate()`) MUST use numeric `project_id`, never slugs
- Slugs collide when two child projects share the same name; numeric IDs are always unique
- Use `useProjectFromParam(param)` from `@/lib/resolveProject` to resolve route params
- Do NOT do ad-hoc `projects.find(p => p.slug === params.id)` — fails for numeric ids
- Display text: use `formatProjectLabel()` from `@/lib/projectLabel`
