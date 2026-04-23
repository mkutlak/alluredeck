---
paths:
  - "e2e/**/*.ts"
---

## E2E Selector Conventions

Use `getByTestId()` for structural elements (containers, lists, rows, nav, inputs, iframe, toggles). Use `getByRole()` or `getByText()` only for asserting user-visible copy — page headings, error messages, button labels.

Copy changes, ARIA refactors, and Tailwind churn must not break E2E tests. The `data-testid` is the stable contract between UI and test suite.

### Existing testids
- `sidebar-nav-overview`, `sidebar-nav-analytics`, `sidebar-nav-defects`, `sidebar-nav-timeline`, `sidebar-nav-known-issues`, `sidebar-nav-attachments`
- `project-overview`, `report-list`, `report-row` (with `data-report-id`)
- `projects-grid`, `project-card` (with `data-project-slug`)
- `allure-iframe`, `view-toggle-playwright`, `view-toggle-allure`

### Rules
- Prefer `await expect(locator).toBeVisible({ timeout })` over `waitForTimeout`
- Avoid `.first()` — use a unique testid or `.filter({ has: page.locator('[data-...="..."]') })`
- If the UI lacks a testid you need, add it in the same PR (additive, no behavior change)
- Renaming or deleting a testid is a breaking change — update all specs in the same PR
- Import `test` from `../fixtures/project` for populated projects, else from `../fixtures/auth`
