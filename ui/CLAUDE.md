## UI Conventions
- Path alias `@/` maps to `ui/src/`
- Components use named exports (no default exports for components)
- Tailwind classes sorted by `prettier-plugin-tailwindcss`
- No `any` — use `unknown` and type guards instead
- API base URL configured via `VITE_API_URL` env var
- Test files alongside source or in `ui/src/test/`
- Use Testing Library queries (getByRole, etc.) — avoid `container.querySelector`
